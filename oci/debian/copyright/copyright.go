// Package copyright reads the licence a Debian package declares for itself.
//
// Debian's `Packages` index carries no licence field, so a package's
// `usr/share/doc/<name>/copyright` file is the only in-band source. Since
// DEP-5 that file is often machine-readable; before it, and still for some
// packages, it is prose.
package copyright

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

// maxCopyrightBytes caps how much is read. Real copyright files run to tens of
// kilobytes; glibc's, the largest here, is well under a megabyte.
const maxCopyrightBytes = 8 << 20

var (
	// ErrNotMachineReadable marks a copyright file that predates DEP-5 or
	// never adopted it. Roughly one Debian package in seven.
	ErrNotMachineReadable = errors.New("copyright file is not in machine-readable DEP-5 format")

	// ErrNoOverallLicense marks a DEP-5 file that licenses individual files
	// but never the package as a whole.
	ErrNoOverallLicense = errors.New("copyright file declares no package-wide license")
)

// License is what a Debian package declares about itself.
//
// Both fields matter because they are not interchangeable: CycloneDX's
// `license.id` must be a valid SPDX identifier, while `license.name` is free
// text. A Debian short name with no SPDX equivalent — `BSD-variant`,
// `public-domain` — is real information that must not be discarded just
// because it cannot be expressed as an id.
type License struct {
	// Name is the Debian short name, e.g. "GPL-2+". Always set.
	Name string
	// SPDX is the corresponding SPDX identifier, or empty when Debian's name
	// has no equivalent. Empty means "no SPDX id exists for this", never
	// "no licence".
	SPDX string
}

// Parse reads a DEP-5 copyright file and returns the licence declared by its
// `Files: *` stanza — the licence of the package as a whole.
//
// This is a declared licence, not an audited one. A package can declare one
// licence overall and still ship files under others, which is why only the
// package-wide stanza is read: aggregating per-file stanzas would produce a
// confident answer that nobody checked.
func Parse(r io.Reader) (License, error) {
	document, err := io.ReadAll(io.LimitReader(r, maxCopyrightBytes))
	if err != nil {
		return License{}, fmt.Errorf("reading copyright file: %w", err)
	}

	// DEP-5 requires the header paragraph to open with `Format:`. Everything
	// else is prose, and guessing at prose is how you publish a licence claim
	// that is wrong.
	if !bytes.HasPrefix(document, []byte("Format:")) {
		return License{}, ErrNotMachineReadable
	}

	for _, paragraph := range strings.Split(string(document), "\n\n") {
		fields := fields(paragraph)
		if !coversWholePackage(fields["files"].value()) {
			continue
		}
		name := fields["license"].name()
		if name == "" {
			continue
		}
		return License{Name: name, SPDX: spdx[name]}, nil
	}
	return License{}, ErrNoOverallLicense
}

// coversWholePackage reports whether a `Files:` list applies to the entire
// package, which is true when `*` is among its patterns.
//
// Not "is exactly `*`": `sed` writes `Files: *` and then redundantly names
// `doc/local.mk` under the same stanza. The stanza still covers everything, so
// requiring `*` to stand alone would drop a license we do know.
func coversWholePackage(files string) bool {
	return slices.Contains(strings.Fields(files), "*")
}

// field is one DEP-5 field: whatever followed the `Key:` on its own line, plus
// any indented continuation lines beneath it.
//
// The two are kept apart because the two fields here want different halves.
// `Files:` may spell its value either way — `Files: *` and `Files:\n *` mean
// the same thing — so it needs them folded together. `License:` puts the short
// name first and then the full licence text underneath, so it needs only the
// first line.
type field struct {
	inline       string
	continuation []string
}

// value is the whole field with continuations folded in.
func (f field) value() string {
	parts := f.continuation
	if f.inline != "" {
		parts = append([]string{f.inline}, f.continuation...)
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

// name is the field's first line, wherever it was written.
func (f field) name() string {
	if f.inline != "" {
		return f.inline
	}
	if len(f.continuation) > 0 {
		return f.continuation[0]
	}
	return ""
}

// fields reads one paragraph into its DEP-5 fields.
func fields(paragraph string) map[string]field {
	values := make(map[string]field)
	key := ""
	for _, line := range strings.Split(paragraph, "\n") {
		if line == "" {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			if key != "" {
				entry := values[key]
				entry.continuation = append(entry.continuation, strings.TrimSpace(line))
				values[key] = entry
			}
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			key = ""
			continue
		}
		key = strings.ToLower(strings.TrimSpace(name))
		values[key] = field{inline: strings.TrimSpace(value)}
	}
	return values
}

// spdx maps Debian's licence short names onto SPDX identifiers.
//
// Deliberately partial. Names with no SPDX equivalent — `BSD-variant`,
// `public-domain`, `Artistic-dist` — are absent rather than approximated: a
// wrong identifier in a signed attestation is worse than an honest blank,
// because downstream compliance tooling will believe it.
var spdx = map[string]string{
	"Apache-2.0":   "Apache-2.0",
	"Artistic":     "Artistic-1.0",
	"BSD-2-clause": "BSD-2-Clause",
	"BSD-2-Clause": "BSD-2-Clause",
	"BSD-3-clause": "BSD-3-Clause",
	"BSD-3-Clause": "BSD-3-Clause",
	"BSD-4-clause": "BSD-4-Clause",
	"0BSD":         "0BSD",
	"CC0-1.0":      "CC0-1.0",
	// Debian's canonical name for the MIT licence.
	"Expat":     "MIT",
	"MIT/X11":   "MIT",
	"GFDL-1.3+": "GFDL-1.3-or-later",
	"GPL-2":     "GPL-2.0-only",
	// Debian also spells the "or later" forms with the SPDX version number
	// but its own `+` suffix.
	"GPL-2.0+":  "GPL-2.0-or-later",
	"GPL-3.0+":  "GPL-3.0-or-later",
	"LGPL-2.0+": "LGPL-2.0-or-later",
	"LGPL-3.0+": "LGPL-3.0-or-later",
	// Some packages declare an SPDX identifier directly.
	"GPL-2.0-only":      "GPL-2.0-only",
	"GPL-2.0-or-later":  "GPL-2.0-or-later",
	"GPL-3.0-only":      "GPL-3.0-only",
	"GPL-3.0-or-later":  "GPL-3.0-or-later",
	"LGPL-2.1-only":     "LGPL-2.1-only",
	"LGPL-2.1-or-later": "LGPL-2.1-or-later",
	"GPL-2+":            "GPL-2.0-or-later",
	"GPL-3":             "GPL-3.0-only",
	"GPL-3+":            "GPL-3.0-or-later",
	"ISC":               "ISC",
	"LGPL-2":            "LGPL-2.0-only",
	"LGPL-2+":           "LGPL-2.0-or-later",
	"LGPL-2.1":          "LGPL-2.1-only",
	"LGPL-2.1+":         "LGPL-2.1-or-later",
	"LGPL-3":            "LGPL-3.0-only",
	"LGPL-3+":           "LGPL-3.0-or-later",
	"MIT":               "MIT",
	"MPL-2.0":           "MPL-2.0",
	"Zlib":              "Zlib",
}
