load("@bazel_skylib//rules:diff_test.bzl", "diff_test")
load("@rules_shell//shell:sh_test.bzl", "sh_test")
load("//cosign:defs.bzl", "slsa_predicate")

# Golden test for `slsa_predicate`. The rule's action runs sed-based
# substitution of `{{STABLE_*}}` placeholders against the workspace status
# file at action time. This fixture uses no placeholders, so the output is
# entirely determined by the rule's static JSON encoding — byte-stable
# across runs and independent of stamp values.
slsa_predicate(
    name = "fixture_predicate",
    build_type = "https://example.com/test-build-type/v1",
    builder_id = "https://example.com/builder/test@refs/heads/main",
    external_parameters = {
        "bazelTarget": "//test:fixture",
        "sourceUri": "git+https://example.com/repo.git@deadbeef",
    },
    internal_parameters = {
        "monorepoVersion": "1.2.3-test",
    },
    tags = ["manual"],
)

diff_test(
    name = "fixture_predicate_test",
    file1 = ":fixture_predicate",
    file2 = "fixture_predicate.golden.json",
)

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
