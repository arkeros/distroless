"""Generate Terraform `.tf.json` from Starlark and run terraform via Bazel.

Three primitives:

- Resource constructors that return a struct with `.tf` (the JSON body) and
  one attribute per cross-resource reference. Wrap with `resource(...)` so
  the construction stays terse. Provider-specific constructors live in
  `resources/<provider>.bzl` next door.

- `tf_root(name, docs, backend_bucket, ...)`: emit `main.tf.json` +
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
    ":lockfile.bzl",
    _tf_root_provider_artifacts = "tf_root_provider_artifacts",
)
load(":rule.bzl", "tf_runner")

# ---------- references ------------------------------------------------------

def resource(rtype, name, body, attrs = ()):
    """Wrap one Terraform resource as a struct with refs to its attributes.

    `.tf` is the JSON dict that goes into the root. Each name in `attrs` becomes
    a struct field whose value is the interpolation string `${rtype.name.attr}`,
    so callers can do `service_cloudrun(service_account_email = sa.email, ...)`
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

def output(name, value, description = None):
    """Declare a Terraform `output`. Sugar over `{"output": {name: {...}}}`.

    Pass these into `tf_root(docs = [...])` alongside `resource` structs;
    `tf_root` lifts `.tf` off and merges them into the root document. Outputs
    land in the state file and are readable by other roots via `remote_state`.
    """
    body = {"value": value}
    if description != None:
        body["description"] = description
    return struct(tf = {"output": {name: body}})

def variable(name, type = "string", description = None, default = None, sensitive = None):
    """Declare a Terraform input `variable`. Sugar over `{"variable": {name: {...}}}`.

    Reference the value with `var(name)` (which returns `${var.<name>}`).
    Omit `default` to require the value at plan time (`-input=false` then
    fails fast when `$TF_VAR_<name>` isn't set).
    """
    body = {"type": type}
    if description != None:
        body["description"] = description
    if default != None:
        body["default"] = default
    if sensitive != None:
        body["sensitive"] = sensitive
    return struct(tf = {"variable": {name: body}})

def remote_state(name, prefix, outputs, bucket):
    """Read another tf_root's outputs via `terraform_remote_state`.

    Each name in `outputs` becomes a struct field on the result, whose value is
    `${data.terraform_remote_state.<name>.outputs.<output>}`. The upstream root
    must have been applied at least once so its state file exists in GCS.

    `bucket` is required — the module doesn't carry a default since the
    bucket is a per-consumer constant.
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

def merge_tf(*structs_or_dicts):
    """Merge any mix of resource-structs (with `.tf`) and raw dicts.

    Convenience over `_merge` for callers composing several `resource(...)`
    structs into one bundle: lifts `.tf` off each struct then runs the
    same three-level deep merge.
    """
    return _merge(*[d.tf if hasattr(d, "tf") else d for d in structs_or_dicts])

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

# --- deploy DAG metadata -----------------------------------------------------
#
# The deploy graph is published as tags rather than kept in an orchestrator-side
# list, so what a node is and what it waits for travel with the node itself.
# `tf_root` is a macro — its arguments are erased at analysis time — so tags on
# the public target are the only thing `bazel query` can still see.
#
# Nodes are *operations*, not roots. A Terraform root is one kind of node; a
# plain runnable (senku pushes container images) is another. Modelling the
# operation is what lets an edge point at the thing that actually has the
# dependency: an app's image push needs the registry to exist, while the app's
# Terraform does not.

# Marks every `tf_root`, deployable or not. Handy for "show me all the roots";
# not what the orchestrator selects on.
DEPLOY_TAG_ROOT = "tf-root"

# Membership. A node without this is never planned or applied — examples and
# fixtures opt out here.
DEPLOY_TAG_DEPLOY = "tf-deploy"

# How to run the node. A root is driven through its `.apply` runnable; a task
# is a runnable that is simply executed. The module stays agnostic about what a
# task does — that a task might push an image is senku's business, not ours.
DEPLOY_TAG_KIND_ROOT = "tf-kind=root"

DEPLOY_TAG_KIND_TASK = "tf-kind=task"

# Roots that provision the CI identity itself, so a CI-side apply could revoke
# its own permissions. Skipped under `$CI`, walked locally.
DEPLOY_TAG_BOOTSTRAP = "tf-bootstrap"

# One per edge, as `tf-after=<absolute label>`. The named node must have run to
# completion first.
DEPLOY_TAG_AFTER = "tf-after="

def _abs_label(label, context):
    """Absolute form of `label`, so a tag matches what `bazel query` prints.

    Relative labels are resolved against the calling package, which is what
    makes `deploy_after = [":image_push"]` work in the common case of a node
    depending on a sibling.
    """
    if label.startswith("//"):
        return label
    if label.startswith(":"):
        return "//{}{}".format(native.package_name(), label)
    fail("{}: deploy_after entries must be labels (`//pkg:target` or `:target`), got {}".format(
        context,
        label,
    ))

def deploy_tags(after = [], kind = DEPLOY_TAG_KIND_TASK, context = "deploy_tags"):
    """Tags marking any target as a node in the deploy DAG.

    Exported for nodes that are not `tf_root`s. senku uses it to make a
    container-image push a first-class node: the push is what needs the
    registry to exist, so the push is what carries the edge.

    Args:
        after: labels of nodes that must run first.
        kind: `DEPLOY_TAG_KIND_TASK` (run the target) or
            `DEPLOY_TAG_KIND_ROOT` (run its `.apply`).
        context: name used in error messages.
    """
    return [DEPLOY_TAG_DEPLOY, kind] + [
        DEPLOY_TAG_AFTER + _abs_label(a, context) for a in after
    ]

def deploy_task(name, run, after = None, deploy = True, visibility = None):
    """A deploy-DAG node that runs an existing target.

    The counterpart to `tf_root`: a node that is a plain runnable rather than
    a Terraform root. Use it for a step that has to happen in a particular
    place in the deploy order but manages no state of its own — pushing a
    container image, warming a cache, running a smoke check.

    Prefer this over a `tf_root` with `docs = []`. That works, but every root
    carries a backend, so a "root" managing nothing still creates a state
    object, takes an `init`/`apply` round trip on every deploy, and shows up
    as "No changes." in every plan.

    Emitted as an alias, so the node has its own label in the graph while the
    underlying target stays independently runnable. `manual` keeps it out of
    wildcard builds — being in the deploy DAG is what makes it run.

    Args:
        name: Target name; this is the node's label.
        run: Label of the executable to run.
        after: Labels of nodes that must run first.
        deploy: False leaves the alias in place but out of the graph.
        visibility: Standard.
    """
    native.alias(
        name = name,
        actual = run,
        tags = ["manual"] + (deploy_tags(
            after = after or [],
            context = "deploy_task({})".format(name),
        ) if deploy else []),
        visibility = visibility,
    )

def _root_deploy_tags(name, deploy, bootstrap, deploy_after):
    if not deploy:
        # Still identifiable as a root, just not part of the graph.
        return [DEPLOY_TAG_ROOT]
    tags = [DEPLOY_TAG_ROOT] + deploy_tags(
        after = deploy_after,
        kind = DEPLOY_TAG_KIND_ROOT,
        context = "tf_root({})".format(name),
    )
    if bootstrap:
        tags.append(DEPLOY_TAG_BOOTSTRAP)
    return tags

def tf_root(
        name,
        docs,
        backend_bucket,
        backend_prefix = None,
        required_providers = None,
        required_version = ">= 1.14.0",
        tfvars = None,
        modules = None,
        pre_apply = None,
        main_postprocess = None,
        providers = None,
        deploy = True,
        bootstrap = False,
        deploy_after = None,
        visibility = None):
    """Emit `.tf.json` files + plan/apply runnables for one Terraform root.

    Args:
        name: Target name. Runnables are `:<name>.{plan,apply,destroy}`. The
            generated files land in `<name>/` under the package's bazel-bin.
        docs: List of resource structs (from `resource`/`remote_state`) and/or
            raw dicts shaped like Terraform JSON (`output`, `module`, ...).
            Empty list is allowed and produces a backend-only root.
        backend_bucket: GCS bucket holding state. Required — no default.
        backend_prefix: GCS state prefix for this root. Defaults to the
            calling package's path (`native.package_name()`), which is the
            convention for new roots. Pass an explicit value only when the
            existing state lives at a different prefix (legacy roots) and
            you'd rather not migrate it.
        required_providers: Legacy escape hatch — explicit
            `terraform.required_providers` dict written into
            `backend.tf.json`. Mutually exclusive with `providers`. Use
            `providers` for new code; this is kept for the rare case
            where a root needs a hand-rolled spec the central
            `terraform.install(...)` doesn't cover.
        providers: List of `tf_provider_target` labels (typically
            `["@<install_name>//:google", …]`). Each provider's
            source/version is rendered into a sibling
            `providers.tf.json`; its multi-platform hashes drive the
            generated `.terraform.lock.hcl`; its archive files become
            the filesystem-mirror tree under `_providers/`. Together
            these turn the bazel-bin output dir into a self-contained
            terraform working directory — `terraform init` resolves
            providers from the mirror without hitting the network.
        required_version: Terraform CLI version constraint.
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
        main_postprocess: Optional callable `(name, template, out)` that
            consumes a `main.tf.json.tpl` (the JSON-encoded template) and
            produces the final `main.tf.json`. When set, `tf_root` writes
            the template instead of the final and delegates the final-form
            production to the callback. Used by senku's `render.bzl` to
            substitute `IMAGE_URI` sentinels with `image_push` digest URIs.
            Module is otherwise agnostic to the substitution semantics.
        deploy: Whether this root belongs to the deploy set that
            `aspect plan` / `aspect apply` walk. Defaults True, so a new
            root is deployed without anyone remembering to register it —
            forgetting to *register* is silent, whereas a root that
            deploys when it shouldn't fails loudly on the next apply.
            Set False for examples and fixtures: they are still built and
            analyzed, just never planned or applied.
        bootstrap: Marks a root that provisions the CI identity itself.
            `aspect plan` / `aspect apply` skip these under `$CI` — a
            botched CI-side apply could revoke the SA's own permissions
            and leave CI unable to recover — but walk them locally.
        deploy_after: Labels of deploy nodes that must run to completion
            before this root is applied. Absolute (`//pkg:target`) or
            relative to this package (`:target`).
            An edge means "the resources that node creates must already
            exist", which is not the same as what the root `load()`s:
            importing another root's bucket *name* into a log filter is a
            build dependency, not a deploy one. Prefer deriving these from
            data the root already has — a list nobody has to remember to
            update cannot drift.
        visibility: Standard.
    """
    tfvars = tfvars or []
    modules = modules or {}
    pre_apply = list(pre_apply or [])
    providers = providers or []
    if providers and required_providers:
        fail("tf_root({}): pass either `providers` (recommended) or `required_providers` (legacy), not both.".format(name))
    if backend_prefix == None:
        backend_prefix = native.package_name()

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

    if main_postprocess:
        # Stage the JSON as a template (still contains any sentinels), then
        # delegate the final-form to the callback. Used for image-digest
        # substitution by senku's render.bzl; the module itself stays
        # agnostic to what the postprocessor does.
        template_target = "_{}_main_template".format(name)
        write_file(
            name = template_target,
            out = "{}/main.tf.json.tpl".format(name),
            content = [json.encode_indent(main_doc, indent = "  "), ""],
            visibility = ["//visibility:private"],
        )
        main_postprocess(
            name = main_target,
            template = ":" + template_target,
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

    # When `providers` is set, the artifacts rule emits three more
    # files into the same `<name>/` subdir: providers.tf.json (the
    # required_providers block, kept separate so `tf_root` doesn't need
    # macro-time access to provider metadata), .terraform.lock.hcl, and
    # the `_providers/` filesystem-mirror tree. The runner writes a
    # fresh .terraformrc into $WORK at run time with the absolute mirror
    # path baked in (per-host, so not a bazel output).
    if providers:
        artifacts_target = "_{}_artifacts".format(name)
        _tf_root_provider_artifacts(
            name = artifacts_target,
            providers = providers,
            gen_dir = name,
            visibility = ["//visibility:private"],
        )
        generated.append(":" + artifacts_target)

    native.filegroup(
        name = name,
        srcs = generated,
        tags = _root_deploy_tags(name, deploy, bootstrap, deploy_after or []),
        visibility = visibility,
    )

    # Per-verb runners. The `tf_runner` rule resolves the terraform
    # toolchain at analysis time and bakes the resulting paths into a
    # generated wrapper script — no `bazel run`-only `args = [...]`
    # injection — so direct-spawn callers (e.g. `aspect plan` via
    # `runnable`) work without bouncing through `bazel run`.
    #
    # The workdir is the bazel-bin output dir of this tf_root (run.sh
    # resolves it at runtime via rlocation), so two roots in distinct
    # packages naturally get distinct working directories without an
    # explicit key.
    for verb in ("plan", "apply", "destroy"):
        tf_runner(
            name = "{}.{}".format(name, verb),
            verb = verb,
            generated = generated,
            tfvars = tfvars,
            modules = {label: subdir for subdir, label in modules.items()},
            pre_apply = pre_apply,
            tags = ["manual"],
            visibility = visibility,
        )
