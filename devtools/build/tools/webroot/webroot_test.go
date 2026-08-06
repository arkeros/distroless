package webroot_test

import (
	"testing"

	"github.com/arkeros/senku/devtools/build/tools/webroot"
)

// The rules a react_app's webroot ships with, in the order
// `//devtools/build/react_component:cache.bzl` declares them. Mirrors what
// the nginx `default.conf` expresses with location precedence: the prefixes
// beat the extension suffixes.
func appRules() webroot.Rules {
	return webroot.Rules{
		DefaultCacheControl: "no-cache",
		Rules: []webroot.Rule{
			{Match: webroot.MatchPrefix, Path: "dino_bundle/", CacheControl: immutable},
			{Match: webroot.MatchPrefix, Path: "assets/", CacheControl: immutable},
			{Match: webroot.MatchSuffix, Path: ".html", CacheControl: "no-cache"},
			{Match: webroot.MatchSuffix, Path: ".css", CacheControl: "no-cache"},
			{Match: webroot.MatchSuffix, Path: ".js", CacheControl: "no-cache"},
		},
	}
}

const immutable = "public, max-age=31536000, immutable"

func TestCacheControl(t *testing.T) {
	rules := appRules()

	for _, tc := range []struct {
		name string
		path string
		want string
	}{{
		// The entry carries a content hash like every other file esbuild
		// emits, so the prefix rule covers it and the exact rule that once
		// had to precede that prefix is gone.
		name: "content-addressed entry bundle is immutable",
		path: "dino_bundle/dino_main-A1B2C3D4.js",
		want: immutable,
	}, {
		name: "hashed chunk under the same prefix is immutable",
		path: "dino_bundle/chunk-A1B2C3D4.js",
		want: immutable,
	}, {
		// The .js suffix rule must not steal this: nginx gives the `^~`
		// prefix priority over the regex, and the ordered list has to
		// reproduce that.
		name: "hashed route chunk beats the .js suffix rule",
		path: "dino_bundle/Play-DEADBEEF.js",
		want: immutable,
	}, {
		name: "content-addressed asset is immutable",
		path: "assets/sprite-0F0F0F0F.png",
		want: immutable,
	}, {
		name: "asset css is immutable, not no-cache",
		path: "assets/inline-12345678.css",
		want: immutable,
	}, {
		name: "index.html revalidates",
		path: "index.html",
		want: "no-cache",
	}, {
		name: "stylex sheet revalidates",
		path: "dino_styles.css",
		want: "no-cache",
	}, {
		name: "unmatched file falls back to the default",
		path: "manifest.webmanifest",
		want: "no-cache",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			if got := rules.CacheControl(tc.path); got != tc.want {
				t.Errorf("CacheControl(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestContentType(t *testing.T) {
	for _, tc := range []struct {
		path string
		want string
	}{
		{"index.html", "text/html; charset=utf-8"},
		{"dino_styles.css", "text/css; charset=utf-8"},
		{"dino_bundle/dino_main.js", "text/javascript; charset=utf-8"},
		{"dino_assets.json", "application/json"},
		{"icons/icon.svg", "image/svg+xml"},
		{"icons/icon-192.png", "image/png"},
		{"favicon.ico", "image/x-icon"},
		{"assets/font-ABCDEF01.woff2", "font/woff2"},
		// nginx's bundled mime.types has no .webmanifest entry either — the
		// per-app conf works around it with `default_type`. GCS has no such
		// table at all, so the extension has to be in ours or browsers
		// decline to install the app.
		{"manifest.webmanifest", "application/manifest+json"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			if got := webroot.ContentType(tc.path); got != tc.want {
				t.Errorf("ContentType(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

// An unknown extension must not be guessed at. Serving the wrong type is
// worse than serving the generic one — a mistyped .js is refused by the
// browser's module loader outright.
func TestContentTypeUnknownExtensionIsOctetStream(t *testing.T) {
	if got, want := webroot.ContentType("data/board.bin"), "application/octet-stream"; got != want {
		t.Errorf("ContentType = %q, want %q", got, want)
	}
}
