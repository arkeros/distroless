// rpm-extract: one .rpm -> (content.tar, header.blob).
//
// Inputs: a single RPM file already SHA256-verified by Bazel via the
// repository_ctx download attribute. This tool additionally verifies the
// in-rpm GPG signature against the consumer-provided key so a tampered RPM
// substituted past the sha256 boundary is also caught.
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

	"github.com/arkeros/senku/bazel/modules/rules_rpm/rpm/tools/internal/keyring"
	"github.com/sassoftware/go-rpmutils"
	"github.com/sassoftware/go-rpmutils/cpio"
)

func main() {
	var (
		rpm         = flag.String("rpm", "", "path to .rpm file")
		gpgKey      = flag.String("gpg-key", "", "path to ascii-armored gpg public key")
		contentOut  = flag.String("content-out", "", "output content tar")
		headerOut   = flag.String("header-out", "", "output rpm header blob")
		wantPackage = flag.String("package", "", "expected package name (sanity check)")
		wantVersion = flag.String("version", "", "expected version-release (sanity check)")
		wantArch    = flag.String("arch", "", "expected arch (sanity check)")
	)
	flag.Parse()

	if *rpm == "" || *contentOut == "" || *headerOut == "" {
		fmt.Fprintln(os.Stderr, "rpm-extract: --rpm, --content-out, --header-out are required")
		os.Exit(2)
	}

	if err := verifyRpmSignature(*rpm, *gpgKey); err != nil {
		fmt.Fprintln(os.Stderr, "rpm-extract:", err)
		os.Exit(1)
	}

	if err := validateRpmMetadata(*rpm, *wantPackage, *wantVersion, *wantArch); err != nil {
		fmt.Fprintln(os.Stderr, "rpm-extract:", err)
		os.Exit(1)
	}

	if err := Extract(*rpm, *contentOut, *headerOut); err != nil {
		fmt.Fprintln(os.Stderr, "rpm-extract:", err)
		os.Exit(1)
	}
}

// verifyRpmSignature checks the in-rpm PGP signature against the supplied
// ASCII-armored keyring. The keyring file may contain multiple armor blocks
// (Hummingbird's hummingbird-release.pgp ships three keys for key-rotation
// continuity); the shared keyring package decodes each block independently
// and accumulates into one EntityList passed to rpmutils.Verify, which
// walks the signature headers and fails if none match.
//
// Empty --gpg-key skips verification so the binary stays usable as a one-off
// CLI; rpm_package always passes the flag.
func verifyRpmSignature(rpmPath, keyPath string) error {
	if keyPath == "" {
		return nil
	}
	keys, err := keyring.ReadMultiBlock(keyPath)
	if err != nil {
		return fmt.Errorf("load gpg key: %w", err)
	}
	rf, err := os.Open(rpmPath)
	if err != nil {
		return fmt.Errorf("open rpm for signature check: %w", err)
	}
	defer rf.Close()
	if _, _, err := rpmutils.Verify(rf, keys); err != nil {
		return fmt.Errorf("gpg signature verification failed: %w", err)
	}
	return nil
}

// validateRpmMetadata enforces the --package/--version/--arch sanity checks
// against the actual RPM header so a lockfile entry whose name/version/arch
// drifts away from the on-disk .rpm bytes (e.g. URL rewrite, cache poisoning
// past the sha256 boundary) is caught at extract time. Empty expected values
// are skipped — the rpm_package Bazel rule always passes all three, but the
// binary stays usable as a one-off CLI without them.
//
// `--version` carries the lockfile-stored shape `[<epoch>:]<version>-<release>`
// (epoch omitted when zero). Same split convention as `_split_epoch` in
// install.bzl — keep them in lockstep if the encoding ever changes.
func validateRpmMetadata(rpmPath, wantPkg, wantVer, wantArch string) error {
	if wantPkg == "" && wantVer == "" && wantArch == "" {
		return nil
	}
	f, err := os.Open(rpmPath)
	if err != nil {
		return fmt.Errorf("open rpm for metadata check: %w", err)
	}
	defer f.Close()
	rpmFile, err := rpmutils.ReadRpm(f)
	if err != nil {
		return fmt.Errorf("parse rpm for metadata check: %w", err)
	}
	nevra, err := rpmFile.Header.GetNEVRA()
	if err != nil {
		return fmt.Errorf("read NEVRA: %w", err)
	}
	if wantPkg != "" && nevra.Name != wantPkg {
		return fmt.Errorf("package mismatch: rpm header says %q, --package=%q", nevra.Name, wantPkg)
	}
	if wantArch != "" && nevra.Arch != wantArch {
		return fmt.Errorf("arch mismatch: rpm header says %q, --arch=%q", nevra.Arch, wantArch)
	}
	if wantVer != "" {
		got := rpmEVR(nevra)
		if got != wantVer {
			return fmt.Errorf("version mismatch: rpm header says %q, --version=%q", got, wantVer)
		}
	}
	return nil
}

// rpmEVR renders an RPM NEVRA's epoch-version-release in the lockfile shape:
// "<epoch>:<version>-<release>" when epoch is non-zero, "<version>-<release>"
// otherwise. Must agree with install.bzl's `_split_epoch` partition rule.
func rpmEVR(n *rpmutils.NEVRA) string {
	if n.Epoch != "" && n.Epoch != "0" {
		return n.Epoch + ":" + n.Version + "-" + n.Release
	}
	return n.Version + "-" + n.Release
}

// Extract reads the RPM at rpmPath and writes a tar of the cpio payload to
// contentTarPath and the raw general-header bytes to headerBlobPath.
func Extract(rpmPath, contentTarPath, headerBlobPath string) (err error) {
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
	defer func() {
		if closeErr := tarFile.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close content tar: %w", closeErr)
		}
	}()

	tw := tar.NewWriter(tarFile)
	defer func() {
		if closeErr := tw.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close tar writer: %w", closeErr)
		}
	}()

	for {
		ent, err := payload.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("cpio next: %w", err)
		}
		name := ent.Filename()
		if shouldStrip(name) {
			// Drain regular-file content so the cpio reader advances. Symlinks
			// and dirs carry no payload.
			if isCpioRegular(ent) {
				if _, err := io.CopyN(io.Discard, payload, int64(ent.Filesize())); err != nil {
					return fmt.Errorf("drain stripped %q: %w", name, err)
				}
			}
			continue
		}
		rewritten, drop := mergedUsr(name)
		if drop {
			continue
		}
		if err := writeCpioEntryAsTar(tw, ent, payload, rewritten); err != nil {
			return fmt.Errorf("write tar entry %q: %w", name, err)
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

// mergedUsr rewrites legacy root paths (/lib, /lib64, /bin, /sbin) onto
// their /usr/* counterparts. RHEL's `filesystem` rpm ships the same as
// runtime symlinks; senku deliberately doesn't pull `filesystem` (per ADR
// 0007), so the merge happens at extract time. Mirrors Debian's
// `mergedusr = True` behaviour in rules_distroless.
//
// Symlinks for these prefixes (e.g. `./lib64 -> usr/lib64`) are
// re-synthesised by the static base — keeping per-package emission of
// them would collide with the base layer's symlink.
//
// Companion: mergedUsrLink rewrites symlink targets (the right-hand
// side of `->`) so an absolute target like `/lib/foo` doesn't end up
// referencing the legacy root when the per-package tar is consumed in
// isolation. Empirically (as of 2026-05-18) no package in the current
// Hummingbird closure — glibc, glibc-common, libgcc, libstdc++,
// openssl-libs, bash, ca-certificates, mailcap, tzdata — ships an
// absolute symlink into /lib*, /bin, or /sbin (their absolute targets
// land in /etc/pki, /etc/crypto-policies, or are relative). The
// rewrite is defensive against future packages and keeps the tar
// internally consistent without relying on the base layer's root
// symlinks (oci/distroless/common:usrmerge_symlinks_hummingbird).
func mergedUsr(filename string) (rewritten string, drop bool) {
	clean := strings.TrimPrefix(filename, "./")
	for _, prefix := range []string{"lib64", "lib", "bin", "sbin"} {
		// Drop the legacy root-symlink entries — base layer owns them.
		if clean == prefix {
			return "", true
		}
		if rest, ok := strings.CutPrefix(clean, prefix+"/"); ok {
			return "./usr/" + prefix + "/" + rest, false
		}
	}
	return filename, false
}

// mergedUsrLink rewrites a symlink target so absolute paths into the
// legacy roots (/lib, /lib64, /bin, /sbin) point under /usr/. Relative
// targets are returned verbatim — they're already location-relative
// and the path-side rewrite of the symlink's own location preserves
// the right resolution. Absolute targets outside the legacy roots
// (e.g. /etc/pki/..., /opt/...) are also returned verbatim.
//
// /lib64 is listed before /lib so a target of `/lib64/foo` matches the
// longer prefix first; the `prefix == target || prefix+"/" matches`
// shape would handle this anyway but ordering longest-first makes the
// intent obvious.
func mergedUsrLink(target string) string {
	if !strings.HasPrefix(target, "/") {
		return target
	}
	for _, prefix := range []string{"/lib64", "/lib", "/bin", "/sbin"} {
		if target == prefix || strings.HasPrefix(target, prefix+"/") {
			return "/usr" + target
		}
	}
	return target
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
func writeCpioEntryAsTar(tw *tar.Writer, ent *cpio.Cpio_newc_header, payload *cpio.Reader, name string) error {
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
		linkname = mergedUsrLink(string(buf))
	case cpio.S_ISCHR, cpio.S_ISBLK, cpio.S_ISFIFO, cpio.S_ISSOCK:
		// Device nodes / fifos / sockets aren't in our allow-list for now.
		return nil
	}

	hdr := &tar.Header{
		Name:     name,
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
