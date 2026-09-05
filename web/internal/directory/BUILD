load("@aspect_rules_esbuild//esbuild:defs.bzl", "esbuild")
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

# What the browser is sent, as opposed to what is written: main.mjs and the
# module it imports as one minified file, and the stylesheet minified. The
# sources stay the sources — the js_test above imports directory.mjs directly —
# and only the copy the server embeds loses its comments. No source map: there
# is nothing to debug in production that the sources would not show better.
esbuild(
    name = "script",
    srcs = ["static/directory.mjs"],
    entry_point = "static/main.mjs",
    format = "esm",
    minify = True,
    output = "dist/directory.mjs",
    platform = "browser",
    sourcemap = None,
    # The oldest syntax the sources use is ES2020 — `??` and `?.` — and every
    # browser that runs a module script has it, so nothing is rewritten.
    target = "es2020",
)

# Not bundled: the fonts are referenced by URL and served beside it, and
# bundling would have esbuild go looking for them on disk.
esbuild(
    name = "stylesheet",
    bundle = False,
    entry_point = "static/directory.css",
    minify = True,
    output = "dist/directory.css",
    sourcemap = None,
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
        "static/logos/generic.svg",
        "static/logos/java.svg",
        "static/logos/nginx.svg",
        "static/logos/node.svg",
        "static/main.mjs",
        "templates/index.html",
        "templates/sbom.html",
        "templates/topbar.html",
        "templates/versions.html",
        "templates/vulnerabilities.html",
    ],
    importpath = "github.com/arkeros/distroless/web/internal/directory",
)
