// Package deb reads files out of Debian binary packages.
//
// A .deb is an ar archive of three members: `debian-binary`, `control.tar.*`
// and `data.tar.*`. Everything the package installs lives in the data member,
// including the copyright file that is Debian's only in-band licence record.
package deb

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/blakesmith/ar"
	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

// maxCopyrightBytes caps how much is read out of the archive. glibc's
// copyright file, the largest in these images, is a few hundred kilobytes.
const maxCopyrightBytes = 8 << 20

var (
	// ErrNoCopyright marks a package that ships no copyright file. Rare, but
	// real: three of the packages in these images have none.
	ErrNoCopyright = errors.New("package ships no copyright file")

	// ErrNoDataArchive marks something that is not a .deb.
	ErrNoDataArchive = errors.New("archive has no data member")
)

// Copyright returns the contents of the package's
// `usr/share/doc/<name>/copyright`.
func Copyright(r io.Reader) ([]byte, error) {
	archive := ar.NewReader(r)
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			return nil, ErrNoDataArchive
		}
		if err != nil {
			return nil, fmt.Errorf("reading ar archive: %w", err)
		}

		// ar pads short names with spaces and marks them with a trailing "/".
		// The control member is also `.tar.*` and comes first, so match the
		// prefix rather than the extension.
		name := strings.TrimRight(header.Name, "/ ")
		if !strings.HasPrefix(name, "data.tar") {
			continue
		}

		data, err := decompress(name, archive)
		if err != nil {
			return nil, err
		}
		return copyrightFrom(data)
	}
}

// decompress unwraps the data member according to its extension. Debian has
// used gzip, xz and zstd over the years and a .deb may legally use any.
func decompress(name string, r io.Reader) (io.Reader, error) {
	switch extension := path.Ext(name); extension {
	case ".tar":
		return r, nil
	case ".gz":
		return gzip.NewReader(r)
	case ".xz":
		return xz.NewReader(r)
	case ".zst":
		decoder, err := zstd.NewReader(r)
		if err != nil {
			return nil, err
		}
		return decoder.IOReadCloser(), nil
	default:
		return nil, fmt.Errorf("unsupported data archive compression %q", extension)
	}
}

func copyrightFrom(r io.Reader) ([]byte, error) {
	archive := tar.NewReader(r)
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			return nil, ErrNoCopyright
		}
		if err != nil {
			return nil, fmt.Errorf("reading data archive: %w", err)
		}
		if isCopyright(header.Name) {
			return io.ReadAll(io.LimitReader(archive, maxCopyrightBytes))
		}
	}
}

// isCopyright matches `usr/share/doc/<package>/copyright`. Entries in a .deb
// are conventionally "./"-prefixed, and the directory is the *source* package
// name, which need not match the binary package — so only the shape is
// matched, not a specific name.
func isCopyright(name string) bool {
	name = strings.TrimPrefix(name, "./")
	return strings.HasPrefix(name, "usr/share/doc/") && path.Base(name) == "copyright"
}
