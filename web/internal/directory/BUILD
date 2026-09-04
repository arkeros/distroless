load("@aspect_rules_js//js:defs.bzl", "js_test")
load("@rules_go//go:def.bzl", "go_library", "go_test")

package(default_visibility = ["//web:__subpackages__"])

go_test(
    name = "directory_test",
    srcs = [
        "page_test.go",
        "sbom_test.go",
        "scan_test.go",
        "version_test.go",
        "versions_test.go",
        "vulnerabilities_test.go",
    ],
    deps = [":directory"],
)

# The Go tests render the templates but never run the script they reference,
# so this is the only thing that would notice directory.mjs breaking.
js_test(
    name = "directory_js_test",
    size = "small",
    data = ["static/directory.mjs"],
    entry_point = "directory_test.mjs",
)

go_library(
    name = "directory",
    srcs = [
        "catalog.go",
        "page.go",
        "sbom.go",
        "scan.go",
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
        "static/directory.mjs",
        "static/logos/LICENSE.txt",
        "static/logos/bash.svg",
        "static/logos/java.svg",
        "static/logos/nginx.svg",
        "static/logos/node.svg",
        "static/main.mjs",
        "templates/index.html",
        "templates/sbom.html",
        "templates/versions.html",
        "templates/vulnerabilities.html",
    ],
    importpath = "github.com/arkeros/distroless/web/internal/directory",
)
