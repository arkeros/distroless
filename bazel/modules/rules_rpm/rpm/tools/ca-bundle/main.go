// ca-bundle: the ca-certificates rpm's trust source -> the PEM bundle that
// `update-ca-trust` would have written at install time.
//
// RHEL's ca-certificates ships its roots as one p11-kit trust file
// (/usr/share/pki/ca-trust-source/ca-bundle.trust.p11-kit) and generates
// every consumer-facing form of it — /etc/pki/ca-trust/extracted/pem/
// tls-ca-bundle.pem above all — from a %post script. rpm-extract runs no
// scripts, so an image composed from the rpm alone has the symlinks
// (/etc/ssl/certs/ca-certificates.crt among them) and nothing behind them:
// every TLS verification fails with "unable to get local issuer
// certificate".
//
// This tool is the part of `trust extract --filter=ca-anchors --purpose
// server-auth --format=pem-bundle` those symlinks need. A certificate is
// written when its object is `trusted: true`, is not `x-distrusted`, and
// its extended-key-usage object, if p11-kit attaches one, allows server
// authentication. Nothing else: no OpenSSL "TRUSTED CERTIFICATE" form, no
// Java keystore, no per-hash directory.
//
// One rule goes further than the distro's bundle. Mozilla marks some roots
// `nss-server-distrust-after`: leaves issued after that date are not
// trusted, while earlier ones stay valid until they expire. NSS and Chrome
// enforce it against the leaf's notBefore at verification time; a PEM
// bundle has no field for it, so `trust extract` keeps those roots and so
// does RHEL's tls-ca-bundle.pem. This bundle keeps them only until no leaf
// a browser would accept can still chain to them: the date plus the
// 398-day maximum lifetime of a public TLS certificate, measured against
// the build time, which is the lockfile's snapshot so the output is
// reproducible. Ten roots carried such a date at the time of writing, all
// past that point.
//
// Output is a tar (root-owned, epoch mtime, USTAR) carrying the bundle and
// the two /etc/pki/tls symlinks OpenSSL's compiled-in default paths
// resolve, ready for `flatten` beside the rpm's own content.
package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const (
	trustSourcePath = "./usr/share/pki/ca-trust-source/ca-bundle.trust.p11-kit"
	bundlePath      = "./etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem"
	bundleTarget    = "/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem"
)

func main() {
	var (
		contentTar = flag.String("content-tar", "", "content tar of the ca-certificates rpm, as rpm-extract writes it")
		out        = flag.String("out", "", "output tar")
		nowFile    = flag.String("now-file", "", "file holding the build time as RFC 3339, the lockfile's snapshot; distrust dates are measured against it")
	)
	flag.Parse()
	if *contentTar == "" || *out == "" || *nowFile == "" {
		fmt.Fprintln(os.Stderr, "ca-bundle: --content-tar, --out and --now-file are required")
		os.Exit(2)
	}
	now, err := readNow(*nowFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ca-bundle:", err)
		os.Exit(1)
	}
	if err := run(*contentTar, *out, now); err != nil {
		fmt.Fprintln(os.Stderr, "ca-bundle:", err)
		os.Exit(1)
	}
}

func readNow(path string) (time.Time, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, fmt.Errorf("read %s: %w", path, err)
	}
	now, err := time.Parse(time.RFC3339, strings.TrimSpace(string(b)))
	if err != nil {
		return time.Time{}, fmt.Errorf("%s: %w", path, err)
	}
	return now, nil
}

func run(contentTarPath, outPath string, now time.Time) error {
	source, err := readTarEntry(contentTarPath, trustSourcePath)
	if err != nil {
		return err
	}
	bundle, err := extractServerAnchors(bytes.NewReader(source), now)
	if err != nil {
		return fmt.Errorf("parse %s: %w", trustSourcePath, err)
	}
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", outPath, err)
	}
	if err := writeBundleTar(f, bundle); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func readTarEntry(tarPath, name string) ([]byte, error) {
	f, err := os.Open(tarPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", tarPath, err)
	}
	defer f.Close()
	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("%s has no %s", tarPath, name)
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", tarPath, err)
		}
		if hdr.Name == name {
			return io.ReadAll(tr)
		}
	}
}

// p11Object is one `[p11-kit-object-v1]` block: its attributes and the
// PEM blocks that follow them.
type p11Object struct {
	attrs map[string]string
	pems  []*pem.Block
}

// parseP11Kit reads p11-kit's persistence format: objects introduced by a
// `[p11-kit-object-v1]` line, `key: value` attribute lines, PEM blocks, and
// `#` comments (the human-readable certificate dump after each block).
func parseP11Kit(r io.Reader) ([]p11Object, error) {
	var objects []p11Object
	var cur *p11Object
	var pemLines []string
	inPEM := false
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case inPEM:
			pemLines = append(pemLines, line)
			if strings.HasPrefix(line, "-----END ") {
				block, _ := pem.Decode([]byte(strings.Join(pemLines, "\n") + "\n"))
				if block == nil {
					return nil, fmt.Errorf("undecodable PEM block in object %s", cur.attrs["label"])
				}
				cur.pems = append(cur.pems, block)
				inPEM = false
			}
		case line == "[p11-kit-object-v1]":
			objects = append(objects, p11Object{attrs: map[string]string{}})
			cur = &objects[len(objects)-1]
		case strings.HasPrefix(line, "-----BEGIN "):
			if cur == nil {
				return nil, fmt.Errorf("PEM block before any object header")
			}
			pemLines = []string{line}
			inPEM = true
		case cur == nil, line == "", strings.HasPrefix(line, "#"):
		default:
			key, value, ok := strings.Cut(line, ":")
			if !ok {
				return nil, fmt.Errorf("attribute line without ':' in object %s: %q", cur.attrs["label"], line)
			}
			cur.attrs[key] = strings.TrimSpace(value)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if inPEM {
		return nil, fmt.Errorf("unterminated PEM block")
	}
	return objects, nil
}

var (
	oidExtKeyUsage    = asn1.ObjectIdentifier{2, 5, 29, 37}
	oidServerAuth     = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 1}
	oidAnyExtendedKey = asn1.ObjectIdentifier{2, 5, 29, 37, 0}
)

// maxLeafLifetime is the longest a public TLS certificate may be valid,
// per the CA/Browser Forum baseline requirements since 2020. An anchor
// whose server distrust date is further in the past than this can no
// longer validate any leaf a browser would accept.
const maxLeafLifetime = 398 * 24 * time.Hour

// extractServerAnchors returns the PEM bundle of every certificate the
// trust source anchors for server authentication at `now`.
func extractServerAnchors(r io.Reader, now time.Time) ([]byte, error) {
	objects, err := parseP11Kit(r)
	if err != nil {
		return nil, err
	}

	// p11-kit attaches usage restrictions as separate objects that name a
	// certificate by its public key: `class: x-certificate-extension`,
	// `object-id: 2.5.29.37`, the whole X.509 extension (id, critical flag,
	// OCTET STRING around the ExtKeyUsage) as percent-encoded DER in
	// `value`, and the key as a PUBLIC KEY block.
	serverAuthByKey := map[string]bool{}
	for _, o := range objects {
		if o.attrs["class"] != "x-certificate-extension" || o.attrs["object-id"] != oidExtKeyUsage.String() {
			continue
		}
		der, err := decodeP11Value(o.attrs["value"])
		if err != nil {
			return nil, fmt.Errorf("object %s: %w", o.attrs["label"], err)
		}
		var ext pkix.Extension
		if _, err := asn1.Unmarshal(der, &ext); err != nil {
			return nil, fmt.Errorf("object %s: extension: %w", o.attrs["label"], err)
		}
		var usages []asn1.ObjectIdentifier
		if _, err := asn1.Unmarshal(ext.Value, &usages); err != nil {
			return nil, fmt.Errorf("object %s: ExtKeyUsage: %w", o.attrs["label"], err)
		}
		allowed := false
		for _, u := range usages {
			if u.Equal(oidServerAuth) || u.Equal(oidAnyExtendedKey) {
				allowed = true
			}
		}
		for _, b := range o.pems {
			if b.Type == "PUBLIC KEY" {
				serverAuthByKey[hex.EncodeToString(b.Bytes)] = allowed
			}
		}
	}

	var bundle bytes.Buffer
	for _, o := range objects {
		if o.attrs["trusted"] != "true" || o.attrs["x-distrusted"] == "true" {
			continue
		}
		if dead, err := distrustedForServersAt(o.attrs["nss-server-distrust-after"], now); err != nil {
			return nil, fmt.Errorf("object %s: %w", o.attrs["label"], err)
		} else if dead {
			continue
		}
		for _, b := range o.pems {
			if b.Type != "CERTIFICATE" {
				continue
			}
			spki, err := subjectPublicKeyInfo(b.Bytes)
			if err != nil {
				return nil, fmt.Errorf("object %s: %w", o.attrs["label"], err)
			}
			if allowed, restricted := serverAuthByKey[hex.EncodeToString(spki)]; restricted && !allowed {
				continue
			}
			if err := pem.Encode(&bundle, &pem.Block{Type: "CERTIFICATE", Bytes: b.Bytes}); err != nil {
				return nil, err
			}
		}
	}
	return bundle.Bytes(), nil
}

// subjectPublicKeyInfo returns the DER of a certificate's
// SubjectPublicKeyInfo, the key an extension object names it by. It walks
// the TBSCertificate rather than calling x509.ParseCertificate: that
// parser judges the whole certificate, and refuses Mozilla's EC-ACC root
// for its negative serial number, which every TLS stack accepts and which
// belongs in the bundle all the same.
func subjectPublicKeyInfo(cert []byte) ([]byte, error) {
	var outer, tbs asn1.RawValue
	if _, err := asn1.Unmarshal(cert, &outer); err != nil {
		return nil, fmt.Errorf("certificate: %w", err)
	}
	if _, err := asn1.Unmarshal(outer.Bytes, &tbs); err != nil {
		return nil, fmt.Errorf("tbsCertificate: %w", err)
	}
	// version [0] EXPLICIT (optional), serialNumber, signature, issuer,
	// validity, subject, subjectPublicKeyInfo.
	rest := tbs.Bytes
	var field asn1.RawValue
	var err error
	if rest, err = asn1.Unmarshal(rest, &field); err != nil {
		return nil, fmt.Errorf("tbsCertificate: %w", err)
	}
	if field.Class == asn1.ClassContextSpecific && field.Tag == 0 {
		if rest, err = asn1.Unmarshal(rest, &field); err != nil {
			return nil, fmt.Errorf("serialNumber: %w", err)
		}
	}
	for _, name := range []string{"signature", "issuer", "validity", "subject", "subjectPublicKeyInfo"} {
		if rest, err = asn1.Unmarshal(rest, &field); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	return field.FullBytes, nil
}

// distrustedForServersAt reports whether an anchor's nss-server-distrust-
// after date, as p11-kit persists it, is more than one maximum leaf
// lifetime before `now`. The value is a quoted ASN.1 UTCTime
// ("YYMMDDhhmmssZ") or GeneralizedTime ("YYYYMMDDhhmmssZ"); "%00" is the
// attribute unset.
func distrustedForServersAt(value string, now time.Time) (bool, error) {
	if value == "" || value == `"%00"` {
		return false, nil
	}
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return false, fmt.Errorf("nss-server-distrust-after is not quoted: %s", value)
	}
	raw := value[1 : len(value)-1]
	var date time.Time
	var err error
	switch len(raw) {
	case len("YYMMDDhhmmssZ"):
		date, err = time.Parse("060102150405Z", raw)
	case len("YYYYMMDDhhmmssZ"):
		date, err = time.Parse("20060102150405Z", raw)
	default:
		err = fmt.Errorf("neither UTCTime nor GeneralizedTime")
	}
	if err != nil {
		return false, fmt.Errorf("nss-server-distrust-after %s: %w", value, err)
	}
	return date.Add(maxLeafLifetime).Before(now), nil
}

// decodeP11Value undoes p11-kit's quoting of a binary attribute: the value
// is double-quoted, printable bytes appear as themselves and every other
// byte as %XX.
func decodeP11Value(quoted string) ([]byte, error) {
	if len(quoted) < 2 || quoted[0] != '"' || quoted[len(quoted)-1] != '"' {
		return nil, fmt.Errorf("value is not quoted: %q", quoted)
	}
	s := quoted[1 : len(quoted)-1]
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			out = append(out, s[i])
			continue
		}
		if i+2 >= len(s) {
			return nil, fmt.Errorf("truncated %%XX escape in %q", quoted)
		}
		b, err := hex.DecodeString(s[i+1 : i+3])
		if err != nil {
			return nil, fmt.Errorf("bad %%XX escape in %q: %w", quoted, err)
		}
		out = append(out, b[0])
		i += 2
	}
	return out, nil
}

// writeBundleTar writes the bundle and the two symlinks OpenSSL's
// compiled-in defaults on RHEL resolve (`openssl version -d` paths:
// cafile /etc/pki/tls/cert.pem, capath /etc/pki/tls/certs). No parent
// directories: the rpm ships every one of them, and a second entry for
// the same path with other metadata survives `flatten`'s dedupe, which
// merges identical entries only, as a duplicate path — one dockerd's
// classic store refuses to load. This tar is only ever composed beside
// the rpm's content.
func writeBundleTar(w io.Writer, bundle []byte) error {
	tw := tar.NewWriter(w)
	epoch := time.Unix(0, 0)
	if err := tw.WriteHeader(&tar.Header{Name: bundlePath, Mode: 0o644, Size: int64(len(bundle)), Typeflag: tar.TypeReg, ModTime: epoch, Format: tar.FormatUSTAR}); err != nil {
		return err
	}
	if _, err := tw.Write(bundle); err != nil {
		return err
	}
	for _, link := range []string{"./etc/pki/tls/certs/ca-bundle.crt", "./etc/pki/tls/cert.pem"} {
		if err := tw.WriteHeader(&tar.Header{Name: link, Mode: 0o777, Typeflag: tar.TypeSymlink, Linkname: bundleTarget, ModTime: epoch, Format: tar.FormatUSTAR}); err != nil {
			return err
		}
	}
	return tw.Close()
}
