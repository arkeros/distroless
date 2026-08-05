// Package webroot classifies the files of a built frontend for publication
// to an object store.
//
// A bucket has no equivalent of nginx's `location` blocks: every response
// header is metadata stamped onto the object when it is written. The cache
// policy an app declares therefore has to be evaluated here, at upload time,
// against the same file paths nginx would have matched at request time.
//
// The rules themselves are not defined in Go. They are declared once in
// Starlark (`//devtools/build/react_component:cache.bzl`), which renders both
// the nginx `default.conf` for the container image and the JSON this package
// reads — so the two origins cannot come to disagree.
package webroot

import (
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
)

// Match is how a Rule's Path is compared against an object's name.
type Match string

const (
	// MatchExact matches one object name.
	MatchExact Match = "exact"
	// MatchPrefix matches every object under a path prefix.
	MatchPrefix Match = "prefix"
	// MatchSuffix matches by file extension.
	MatchSuffix Match = "suffix"
)

// Rule assigns a Cache-Control value to the object names it matches.
type Rule struct {
	Match        Match  `json:"match"`
	Path         string `json:"path"`
	CacheControl string `json:"cache_control"`
}

func (r Rule) matches(name string) bool {
	switch r.Match {
	case MatchExact:
		return name == r.Path
	case MatchPrefix:
		return strings.HasPrefix(name, r.Path)
	case MatchSuffix:
		return strings.HasSuffix(name, r.Path)
	}
	return false
}

// Rules is an app's full cache policy.
//
// Order is significant and first match wins. nginx resolves overlapping
// locations by an implicit precedence — an `=` exact beats a `^~` prefix
// beats a regex — which a reader has to know to predict. Stating the order
// explicitly means the same policy reads the same way in both origins, and
// the ordering is something a test can pin (see TestCacheControl).
type Rules struct {
	Rules []Rule `json:"rules"`
	// DefaultCacheControl applies to objects no rule matches.
	DefaultCacheControl string `json:"default_cache_control"`
}

// CacheControl is the header to stamp on the object named by name, which is
// a bucket-relative path such as "dino_bundle/chunk-A1B2C3D4.js".
func (r Rules) CacheControl(name string) string {
	for _, rule := range r.Rules {
		if rule.matches(name) {
			return rule.CacheControl
		}
	}
	return r.DefaultCacheControl
}

// ParseRules reads the JSON rendered by `cache.bzl`.
func ParseRules(src io.Reader) (Rules, error) {
	var rules Rules
	dec := json.NewDecoder(src)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rules); err != nil {
		return Rules{}, fmt.Errorf("parsing cache rules: %w", err)
	}
	if rules.DefaultCacheControl == "" {
		return Rules{}, fmt.Errorf("cache rules declare no default_cache_control")
	}
	for i, rule := range rules.Rules {
		switch rule.Match {
		case MatchExact, MatchPrefix, MatchSuffix:
		default:
			return Rules{}, fmt.Errorf("cache rule %d: unknown match kind %q", i, rule.Match)
		}
	}
	return rules, nil
}

// contentTypes is deliberately a closed table rather than a call to
// `mime.TypeByExtension`. That function consults /etc/mime.types when one
// exists, so its answers depend on the machine running the upload — the
// same build would stamp different types on different hosts. Every
// extension a react_app can emit is listed here; anything else is an
// explicit failure to classify, not a guess.
var contentTypes = map[string]string{
	".css":         "text/css; charset=utf-8",
	".html":        "text/html; charset=utf-8",
	".ico":         "image/x-icon",
	".jpg":         "image/jpeg",
	".js":          "text/javascript; charset=utf-8",
	".json":        "application/json",
	".png":         "image/png",
	".svg":         "image/svg+xml",
	".txt":         "text/plain; charset=utf-8",
	".webmanifest": "application/manifest+json",
	".webp":        "image/webp",
	".woff2":       "font/woff2",
}

// DefaultContentType is served for extensions the table does not cover.
// Generic is the safe answer: a browser will download an octet-stream it
// cannot render, where a confidently wrong type makes it refuse content it
// could have used — a mistyped module script is rejected outright.
const DefaultContentType = "application/octet-stream"

// ContentType is the Content-Type to stamp on the object named by name.
func ContentType(name string) string {
	if ct, ok := contentTypes[strings.ToLower(path.Ext(name))]; ok {
		return ct
	}
	return DefaultContentType
}
