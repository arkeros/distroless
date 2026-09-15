// Package tarballs pins upstream prebuilt runtimes (nodejs.org, Adoptium
// Temurin, envoyproxy's release assets) in a JSON lockfile that the
// `tarballs` module extension in //bazel/tarballs:extensions.bzl turns into
// repos — an http_archive per archive entry, a downloaded executable per
// bare-file entry.
//
// Each lockfile covers one upstream (`source`) and lists the release
// lines the images ship, one `Line` per major. Which majors appear is a
// policy decision made by hand; `Update` only moves the ones already
// listed to their newest release.
package tarballs

import (
	"encoding/json"
	"fmt"
	"os"
)

// Archive is one upstream download Bazel pins. Exactly one of StripPrefix
// and File decides what becomes of the bytes: StripPrefix unpacks them as
// an archive and drops that leading directory, File keeps them as a single
// executable of that name. The extension fails on an entry that sets both
// or neither.
type Archive struct {
	URL         string `json:"url"`
	SHA256      string `json:"sha256"`
	StripPrefix string `json:"strip_prefix,omitempty"`
	// File is the name to download to, for an upstream that publishes a
	// bare binary rather than an archive (envoy's release assets).
	File string `json:"file,omitempty"`
}

// Line is one release line (a major version) and its per-variant archives,
// keyed by the Bazel repo name the image BUILD references.
type Line struct {
	Major   string `json:"major"`
	Version string `json:"version"`
	// Build is the upstream build counter for sources that have one
	// (Temurin's `+N`); empty for nodejs.
	Build    string             `json:"build,omitempty"`
	Archives map[string]Archive `json:"archives"`
}

// Lock is the on-disk lockfile.
type Lock struct {
	SchemaVersion int    `json:"schema_version"`
	Source        string `json:"source"`
	Lines         []Line `json:"lines"`
}

const schemaVersion = 1

// ReadLock parses the lockfile at path.
func ReadLock(path string) (*Lock, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var lock Lock
	if err := json.Unmarshal(raw, &lock); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if lock.SchemaVersion != schemaVersion {
		return nil, fmt.Errorf("%s: unsupported schema_version %d (want %d)", path, lock.SchemaVersion, schemaVersion)
	}
	return &lock, nil
}

// Write serialises the lock to path, pretty-printed with a trailing newline
// so the file diffs cleanly.
func (l *Lock) Write(path string) error {
	buf, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(buf, '\n'), 0o644)
}
