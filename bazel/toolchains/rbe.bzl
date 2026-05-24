"""Remote execution platform for BuildBuddy workers.

Defines a single `platform()` target (`@rbe_platform//:rbe_platform`) that:

  - Constrains execution to linux + the host CPU arch (so `--config=rbe` from
    an amd64 host schedules x86_64 actions, arm64 host schedules aarch64).
  - Pins the BuildBuddy worker container image by SHA. We use BuildBuddy's
    stock `rbe-ubuntu24-04` image; senku's actions are hermetic enough not
    to need a custom image (LLVM cc toolchain extracts into the container;
    grype/cosign/kubectl/helm come from `//bazel/toolchains` repos). Bump
    the SHA deliberately when picking up a new ubuntu24 base. (BuildBuddy
    publishes 16/20/22/24 variants under gcr.io/flame-public/rbe-ubuntu*;
    26.04 isn't out yet as of 2026-05.)
  - Asserts `@llvm//constraints/libc:gnu.2.28` to let the hermetic LLVM
    toolchain select the right glibc-targeting variant (ubuntu24 ships
    glibc 2.39 at runtime; LLVM's gnu.2.28 baseline links forward-
    compatibly so binaries built here run on anything ≥ 2.28).

Pattern adapted from openai/codex's `rbe.bzl` (they bring their own image to
bundle `dotslash`/`git`/`python3` for integration tests; senku doesn't need
that yet — fall back to BB's stock image and add tooling if a test ever
needs it).
"""

# `gcr.io/flame-public/rbe-ubuntu24-04:latest` resolved 2026-05-24.
# Bump deliberately; this digest is what every RBE action runs against.
_RBE_UBUNTU24_IMAGE_DIGEST = "sha256:f7db0d4791247f032fdb4451b7c3ba90e567923a341cc6dc43abfc283436791a"

def _impl(rctx):
    arch_to_cpu = {
        "x86_64": "x86_64",
        "amd64": "x86_64",
        "aarch64": "aarch64",
        "arm64": "aarch64",
    }
    cpu = arch_to_cpu.get(rctx.os.arch)
    if cpu == None:
        fail("Unsupported host arch for rbe_platform: {}".format(rctx.os.arch))

    rctx.file("BUILD.bazel", """\
platform(
    name = "rbe_platform",
    constraint_values = [
        "@platforms//cpu:{cpu}",
        "@platforms//os:linux",
        "@bazel_tools//tools/cpp:clang",
        "@llvm//constraints/libc:gnu.2.28",
    ],
    exec_properties = {{
        "container-image": "docker://gcr.io/flame-public/rbe-ubuntu24-04@{digest}",
        "OSFamily": "Linux",
    }},
    visibility = ["//visibility:public"],
)
""".format(
        cpu = cpu,
        digest = _RBE_UBUNTU24_IMAGE_DIGEST,
    ))

rbe_platform_repository = repository_rule(
    implementation = _impl,
    doc = "Emits @rbe_platform//:rbe_platform for BuildBuddy remote execution.",
)
