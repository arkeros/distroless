load("@bazel_skylib//rules:diff_test.bzl", "diff_test")
load("//oci:vex.bzl", "vex_document", "vex_statement")

# Smoke test: a small VEX document covering each supported status. The
# generated JSON must match the checked-in golden byte-for-byte.
vex_document(
    name = "example",
    author = "test@example.com",
    id = "https://example.com/vex/example",
    statements = [
        vex_statement(
            impact_statement = "example never calls iconv() with IBM1390/1399",
            justification = "vulnerable_code_not_in_execute_path",
            products = ["pkg:oci/example"],
            status = "not_affected",
            vulnerability = "CVE-2026-4046",
        ),
        vex_statement(
            products = ["pkg:oci/example"],
            status = "fixed",
            vulnerability = "CVE-2025-0001",
        ),
    ],
    timestamp = "2026-04-30T00:00:00Z",
    visibility = ["//visibility:private"],
)

diff_test(
    name = "example_test",
    file1 = ":example",
    file2 = "expected.openvex.json",
)
