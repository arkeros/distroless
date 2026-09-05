"""Unit tests for install.bzl's upstream-identity helpers."""

load("@bazel_skylib//lib:unittest.bzl", "asserts", "unittest")
load(":install.bzl", "upstream_identity", "upstream_version")

def _upstream_version_test(ctx):
    env = unittest.begin(ctx)
    asserts.equals(env, "1.30.4", upstream_version("2:1.30.4-1.el10.ngx"))
    asserts.equals(env, "1.30.4", upstream_version("1.30.4-1.el10.ngx"))
    asserts.equals(env, "1.30.4", upstream_version("1.30.4"))

    # Only the release is stripped; a dash inside the version survives.
    asserts.equals(env, "1.2-rc1", upstream_version("1.2-rc1-3.el10"))
    return unittest.end(env)

def _upstream_identity_test(ctx):
    env = unittest.begin(ctx)
    purl, cpe = upstream_identity("nginx", "2:1.31.5-1.el10.ngx", "aarch64", "f5:nginx")
    asserts.equals(env, "pkg:generic/nginx@1.31.5?arch=aarch64", purl)
    asserts.equals(env, "cpe:2.3:a:f5:nginx:1.31.5:*:*:*:*:*:*:*", cpe)

    # purl-reserved characters in the version are percent-encoded in the
    # purl. In the CPE, every printable non-alphanumeric character other
    # than `-`, `.` and `_` is backslash-escaped, per the CPE 2.3 formatted
    # string binding and as syft writes them.
    purl, cpe = upstream_identity("nginx", "1.31.0~rc1+git-1.el10", "x86_64", "f5:nginx")
    asserts.equals(env, "pkg:generic/nginx@1.31.0~rc1%2Bgit?arch=x86_64", purl)
    asserts.equals(env, "cpe:2.3:a:f5:nginx:1.31.0\\~rc1\\+git:*:*:*:*:*:*:*", cpe)
    return unittest.end(env)

upstream_version_test = unittest.make(_upstream_version_test)
upstream_identity_test = unittest.make(_upstream_identity_test)

def install_test_suite(name):
    unittest.suite(
        name,
        upstream_version_test,
        upstream_identity_test,
    )
