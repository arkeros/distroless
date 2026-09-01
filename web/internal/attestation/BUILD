load("@bazel_skylib//rules:copy_file.bzl", "copy_file")
load("@rules_go//go:def.bzl", "go_library", "go_test")

package(default_visibility = ["//web:__subpackages__"])

# `go:embed` resolves paths relative to the package, so the fetched root has to
# be staged here under the name attestation.go asks for. Embedded, not a
# runfile: the image is built with `include_runfiles = False`.
copy_file(
    name = "trusted_root",
    src = "@sigstore_trusted_root//file",
    out = "trusted_root.json",
)

go_library(
    name = "attestation",
    srcs = ["attestation.go"],
    embedsrcs = ["trusted_root.json"],
    importpath = "github.com/arkeros/distroless/web/internal/attestation",
    deps = [
        "@com_github_sigstore_sigstore_go//pkg/bundle",
        "@com_github_sigstore_sigstore_go//pkg/root",
        "@com_github_sigstore_sigstore_go//pkg/verify",
    ],
)

go_test(
    name = "attestation_test",
    srcs = ["attestation_test.go"],
    deps = [
        ":attestation",
        "@com_github_sigstore_sigstore_go//pkg/testing/ca",
        "@com_github_sigstore_sigstore_go//pkg/verify",
    ],
)
