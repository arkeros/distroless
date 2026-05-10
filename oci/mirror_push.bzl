"""`mirror_push` — the policy unit for publishing an image to the public mirror.

Wraps `image_push` + `slsa_predicate` + `cosign_sign` + `cosign_attest` (+ a
per-image SBOM attestation if `sbom` is set) into a coherent set of targets.
There is no path to publish to the public mirror surface
(`ghcr.io/arkeros/senku/*`) except via this macro, encoding the
"every mirror image is signed and provenance-attested" policy in the build
graph rather than in CI script discipline.

Per ADR 0006 (`docs/adr/0006-bazel-native-cosign-mirror-signing.md`).
"""

load("@cosign.bzl", "cosign_attest", "cosign_sign", "slsa_predicate")
load("@rules_img//img:push.bzl", "image_push")
load("//oci:config.bzl", "OCI_REGISTRY", "OCI_REPOSITORY_PREFIX")
load("//oci:cosign_policy.bzl", "BUILDER_ID", "BUILD_TYPE_URI")

def mirror_push(
        name,
        image,
        repository,
        tag_list = [],
        sbom = None,
        registry = OCI_REGISTRY,
        repository_prefix = OCI_REPOSITORY_PREFIX,
        bazel_target = None,
        tags = None,
        visibility = None,
        **kwargs):
    """Publish an image to the public mirror with signature + SLSA + SBOM attestations.

    Generates these targets:

      `<name>_predicate`   — Bazel-built SLSA v1.0 ProvenanceStatement JSON.
      `<name>_push`        — `image_push` to `<registry>/<repository_prefix>/<repository>`.
      `<name>_sign`        — `cosign sign --recursive --yes` against the pushed digest.
      `<name>_attest`      — `cosign attest --type=slsaprovenance --predicate=<predicate>`.
      `<name>_attest_sbom` — `cosign attest --type=cyclonedx --predicate=<sbom>` (only when `sbom` is set).

    CI invokes them in order: push, sign, attest, attest_sbom. Each is
    independently re-runnable (cosign is idempotent on `<repo>@<digest>`)
    so transient sign/attest failures retry without re-pushing.

    Signatures and attestations are written via the OCI 1.1 referrers API
    (`subject` field on the manifest) rather than the legacy `.sig`/`.att`
    sibling-tag scheme. Consumers must discover via the referrers endpoint
    or any tool that follows OCI 1.1 (cosign 2.x+, oras, crane).

    Args:
      name: Base name. Sub-targets are derived as `<name>_<step>`.
      image: Label of the image (rules_img `image_index` / `image_manifest`)
        whose `digest` output group will be signed and attested.
      repository: Path under `<repository_prefix>/`, e.g. `"distroless/static"`.
        MUST NOT contain a tag or digest.
      tag_list: List of tag templates to push (rules_img mustache template
        syntax — `{{.STABLE_*}}` placeholders work).
      sbom: Optional label of a CycloneDX SBOM file (typically the
        `<image>_sbom` target produced by `image_supply_chain`). When set,
        adds an `attest_sbom` step.
      registry: Mirror registry. Default: `OCI_REGISTRY` (`ghcr.io`).
      repository_prefix: Path prefix under the registry. Default:
        `OCI_REPOSITORY_PREFIX` (`arkeros/senku`).
      bazel_target: String identifying the Bazel target in the SLSA predicate's
        `externalParameters.bazelTarget`. Defaults to `//<package>:<name>`.
      tags: Bazel tags forwarded to every generated target. Conventionally
        set to `["manual"]` by callers to keep stamp-dependent push/sign/attest
        targets out of `bazel build //...` wildcard expansions — the
        `STABLE_*` workspace status keys change on every commit, so without
        `manual` casual builds re-stamp every mirror image's push wrapper
        and predicate JSON each commit. Note this is a *cache-hygiene*
        concern, not a side-effect protection: `bazel build :foo_push`
        does not push (only `bazel run` triggers the network call), so
        the `manual` tag is about dev-loop performance, not safety.
      visibility: Visibility forwarded to every generated target.
      **kwargs: Forwarded to `image_push` (e.g. `args`, `local_load_only`).
    """
    if ":" in repository or "@" in repository:
        fail("`repository` must not contain a tag or digest, got: {}".format(repository))

    full_repository = "{}/{}".format(repository_prefix, repository)
    full_url = "{}/{}".format(registry, full_repository)

    if bazel_target == None:
        bazel_target = "//{}:{}".format(native.package_name(), name)

    slsa_predicate(
        name = name + "_predicate",
        build_type = BUILD_TYPE_URI,
        builder_id = BUILDER_ID,
        external_parameters = {
            "bazelTarget": bazel_target,
            "sourceUri": "git+https://github.com/arkeros/senku@{{STABLE_GIT_COMMIT}}",
        },
        internal_parameters = {
            "monorepoVersion": "{{STABLE_MONOREPO_VERSION}}",
        },
        tags = tags,
        visibility = visibility,
    )

    # `mirror_push_managed` is the opt-in marker `mirror_push_enforcement_aspect`
    # looks for. Raw `image_push` targets without it that fall under the mirror
    # prefix fail analysis. See `//oci:aspects.bzl`.
    push_tags = ["mirror_push_managed"] + (tags or [])
    image_push(
        name = name + "_push",
        image = image,
        registry = registry,
        repository = full_repository,
        stamp = "force",
        tag_list = tag_list,
        tags = push_tags,
        visibility = visibility,
        **kwargs
    )

    cosign_sign(
        name = name + "_sign",
        image = image,
        repository = full_url,
        referrers_mode = "oci-1-1",
        tags = tags,
        visibility = visibility,
    )

    cosign_attest(
        name = name + "_attest",
        image = image,
        repository = full_url,
        type = "slsaprovenance",
        predicate = ":" + name + "_predicate",
        referrers_mode = "oci-1-1",
        tags = tags,
        visibility = visibility,
    )

    if sbom:
        cosign_attest(
            name = name + "_attest_sbom",
            image = image,
            repository = full_url,
            type = "cyclonedx",
            predicate = sbom,
            referrers_mode = "oci-1-1",
            tags = tags,
            visibility = visibility,
        )
