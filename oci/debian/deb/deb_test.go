package deb_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"testing"
	"time"

	"github.com/blakesmith/ar"

	"github.com/arkeros/distroless/oci/debian/deb"
)

// build assembles a .deb: an ar archive whose data member is a tarball. Real
// packages use .xz; gzip keeps the fixture to the standard library, and the
// code path being exercised — ar member lookup, then tar walk — is the same.
func build(t *testing.T, entries map[string]string) []byte {
	t.Helper()

	var tarball bytes.Buffer
	zipped := gzip.NewWriter(&tarball)
	archive := tar.NewWriter(zipped)
	for name, body := range entries {
		if err := archive.WriteHeader(&tar.Header{
			Name: name, Mode: 0o644, Size: int64(len(body)), ModTime: time.Unix(0, 0),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := archive.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zipped.Close(); err != nil {
		t.Fatal(err)
	}

	var pkg bytes.Buffer
	writer := ar.NewWriter(&pkg)
	if err := writer.WriteGlobalHeader(); err != nil {
		t.Fatal(err)
	}
	members := []struct {
		name string
		body []byte
	}{
		{"debian-binary", []byte("2.0\n")},
		{"control.tar.gz", []byte("ignored")},
		{"data.tar.gz", tarball.Bytes()},
	}
	for _, m := range members {
		if err := writer.WriteHeader(&ar.Header{
			Name: m.name, Size: int64(len(m.body)), Mode: 0o644, ModTime: time.Unix(0, 0),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(m.body); err != nil {
			t.Fatal(err)
		}
	}
	return pkg.Bytes()
}

func TestCopyrightReadsTheFileOutOfTheDataArchive(t *testing.T) {
	// Tar entries in a .deb are conventionally "./"-prefixed.
	pkg := build(t, map[string]string{
		"./usr/share/doc/libc6/copyright": "Format: 1.0\nLicense: LGPL-2.1+\n",
		"./usr/lib/libc.so.6":             "binary",
	})

	got, err := deb.Copyright(bytes.NewReader(pkg))
	if err != nil {
		t.Fatalf("Copyright: %v", err)
	}
	if !bytes.Contains(got, []byte("LGPL-2.1+")) {
		t.Errorf("got %q, want the copyright file contents", got)
	}
}

// Three of the packages in these images ship no copyright file at all, and
// that has to be reported rather than look like an empty licence.
func TestCopyrightReportsAbsence(t *testing.T) {
	pkg := build(t, map[string]string{"./usr/lib/libc.so.6": "binary"})

	if _, err := deb.Copyright(bytes.NewReader(pkg)); !errors.Is(err, deb.ErrNoCopyright) {
		t.Errorf("err = %v, want ErrNoCopyright", err)
	}
}

// The control member also ends in .tar.gz and comes first; picking it would
// silently find nothing.
func TestCopyrightIgnoresTheControlArchive(t *testing.T) {
	pkg := build(t, map[string]string{"./usr/share/doc/bash/copyright": "Format: 1.0\nLicense: GPL-3+\n"})

	got, err := deb.Copyright(bytes.NewReader(pkg))
	if err != nil {
		t.Fatalf("Copyright: %v", err)
	}
	if !bytes.Contains(got, []byte("GPL-3+")) {
		t.Errorf("got %q, want the data archive's copyright", got)
	}
}
