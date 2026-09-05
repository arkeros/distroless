"""Unit tests for the upstream-identity helpers that
rules_distroless_package_metadata.patch adds to rules_distroless."""

load("@bazel_skylib//lib:unittest.bzl", "asserts", "unittest")

# The module is private to rules_distroless by convention only; it declares
# no load visibility, and the helpers under test are ours, added by the
# patch. buildifier: disable=bzl-visibility
load("@rules_distroless//apt/private:deb_translate_lock.bzl", "upstream_identity", "upstream_version")

def _upstream_version_test(ctx):
    env = unittest.begin(ctx)
    asserts.equals(env, "1.30.4", upstream_version("1.30.4-1~trixie"))
    asserts.equals(env, "1.0", upstream_version("1:1.0-1"))
    asserts.equals(env, "2.41", upstream_version("2.41-12+deb13u2"))
    asserts.equals(env, "1.30.4", upstream_version("1.30.4"))

    # Only the Debian revision is stripped; a dash inside the upstream
    # version survives, as do the characters Debian allows there.
    asserts.equals(env, "1.2-rc1", upstream_version("1.2-rc1-3"))
    asserts.equals(env, "1.3.dfsg+really1.3.2", upstream_version("1:1.3.dfsg+really1.3.2-3"))
    return unittest.end(env)

def _upstream_identity_test(ctx):
    env = unittest.begin(ctx)
    purl, cpe = upstream_identity("nginx", "1:1.30.4-1~trixie", "amd64", "f5:nginx")
    asserts.equals(env, "pkg:generic/nginx@1.30.4?arch=amd64", purl)
    asserts.equals(env, "cpe:2.3:a:f5:nginx:1.30.4:*:*:*:*:*:*:*", cpe)

    # purl-reserved characters in the version are percent-encoded in the
    # purl. In the CPE, every printable non-alphanumeric character other
    # than `-`, `.` and `_` is backslash-escaped, per the CPE 2.3 formatted
    # string binding and as syft writes them.
    purl, cpe = upstream_identity("zlib1g", "1:1.3.dfsg+really1.3.2-3", "arm64", "zlib:zlib")
    asserts.equals(env, "pkg:generic/zlib1g@1.3.dfsg%2Breally1.3.2?arch=arm64", purl)
    asserts.equals(env, "cpe:2.3:a:zlib:zlib:1.3.dfsg\\+really1.3.2:*:*:*:*:*:*:*", cpe)
    purl, cpe = upstream_identity("nginx", "1.31.0~rc1-1~trixie", "amd64", "f5:nginx")
    asserts.equals(env, "pkg:generic/nginx@1.31.0~rc1?arch=amd64", purl)
    asserts.equals(env, "cpe:2.3:a:f5:nginx:1.31.0\\~rc1:*:*:*:*:*:*:*", cpe)
    return unittest.end(env)

upstream_version_test = unittest.make(_upstream_version_test)
upstream_identity_test = unittest.make(_upstream_identity_test)

def rules_distroless_upstream_identity_test_suite(name):
    unittest.suite(
        name,
        upstream_version_test,
        upstream_identity_test,
    )
