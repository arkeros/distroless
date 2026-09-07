// Package tarballs pins upstream prebuilt runtime tarballs (nodejs.org,
// Adoptium Temurin) in a JSON lockfile that the `tarballs` module
// extension in //bazel/tarballs:extensions.bzl turns into http_archive repos.
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

// Archive is one http_archive: the bytes Bazel downloads and the
// directory it strips.
type Archive struct {
	URL         string `json:"url"`
	SHA256      string `json:"sha256"`
	StripPrefix string `json:"strip_prefix"`
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
