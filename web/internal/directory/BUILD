load("@rules_go//go:def.bzl", "go_library", "go_test")

package(default_visibility = ["//web:__subpackages__"])

go_test(
    name = "directory_test",
    srcs = [
        "page_test.go",
        "sbom_test.go",
        "version_test.go",
        "versions_test.go",
    ],
    deps = [":directory"],
)

go_library(
    name = "directory",
    srcs = [
        "page.go",
        "sbom.go",
        "version.go",
        "versions.go",
    ],
    embedsrcs = [
        "static/fonts/LICENSE.txt",
        "static/fonts/fira-mono-400-latin-ext.woff2",
        "static/fonts/fira-mono-400-latin.woff2",
        "static/fonts/fira-sans-400-latin-ext.woff2",
        "static/fonts/fira-sans-400-latin.woff2",
        "static/fonts/fira-sans-600-latin-ext.woff2",
        "static/fonts/fira-sans-600-latin.woff2",
        "static/directory.css",
        "static/directory.js",
        "templates/sbom.html",
        "templates/versions.html",
    ],
    importpath = "github.com/arkeros/distroless/web/internal/directory",
)
