load(":rules_distroless_upstream_identity_test.bzl", "rules_distroless_upstream_identity_test_suite")

exports_files(glob(["*.patch"]))

# The helpers rules_distroless_package_metadata.patch adds are exercised
# here, in the consuming repo, since the patched module has no tests of its
# own.
rules_distroless_upstream_identity_test_suite(name = "rules_distroless_upstream_identity_test")

# CI runs `bazel test //...`, which does not reach into the rules_rpm module
# (bazel/modules is in .bazelignore and the module is consumed as
# @rules_rpm). Its unit tests are pulled in here.
test_suite(
    name = "rules_rpm_test",
    tests = ["@rules_rpm//rpm/private:install_test"],
)
