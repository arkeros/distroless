load("@bazel_skylib//rules:diff_test.bzl", "diff_test")
load("@bazel_skylib//rules:write_file.bzl", "write_file")
load("@rules_go//go:def.bzl", "go_library")
load("//oci:cosign_policy.bzl", "CERTIFICATE_IDENTITY_REGEXP", "CERTIFICATE_OIDC_ISSUER")

# `policy_expected.go` is the regenerated copy used only by `policy_check`;
# compiling it alongside `policy.go` would redeclare every constant.
# gazelle:exclude policy_expected.go

package(default_visibility = ["//web:__subpackages__"])

# `policy.go` is checked in rather than generated into the build graph, because
# a purely generated source file is invisible to every Go tool that is not
# Bazel — `go build`, `go mod tidy` and gopls all fail on a package with no
# sources.
#
# It is still derived from //oci:cosign_policy.bzl, which promises that
# renaming the signing workflow is a one-file edit: `policy_expected`
# regenerates the file and `policy_check` fails when the two drift. Same
# decomposed write_file + diff_test shape as the silent-zero gate in
# //oci:supply_chain.bzl.
write_file(
    name = "policy_expected",
    out = "policy_expected.go",
    content = [
        "// Code generated from //oci:cosign_policy.bzl. DO NOT EDIT.",
        "",
        "// Package policy pins the identity a Mirror attestation must be signed by.",
        "package policy",
        "",
        "const (",
        "\t// CertificateIdentityRegexp matches the OIDC subject of the one",
        "\t// workflow permitted to sign for the Mirror.",
        "\tCertificateIdentityRegexp = `" + CERTIFICATE_IDENTITY_REGEXP + "`",
        "",
        "\t// CertificateOIDCIssuer is the issuer that certificate must come from.",
        "\tCertificateOIDCIssuer = \"" + CERTIFICATE_OIDC_ISSUER + "\"",
        ")",
        "",
    ],
)

diff_test(
    name = "policy_check",
    failure_message = "web/internal/policy/policy.go is stale. Regenerate:\n  bazel build //web/internal/policy:policy_expected && cp bazel-bin/web/internal/policy/policy_expected.go web/internal/policy/policy.go",
    file1 = "policy.go",
    file2 = ":policy_expected",
)

go_library(
    name = "policy",
    srcs = ["policy.go"],
    importpath = "github.com/arkeros/distroless/web/internal/policy",
)
