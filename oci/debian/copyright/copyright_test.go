package copyright_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/arkeros/distroless/oci/debian/copyright"
)

// Trimmed from libc6's actual copyright file.
const libc6 = `Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/
Upstream-Name: GNU C Library
Source: https://sourceware.org/git/glibc.git

Files: *
Copyright: 1991-2026 Free Software Foundation, Inc.
License: LGPL-2.1+

Files:
 nptl/default-sched.h
 nss/nss_db/db-initgroups.c
Copyright: 2011-2026 Free Software Foundation, Inc.
License: GPL-2+

License: LGPL-2.1+
 This program is free software; you can redistribute it and/or modify
 it under the terms of the GNU Lesser General Public License.
`

func TestParseTakesTheLicenseOfTheWholePackage(t *testing.T) {
	got, err := copyright.Parse(strings.NewReader(libc6))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// The `Files: *` stanza, not the per-file GPL-2+ stanza that follows it.
	if got.Name != "LGPL-2.1+" {
		t.Errorf("Name = %q, want LGPL-2.1+", got.Name)
	}
	if got.SPDX != "LGPL-2.1-or-later" {
		t.Errorf("SPDX = %q, want LGPL-2.1-or-later", got.SPDX)
	}
}

// Debian short names are not SPDX identifiers, and CycloneDX distinguishes an
// `id` (which must be SPDX) from a free-text `name`.
func TestParseMapsDebianShortNamesToSPDX(t *testing.T) {
	tests := []struct{ debian, spdx string }{
		{"GPL-2+", "GPL-2.0-or-later"},
		{"GPL-2", "GPL-2.0-only"},
		{"GPL-3+", "GPL-3.0-or-later"},
		{"LGPL-2.1+", "LGPL-2.1-or-later"},
		{"BSD-2-clause", "BSD-2-Clause"},
		{"BSD-3-Clause", "BSD-3-Clause"},
		{"MIT", "MIT"},
		{"Apache-2.0", "Apache-2.0"},
		// Debian's own name for the MIT licence, and the one glibc-adjacent
		// packages use. Not mapping it loses a licence we do know.
		{"Expat", "MIT"},
		{"MIT/X11", "MIT"},
		// Already a valid SPDX identifier; some packages declare them directly.
		{"0BSD", "0BSD"},
		// Debian mixes its own `+` suffix with SPDX version numbers.
		{"GPL-2.0+", "GPL-2.0-or-later"},
		{"GPL-2.0-only", "GPL-2.0-only"},
		// No SPDX equivalent — must not be invented.
		{"BSD-variant", ""},
		{"public-domain", ""},
		{"ad-hoc", ""},
		// A disjunction is an SPDX *expression*, not an identifier, so it does
		// not belong in `license.id`.
		{"BSD-3-clause or GPL-2", ""},
	}
	for _, tt := range tests {
		t.Run(tt.debian, func(t *testing.T) {
			document := "Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/\n\nFiles: *\nLicense: " + tt.debian + "\n"
			got, err := copyright.Parse(strings.NewReader(document))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got.SPDX != tt.spdx {
				t.Errorf("SPDX = %q, want %q", got.SPDX, tt.spdx)
			}
			// The Debian name always survives, so an unmapped license is still
			// reported rather than silently dropped.
			if got.Name != tt.debian {
				t.Errorf("Name = %q, want %q", got.Name, tt.debian)
			}
		})
	}
}

// A `License:` line may be followed by the licence text, indented. Only the
// first line names the licence.
func TestParseIgnoresLicenceText(t *testing.T) {
	document := `Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/

Files: *
License: GPL-2+
 This package is free software; you can redistribute it and/or
 modify it under the terms of the GNU General Public License.
`
	got, err := copyright.Parse(strings.NewReader(document))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Name != "GPL-2+" {
		t.Errorf("Name = %q, want GPL-2+ without the licence text", got.Name)
	}
}

// 6 of the 44 packages in these images have a free-form copyright file. Saying
// so is the point — a blank licence and an undeterminable one must not look
// the same.
func TestParseRejectsFreeFormCopyright(t *testing.T) {
	document := "This package was debianized by someone in 1998.\n\nIt is licensed under the GPL, see /usr/share/common-licenses/GPL.\n"

	if _, err := copyright.Parse(strings.NewReader(document)); !errors.Is(err, copyright.ErrNotMachineReadable) {
		t.Errorf("err = %v, want ErrNotMachineReadable", err)
	}
}

// DEP-5 but with no package-wide stanza: also undeterminable, for a different
// reason.
func TestParseRejectsMissingOverallStanza(t *testing.T) {
	document := `Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/

Files: src/foo.c
License: MIT
`
	if _, err := copyright.Parse(strings.NewReader(document)); !errors.Is(err, copyright.ErrNoOverallLicense) {
		t.Errorf("err = %v, want ErrNoOverallLicense", err)
	}
}

// DEP-5 fields may carry their value on indented continuation lines instead of
// on the `Key:` line. `grep` and `libacl1` both write the package-wide stanza
// this way, and reading only the first line silently reports them as having no
// license at all.
func TestParseHandlesMultiLineFilesField(t *testing.T) {
	document := `Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/

Files:
 *
Copyright:
 1992-2012 Free Software Foundation, Inc.
License: GPL-3+

Files:
 lib/savedir.c
License: GPL-2+
`
	got, err := copyright.Parse(strings.NewReader(document))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Name != "GPL-3+" {
		t.Errorf("Name = %q, want GPL-3+", got.Name)
	}
}

// A multi-line file list that is not `*` is still per-file, not package-wide.
func TestParseRejectsMultiLineFileList(t *testing.T) {
	document := `Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/

Files:
 src/foo.c
 src/bar.c
License: MIT
`
	if _, err := copyright.Parse(strings.NewReader(document)); !errors.Is(err, copyright.ErrNoOverallLicense) {
		t.Errorf("err = %v, want ErrNoOverallLicense", err)
	}
}

// The license name may itself sit on a continuation line, with the license
// text following it.
func TestParseHandlesMultiLineLicenseField(t *testing.T) {
	document := `Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/

Files: *
License:
 GPL-2+
 This program is free software; you can redistribute it.
`
	got, err := copyright.Parse(strings.NewReader(document))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Name != "GPL-2+" {
		t.Errorf("Name = %q, want GPL-2+", got.Name)
	}
}

// `sed` writes its package-wide stanza as `Files: *` followed by a redundant
// explicit path. The stanza still covers everything, so what matters is that
// `*` is among the patterns, not that it stands alone.
func TestParseAcceptsWildcardAlongsideOtherPatterns(t *testing.T) {
	document := `Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/

Files: *
 doc/local.mk
Copyright: 1989-2022 Free Software Foundation, Inc.
License: GPL-3+
`
	got, err := copyright.Parse(strings.NewReader(document))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Name != "GPL-3+" {
		t.Errorf("Name = %q, want GPL-3+", got.Name)
	}
}
