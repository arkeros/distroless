load("@rules_shell//shell:sh_test.bzl", "sh_test")

# Asserts the root cosign.bzl re-export shim stays in sync with the public
# API in defs.bzl + toolchain.bzl. Catches the silent-drift case where a
# new rule is added to defs.bzl and not re-exported from the root.
sh_test(
    name = "shim_parity_test",
    srcs = ["shim_parity_test.sh"],
    data = [
        "//:cosign.bzl",
        "//cosign:defs.bzl",
        "//cosign/toolchain:toolchain.bzl",
    ],
)
