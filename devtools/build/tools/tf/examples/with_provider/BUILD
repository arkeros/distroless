load("@bazel_skylib//rules:diff_test.bzl", "diff_test")
load("@bazel_skylib//rules:select_file.bzl", "select_file")
load("//devtools/build/tools/tf:defs.bzl", "tf_root")

# tf_root with one provider declared. Exercises the lockfile,
# .terraformrc-source-of-truth, providers.tf.json, and mirror layout
# that `_tf_root_provider_artifacts` emits — the empty/ example next
# door covers the no-providers path.
tf_root(
    name = "demo",
    docs = [],
    providers = ["@terraform_providers//:google"],
    visibility = ["//visibility:private"],
)

# select_file extracts one specific file from the multi-file
# `_demo_artifacts` target so diff_test (which expects a single file)
# can pin its content. The subpath matches what the artifacts rule
# declares relative to the rule's package.

select_file(
    name = "_lockfile",
    srcs = ":_demo_artifacts",
    subpath = "demo/.terraform.lock.hcl",
)

select_file(
    name = "_providers_json",
    srcs = ":_demo_artifacts",
    subpath = "demo/providers.tf.json",
)

select_file(
    name = "_version_index",
    srcs = ":_demo_artifacts",
    subpath = "demo/_providers/registry.terraform.io/hashicorp/google/7.29.0.json",
)

select_file(
    name = "_provider_index",
    srcs = ":_demo_artifacts",
    subpath = "demo/_providers/registry.terraform.io/hashicorp/google/index.json",
)

# Pin the JSON / HCL shape of every file the artifacts rule emits.
# A regression in `lockfile.bzl` (renderer change, layout change,
# field reordering) would surface as a snapshot diff and fail this
# test before it could ship.

diff_test(
    name = "demo_main_test",
    file1 = ":_demo_main",
    file2 = "expected_main.tf.json",
)

diff_test(
    name = "demo_backend_test",
    file1 = ":_demo_backend",
    file2 = "expected_backend.tf.json",
)

diff_test(
    name = "demo_providers_test",
    file1 = ":_providers_json",
    file2 = "expected_providers.tf.json",
)

diff_test(
    name = "demo_lockfile_test",
    file1 = ":_lockfile",
    file2 = "expected_lock.hcl",
)

diff_test(
    name = "demo_version_index_test",
    file1 = ":_version_index",
    file2 = "expected_version.json",
)

diff_test(
    name = "demo_provider_index_test",
    file1 = ":_provider_index",
    file2 = "expected_index.json",
)
