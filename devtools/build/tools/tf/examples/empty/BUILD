load("@bazel_skylib//rules:diff_test.bzl", "diff_test")
load("//devtools/build/tools/tf:defs.bzl", "tf_dag", "tf_root")

# Smallest tf_root: no resources, no providers — just a backend block. Used
# as a smoke test for `tf_root` itself: `bazel build :noop` must succeed and
# the generated JSON must match the checked-in snapshots below.
tf_root(
    name = "noop",
    docs = [],
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

# Smoke test for `tf_dag` analysis. Walks one root; real graphs would list
# several in topological order.
tf_dag(
    name = "noop_chain",
    roots = [":noop"],
    verb = "plan",
    visibility = ["//visibility:private"],
)
