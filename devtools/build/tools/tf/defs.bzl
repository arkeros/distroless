"""Generate Terraform `.tf.json` from Starlark and run terraform via Bazel.

Three primitives:

- Resource constructors that return a struct with `.tf` (the JSON body) and
  one attribute per cross-resource reference. Wrap with `resource(...)` so
  the construction stays terse. Provider-specific constructors live in
  `resources/<provider>.bzl` next door.

- `tf_root(name, docs, backend_prefix, ...)`: emit `main.tf.json` +
  `backend.tf.json` for one Terraform root, plus `:<name>.{plan,apply,destroy}`
  runnable targets that exec terraform against the generated dir.

Cross-root sequencing (`apply gar, then registry, then lb`) is *not* this
file's job — that's CI's job graph (or a task runner like `mise`/`just`
locally). Bazel owns the build/test DAG; deploy ordering belongs to the
runner that's already orchestrating the rest of the pipeline.

Terraform's interpolation language stays — `${...}` strings flow through the
generated JSON unchanged. Starlark only handles things resolvable at
generation time (loops, defaults, shared constants); cross-resource refs are
emitted as strings and resolved by Terraform at plan time.
"""

load("@bazel_skylib//rules:write_file.bzl", "write_file")
load(
    ":render.bzl",
    _IMAGE_URI = "IMAGE_URI",
    _render_main_with_image = "render_main_with_image",
)
load(":rule.bzl", "tf_runner")

# Re-export `IMAGE_URI` so callers loading `tf_root` from this file can also
# pull the sentinel without a second load line. Starlark's `load` makes the
# symbol available locally but does not re-export — assigning to a top-level
# name does.
IMAGE_URI = _IMAGE_URI

_DEFAULT_BUCKET = "senku-prod-terraform-state"

# ---------- references ------------------------------------------------------

def resource(rtype, name, body, attrs = ()):
    """Wrap one Terraform resource as a struct with refs to its attributes.

    `.tf` is the JSON dict that goes into the root. Each name in `attrs` becomes
    a struct field whose value is the interpolation string `${rtype.name.attr}`,
    so callers can do `cloud_run_service(service_account_email = sa.email, ...)`
    without hand-formatting reference strings.
    """
    refs = {a: "${%s.%s.%s}" % (rtype, name, a) for a in attrs}
    return struct(
        tf = {"resource": {rtype: {name: body}}},
        addr = "{}.{}".format(rtype, name),
        **refs
    )

def var(name):
    """Reference a Terraform input variable: `${var.<name>}`."""
    return "${var.%s}" % name

def remote_state(name, prefix, outputs, bucket = _DEFAULT_BUCKET):
    """Read another tf_root's outputs via `terraform_remote_state`.

    Each name in `outputs` becomes a struct field on the result, whose value is
    `${data.terraform_remote_state.<name>.outputs.<output>}`. The upstream root
    must have been applied at least once so its state file exists in GCS.
    """
    refs = {
        o: "${data.terraform_remote_state.%s.outputs.%s}" % (name, o)
        for o in outputs
    }
    return struct(
        tf = {"data": {"terraform_remote_state": {name: {
            "backend": "gcs",
            "config": {"bucket": bucket, "prefix": prefix},
        }}}},
        addr = "data.terraform_remote_state.{}".format(name),
        **refs
    )

# ---------- merge -----------------------------------------------------------

def _merge(*docs):
    """Three-level deep merge of Terraform-JSON-shaped dicts; later docs win.

    Targets the actual shape: L0 keys (resource/data/module/provider/output/
    terraform/locals), L1 keys (type/provider name), L2 keys (instance name).
    Below that — the instance body — `later wins` is the right semantic since
    overlap inside a single resource's body is a bug, not a merge.

    Iterative because Starlark forbids recursion. Copies on conflict so input
    dicts are never mutated.
    """
    out = {}
    for d in docs:
        for k, v in d.items():
            if k in out and type(out[k]) == type({}) and type(v) == type({}):
                merged_l1 = dict(out[k])
                for k2, v2 in v.items():
                    if k2 in merged_l1 and type(merged_l1[k2]) == type({}) and type(v2) == type({}):
                        merged_l2 = dict(merged_l1[k2])
                        for k3, v3 in v2.items():
                            merged_l2[k3] = v3
                        merged_l1[k2] = merged_l2
                    else:
                        merged_l1[k2] = v2
                out[k] = merged_l1
            else:
                out[k] = v
    return out

# ---------- tf_root ---------------------------------------------------------

def tf_root(
        name,
        docs,
        backend_prefix = None,
        required_providers = None,
        required_version = ">= 1.14.0",
        backend_bucket = _DEFAULT_BUCKET,
        tfvars = None,
        modules = None,
        pre_apply = None,
        image_push = None,
        visibility = None):
    """Emit `.tf.json` files + plan/apply runnables for one Terraform root.

    Args:
        name: Target name. Runnables are `:<name>.{plan,apply,destroy}`. The
            generated files land in `<name>/` under the package's bazel-bin.
        docs: List of resource structs (from `resource`/`remote_state`) and/or
            raw dicts shaped like Terraform JSON (`output`, `module`, ...).
            Empty list is allowed and produces a backend-only root.
        backend_prefix: GCS state prefix for this root. Defaults to the
            calling package's path (`native.package_name()`), which is the
            convention for new roots. Pass an explicit value only when the
            existing state lives at a different prefix (legacy roots) and
            you'd rather not migrate it.
        required_providers: Optional dict for `terraform.required_providers`.
            Defaults to None — emit no `required_providers` block, providers
            are inferred from resource types in the JSON.
        required_version: Terraform CLI version constraint.
        backend_bucket: GCS bucket holding state.
        tfvars: Optional list of labels whose default outputs are JSON files
            named `*.auto.tfvars.json` (Terraform auto-loads any `.json` file
            ending in `.auto.tfvars.json` in the working directory). Each is
            copied into the workdir under its basename, so the source label
            must already produce the right filename.
        modules: Optional dict `{subdir_name: filegroup_label}`. The
            filegroup's files are copied into `<workdir>/<subdir_name>/`, so
            modules in the generated JSON can reference `./<subdir_name>` as
            their `source`.
        pre_apply: Optional list of runnable labels invoked (in order) before
            `terraform apply`. Used for image push or other side effects that
            must happen between Bazel build and Terraform apply. NOT run on
            `plan` or `destroy`.
        image_push: Optional label of an `image_push` target. When set, every
            occurrence of the `IMAGE_URI` sentinel in `docs` is substituted
            at Bazel build time with `<registry>/<repo>@<digest>` from the
            target's deploy manifest. Replaces the older flow of declaring
            a `variable "image"` block fed by an `image.auto.tfvars.json`.
        visibility: Standard.
    """
    tfvars = tfvars or []
    modules = modules or {}
    pre_apply = list(pre_apply or [])
    if backend_prefix == None:
        backend_prefix = native.package_name()

    # When `image_push` is set, the image must be pushed to the registry
    # before Cloud Run can pull it on apply. Auto-prepend to `pre_apply` so
    # callers don't have to repeat the same label.
    if image_push and image_push not in pre_apply:
        pre_apply = [image_push] + pre_apply
    terraform_block = {
        "required_version": required_version,
        "backend": {"gcs": {
            "bucket": backend_bucket,
            "prefix": backend_prefix,
        }},
    }
    if required_providers != None:
        terraform_block["required_providers"] = required_providers

    backend_doc = {"terraform": terraform_block}
    raw_docs = [d.tf if hasattr(d, "tf") else d for d in docs]
    main_doc = _merge(*raw_docs) if raw_docs else {}

    backend_target = "_{}_backend".format(name)
    main_target = "_{}_main".format(name)

    # Trailing empty string forces a final newline (write_file joins lines
    # with `\n` but does not append one at the end).
    write_file(
        name = backend_target,
        out = "{}/backend.tf.json".format(name),
        content = [json.encode_indent(backend_doc, indent = "  "), ""],
        visibility = ["//visibility:private"],
    )

    if image_push:
        # Stage the JSON as a template (still contains the `IMAGE_URI` sentinel),
        # then substitute the digest URI in via the render macro.
        template_target = "_{}_main_template".format(name)
        write_file(
            name = template_target,
            out = "{}/main.tf.json.tpl".format(name),
            content = [json.encode_indent(main_doc, indent = "  "), ""],
            visibility = ["//visibility:private"],
        )
        _render_main_with_image(
            name = main_target,
            template = ":" + template_target,
            image_push = image_push,
            out = "{}/main.tf.json".format(name),
        )
    else:
        write_file(
            name = main_target,
            out = "{}/main.tf.json".format(name),
            content = [json.encode_indent(main_doc, indent = "  "), ""],
            visibility = ["//visibility:private"],
        )

    generated = [":" + backend_target, ":" + main_target]

    native.filegroup(
        name = name,
        srcs = generated,
        visibility = visibility,
    )

    # Stable workdir key. Two tf_roots in different packages (or with different
    # names) get distinct directories, so terraform's .terraform/ + state
    # lockfiles don't collide.
    root_name = "{}_{}".format(
        native.package_name().replace("/", "_"),
        name,
    )

    # Per-verb runners. The `tf_runner` rule resolves the terraform
    # toolchain at analysis time and bakes the resulting paths into a
    # generated wrapper script — no `bazel run`-only `args = [...]`
    # injection — so direct-spawn callers (e.g. `aspect plan` via
    # `runnable`) work without bouncing through `bazel run`.
    for verb in ("plan", "apply", "destroy"):
        tf_runner(
            name = "{}.{}".format(name, verb),
            verb = verb,
            root_name = root_name,
            generated = generated,
            tfvars = tfvars,
            modules = {label: subdir for subdir, label in modules.items()},
            pre_apply = pre_apply,
            tags = ["manual"],
            visibility = visibility,
        )

