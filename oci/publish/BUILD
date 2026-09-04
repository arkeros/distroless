load("@bazel_skylib//rules:diff_test.bzl", "diff_test")
load("@bazel_skylib//rules:write_file.bzl", "write_file")
load("@jq.bzl//jq:jq.bzl", "jq")

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
