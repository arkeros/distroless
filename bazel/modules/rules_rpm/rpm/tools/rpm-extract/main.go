// rpm-extract: one .rpm -> (content.tar, header.blob).
//
// Inputs: a single RPM file already SHA256-verified by Bazel via the
// repository_ctx download attribute. This tool additionally verifies the
// in-rpm GPG signature against the consumer-provided key so a tampered RPM
// substituted via the lockfile is also caught (not yet implemented).
//
// Outputs:
//
//	content.tar  tar with the cpio payload, owners stripped to root:root,
//	             mtimes pinned to a deterministic epoch (not yet — current
//	             impl preserves cpio metadata; canonicalization in a later slice).
//	             No rpmdb writes, no %post/%pre scripts executed.
//	header.blob  raw RPM general-header bytes, fed to rpmdb-merge.
package main

import (
	"archive/tar"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sassoftware/go-rpmutils"
	"github.com/sassoftware/go-rpmutils/cpio"
)

func main() {
	var (
		rpm        = flag.String("rpm", "", "path to .rpm file")
		gpgKey     = flag.String("gpg-key", "", "path to ascii-armored gpg public key")
		contentOut = flag.String("content-out", "", "output content tar")
		headerOut  = flag.String("header-out", "", "output rpm header blob")
		_          = flag.String("package", "", "expected package name (sanity check)")
		_          = flag.String("version", "", "expected version (sanity check)")
		_          = flag.String("arch", "", "expected arch (sanity check)")
	)
	flag.Parse()

	if *rpm == "" || *contentOut == "" || *headerOut == "" {
		fmt.Fprintln(os.Stderr, "rpm-extract: --rpm, --content-out, --header-out are required")
		os.Exit(2)
	}
	_ = *gpgKey // TODO: GPG verification against the supplied key

	if err := Extract(*rpm, *contentOut, *headerOut); err != nil {
		fmt.Fprintln(os.Stderr, "rpm-extract:", err)
		os.Exit(1)
	}
}

// Extract reads the RPM at rpmPath and writes a tar of the cpio payload to
// contentTarPath and the raw general-header bytes to headerBlobPath.
func Extract(rpmPath, contentTarPath, headerBlobPath string) error {
	rpmBytes, err := os.ReadFile(rpmPath)
	if err != nil {
		return fmt.Errorf("read rpm: %w", err)
	}

	rpmFile, err := rpmutils.ReadRpm(bytes.NewReader(rpmBytes))
	if err != nil {
		return fmt.Errorf("parse rpm: %w", err)
	}

	hdrRange := rpmFile.Header.GetRange()
	if err := os.WriteFile(headerBlobPath, rpmBytes[hdrRange.Start:hdrRange.End], 0o644); err != nil {
		return fmt.Errorf("write header blob: %w", err)
	}

	payload, err := rpmFile.PayloadReader()
	if err != nil {
		return fmt.Errorf("open payload: %w", err)
	}

	tarFile, err := os.Create(contentTarPath)
	if err != nil {
		return fmt.Errorf("create content tar: %w", err)
	}
	defer tarFile.Close()

	tw := tar.NewWriter(tarFile)
	defer tw.Close()

	for {
		ent, err := payload.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("cpio next: %w", err)
		}
		if shouldStrip(ent.Filename()) {
			// Drain regular-file content so the cpio reader advances. Symlinks
			// and dirs carry no payload.
			if isCpioRegular(ent) {
				if _, err := io.CopyN(io.Discard, payload, int64(ent.Filesize())); err != nil {
					return fmt.Errorf("drain stripped %q: %w", ent.Filename(), err)
				}
			}
			continue
		}
		if err := writeCpioEntryAsTar(tw, ent, payload); err != nil {
			return fmt.Errorf("write tar entry %q: %w", ent.Filename(), err)
		}
	}
	return nil
}

// shouldStrip drops cpio entries whose paths are useless on a distroless
// image. Currently: `/usr/lib/.build-id/**` — the GDB build-id symlink tree
// is only consumed by debugger tooling that distroless images don't ship,
// and (per ADR 0007's cc bring-up) glibc-common's cpio places these symlinks
// before their parent directory, which strict tar extractors reject. Same
// posture as Wolfi/Chainguard distroless and `rpm --excludedocs`.
func shouldStrip(filename string) bool {
	clean := strings.TrimPrefix(filename, "./")
	return clean == "usr/lib/.build-id" || strings.HasPrefix(clean, "usr/lib/.build-id/")
}

func isCpioRegular(ent *cpio.Cpio_newc_header) bool {
	t := ent.Mode() &^ 07777
	return t == 0 || t == cpio.S_ISREG
}

// writeCpioEntryAsTar translates one cpio entry to a tar entry. Paths are
// preserved verbatim (cpio stores them with a leading "./" — kept so callers
// can address /usr/share/zoneinfo/UTC as ./usr/share/zoneinfo/UTC).
//
// Type-vs-perm bits: the cpio Mode() word packs both — type bits above 07777,
// permission bits below. Use `mode &^ 07777` to isolate the file-type value
// and compare against the S_IS* constants directly.
func writeCpioEntryAsTar(tw *tar.Writer, ent *cpio.Cpio_newc_header, payload *cpio.Reader) error {
	mode := ent.Mode()
	typeflag := byte(tar.TypeReg)
	var linkname string
	switch mode &^ 07777 {
	case cpio.S_ISDIR:
		typeflag = tar.TypeDir
	case cpio.S_ISLNK:
		typeflag = tar.TypeSymlink
		buf := make([]byte, ent.Filesize())
		if _, err := io.ReadFull(payload, buf); err != nil {
			return fmt.Errorf("read symlink target: %w", err)
		}
		linkname = string(buf)
	case cpio.S_ISCHR, cpio.S_ISBLK, cpio.S_ISFIFO, cpio.S_ISSOCK:
		// Device nodes / fifos / sockets aren't in our allow-list for now.
		return nil
	}

	hdr := &tar.Header{
		Name:     ent.Filename(),
		Mode:     int64(mode & 07777),
		Size:     int64(ent.Filesize()),
		Typeflag: typeflag,
		Linkname: linkname,
	}
	if typeflag != tar.TypeReg {
		hdr.Size = 0
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if typeflag == tar.TypeReg {
		if _, err := io.Copy(tw, payload); err != nil {
			return err
		}
	}
	return nil
}
