load("@bazel_skylib//rules:diff_test.bzl", "diff_test")
load("//terraform:defs.bzl", "tf_root")

# Smallest tf_root: no resources, no providers — just a backend block. Smoke
# test for `tf_root` itself: `bazel build :noop` must succeed and the
# generated JSON must match the checked-in snapshots.
tf_root(
    name = "noop",
    docs = [],
    backend_bucket = "example-bucket",
    visibility = ["//visibility:private"],
)

diff_test(
    name = "noop_main_test",
    file1 = ":_noop_main",
    file2 = "expected_main.tf.json",
)

diff_test(
    name = "noop_backend_test",
    file1 = ":_noop_backend",
    file2 = "expected_backend.tf.json",
)
