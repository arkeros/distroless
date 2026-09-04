"""`mirror_push` — the policy unit for publishing an image to the public mirror.

Wraps `image_push` (by digest) + `cosign_sign` + `cosign_attest` for the SBOM,
the vulnerability scan and the VEX document + `crane_tag` into a coherent set
of targets. There is no path to publish to
the public mirror surface (`ghcr.io/arkeros/distroless/*`) except via this
macro, encoding the "every mirror image is signed and has an SBOM" policy in
the build graph rather than in CI script discipline.

Platform provenance is deliberately *not* here. A predicate the build writes
about itself is forgeable by the build; the platform (slsa-github-generator,
called from CI) attaches provenance after `_push`, and CI's `verify` job proves
all three artifacts before `_tag` runs. Per ADR 0014
(`docs/adr/0014-platform-provenance-slsa-github-generator.md`).
"""

load("@cosign.bzl", "cosign_attest", "cosign_sign")
load("@rules_img//img:push.bzl", "image_push")
load("//oci:config.bzl", "OCI_REGISTRY", "OCI_REPOSITORY_PREFIX")
load("//oci:crane_tag.bzl", "crane_tag")

def mirror_push(
        name,
        image,
        repository,
        tag_list,
        sbom = None,
        vulnerabilities = None,
        vex = None,
        registry = OCI_REGISTRY,
        repository_prefix = OCI_REPOSITORY_PREFIX,
        tags = None,
        visibility = None,
        **kwargs):
    """Publish an image to the public mirror with signature + attestations.

    Generates these targets:

      `<name>_push`        — `image_push` of the digest only (no tags) to
                             `<registry>/<repository_prefix>/<repository>`.
      `<name>_sign`        — `cosign sign --recursive --yes` against the pushed digest.
      `<name>_attest_sbom` — `cosign attest --type=cyclonedx --predicate=<sbom>` (only when `sbom` is set).
      `<name>_attest_vuln` — `cosign attest --type=vuln --predicate=<vulnerabilities>` (only when set).
      `<name>_attest_vex`  — `cosign attest --type=openvex --predicate=<vex>` (only when set).
      `<name>_tag`         — `crane tag <repo>@<digest> <tag>` for every entry
                             of `tag_list`, after stamp expansion.

    CI runs them in two phases. `publish`: push, sign, every `_attest_*`. Then
    the platform attaches provenance and `verify` checks every artifact. Only
    then does `release` run `_tag`, so a tag never names a digest that is not
    fully attested. Each target is independently re-runnable. `_push` and
    `_tag` are idempotent on `<repo>@<digest>`; `_sign` and the `_attest_*`
    targets are not — every run appends another signature or attestation to
    the digest, which verification tolerates but `cosign tree` shows.

    Args:
      name: Base name. Sub-targets are derived as `<name>_<step>`.
      image: Label of the image (rules_img `image_index` / `image_manifest`)
        whose `digest` output group will be signed, attested and tagged.
      repository: Path under `<repository_prefix>/`, e.g. `"distroless/static"`.
        MUST NOT contain a tag or digest.
      tag_list: Tags to apply in `_tag`; at least one. A mirror image no tag
        names is unreachable to consumers, and CI's `release` job runs `_tag`
        for every published target. `{{.STABLE_*}}` placeholders (rules_img
        template syntax) are expanded from the workspace status at build time.
      sbom: Optional label of a CycloneDX SBOM file (typically the
        `<image>_sbom` target produced by `image_supply_chain`). When set,
        adds an `attest_sbom` step.
      vulnerabilities: Optional label of a cosign vulnerability scan record
        (the `<image>_vuln` target `image_supply_chain` produces from the same
        scan that gates the image). When set, adds an `attest_vuln` step. The
        Directory's vulnerabilities page is rendered from this. See ADR 0015.
      vex: Optional label of an OpenVEX document (the `<image>_vex` target
        from the same macro). When set, adds an `attest_vex` step. Kept apart
        from the scan on purpose: the scan says what the scanner found, this
        says what the project makes of it, and each is verifiable alone.
      registry: Mirror registry. Default: `OCI_REGISTRY` (`ghcr.io`).
      repository_prefix: Path prefix under the registry. Default:
        `OCI_REPOSITORY_PREFIX` (`arkeros/distroless`).
      tags: Bazel tags forwarded to every generated target. Conventionally
        set to `["manual"]` by callers to keep stamp-dependent targets out of
        `bazel build //...` wildcard expansions — the `STABLE_*` workspace
        status keys change on every commit, so without `manual` casual builds
        re-stamp every mirror image's tag script each commit. This is a
        cache-hygiene concern, not a side-effect protection: `bazel build
        :foo_push` does not push (only `bazel run` triggers the network call).
      visibility: Visibility forwarded to every generated target.
      **kwargs: Forwarded to `image_push` (e.g. `args`, `local_load_only`).
    """
    if ":" in repository or "@" in repository:
        fail("`repository` must not contain a tag or digest, got: {}".format(repository))
    if not tag_list:
        fail(("mirror_push(name = {}): `tag_list` must name at least one tag. " +
              "`_push` publishes by digest only, so without a tag nothing on the " +
              "mirror would ever point at this image.").format(repr(name)))

    full_repository = "{}/{}".format(repository_prefix, repository)
    full_url = "{}/{}".format(registry, full_repository)

    # `mirror_push_managed` is the opt-in marker `mirror_push_enforcement_aspect`
    # looks for. Raw `image_push` targets without it that fall under the mirror
    # prefix fail analysis. See `//oci:aspects.bzl`.
    #
    # No `tag_list`: the push is by digest only, so nothing a consumer can
    # name resolves to this digest until `_tag` runs after verification.
    push_tags = ["mirror_push_managed"] + (tags or [])
    image_push(
        name = name + "_push",
        image = image,
        registry = registry,
        repository = full_repository,
        tags = push_tags,
        visibility = visibility,
        **kwargs
    )

    cosign_sign(
        name = name + "_sign",
        image = image,
        repository = full_url,
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
            tags = tags,
            visibility = visibility,
        )

    if vulnerabilities:
        cosign_attest(
            name = name + "_attest_vuln",
            image = image,
            repository = full_url,
            type = "vuln",
            predicate = vulnerabilities,
            tags = tags,
            visibility = visibility,
        )

    if vex:
        cosign_attest(
            name = name + "_attest_vex",
            image = image,
            repository = full_url,
            type = "openvex",
            predicate = vex,
            tags = tags,
            visibility = visibility,
        )

    crane_tag(
        name = name + "_tag",
        image = image,
        repository = full_url,
        tag_list = tag_list,
        tags = tags,
        visibility = visibility,
    )
