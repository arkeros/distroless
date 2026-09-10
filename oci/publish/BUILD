load("@bazel_skylib//rules:diff_test.bzl", "diff_test")
load("@bazel_skylib//rules:write_file.bzl", "write_file")
load("@jq.bzl//jq:jq.bzl", "jq")
load("@rules_shell//shell:sh_library.bzl", "sh_library")
load("@rules_shell//shell:sh_test.bzl", "sh_test")

# The rule the `publish` job applies before adding an attestation to a digest
# that already carries one of the kind: a jq module, so it is the same program
# here, in CI and on a developer machine. See ADR 0015.
exports_files(["stale.jq"])

# Every case through the rule; the output is the list of cases that came out
# wrong, and the test is that the list is empty. jq rather than jq_test, whose
# error-message templating breaks on filters with quotes and parentheses.
jq(
    name = "stale_test_failures",
    srcs = ["stale_test_cases.json"],
    out = "stale_test_failures.json",
    # The module is found by the search path, not read as input.
    args = [
        "-L",
        package_name(),
    ],
    data = ["stale.jq"],
    filter_file = "stale_test.jq",
)

write_file(
    name = "stale_test_expected",
    out = "stale_test_expected.json",
    content = [
        "[]",
        "",
    ],
)

diff_test(
    name = "stale_test",
    size = "small",
    failure_message = "stale.jq answered a case in stale_test_cases.json differently from what the case wants; the list of such cases is the actual file.",
    file1 = ":stale_test_expected",
    file2 = ":stale_test_failures",
)

# The rule the `release` job applies to decide which of the tags it just
# applied are moves worth an event in the family's tag ledger. Same shape as
# stale.jq, for the same reasons. See ADR 0017.
exports_files(["ledger.jq"])

jq(
    name = "ledger_test_failures",
    srcs = ["ledger_test_cases.json"],
    out = "ledger_test_failures.json",
    args = [
        "-L",
        package_name(),
    ],
    data = ["ledger.jq"],
    filter_file = "ledger_test.jq",
)

write_file(
    name = "ledger_test_expected",
    out = "ledger_test_expected.json",
    content = [
        "[]",
        "",
    ],
)

diff_test(
    name = "ledger_test",
    size = "small",
    failure_message = "ledger.jq answered a case in ledger_test_cases.json differently from what the case wants; the list of such cases is the actual file.",
    file1 = ":ledger_test_expected",
    file2 = ":ledger_test_failures",
)

# The `publish` and `verify` jobs run one cosign/slsa-verifier pass per mirror
# image, and both passes are network round trips end to end — 38 images at
# ~25 s each is the 15m51s the publish loop took in run 34259093980. Running
# them concurrently is the whole fix, but doing it with a bare `&` would lose
# the two properties the jobs are built on: that each image's `::group::`
# block is readable, and that a step which cannot tell what the registry holds
# stops the release rather than being skipped past. This driver keeps both.
# Its test is the specification.
exports_files(["parallel.sh"])

# Not a dependency of anything — `publish` runs the checked-out file. It
# exists so the shellcheck aspect in //tools/lint sees the script at all:
# `exports_files` alone leaves it invisible to the linters, and this is a
# file CI runs unattended over every mirror image.
sh_library(
    name = "parallel",
    srcs = ["parallel.sh"],
)

sh_test(
    name = "parallel_test",
    size = "small",
    srcs = ["parallel_test.sh"],
    data = ["parallel.sh"],
    env = {"PARALLEL_SH": "$(rootpath parallel.sh)"},
)
