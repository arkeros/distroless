package main

import (
	"archive/tar"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// selfSigned returns a CA certificate named cn, its DER and its SPKI PEM,
// which is how a p11-kit extension object names the certificate it
// restricts.
func selfSigned(t *testing.T, cn string) (der []byte, spkiPEM string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Unix(0, 0),
		NotAfter:              time.Unix(0, 0).Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err = x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return der, string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: cert.RawSubjectPublicKeyInfo}))
}

// p11Value quotes DER the way p11-kit persists binary attributes: printable
// ASCII as itself, anything else as %XX.
func p11Value(der []byte) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, c := range der {
		if c >= 0x20 && c < 0x7f && c != '%' && c != '"' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02x", c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// ekuValue is the `value` of an x-certificate-extension object: the whole
// X.509 extension, as the real file carries it —
// `0%16%06%03U%1d%25%01%01%ff%04%0c0%0a%06%08%2b%06%01%05%05%07%03%03`
// is SEQUENCE { id 2.5.29.37, critical TRUE, OCTET STRING { SEQUENCE {
// codeSigning } } }.
func ekuValue(t *testing.T, oids ...asn1.ObjectIdentifier) string {
	t.Helper()
	inner, err := asn1.Marshal(oids)
	if err != nil {
		t.Fatal(err)
	}
	der, err := asn1.Marshal(pkix.Extension{Id: oidExtKeyUsage, Critical: true, Value: inner})
	if err != nil {
		t.Fatal(err)
	}
	return p11Value(der)
}

// fixedNow is the build time the tests run at: 2026-09-06, the Hummingbird
// snapshot the source was read from.
var fixedNow = time.Date(2026, 9, 6, 0, 1, 19, 0, time.UTC)

func certObject(label, attrs string, der []byte) string {
	return "[p11-kit-object-v1]\nlabel: \"" + label + "\"\n" + attrs +
		string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})) +
		"#Certificate:\n#    Data:\n#        Version: 3 (0x2)\n\n"
}

func extensionObject(label, value, spkiPEM string) string {
	return "[p11-kit-object-v1]\nlabel: \"" + label + "\"\nclass: x-certificate-extension\nobject-id: 2.5.29.37\nvalue: " + value + "\nmodifiable: false\n" + spkiPEM + "\n"
}

// TestExtractServerAnchors is the trust policy in one fixture: which of
// the objects in a ca-bundle.trust.p11-kit end up in tls-ca-bundle.pem.
func TestExtractServerAnchors(t *testing.T) {
	server, serverKey := selfSigned(t, "Server Auth CA")
	codeOnly, codeOnlyKey := selfSigned(t, "Code Signing Only CA")
	distrusted, _ := selfSigned(t, "Explicitly Distrusted CA")
	plain, _ := selfSigned(t, "Unrestricted CA")
	untrusted, _ := selfSigned(t, "Present But Not Trusted CA")

	codeSigning := asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 3}
	source := "# This is a bundle of X.509 certificates of public Certificate\n# Authorities.\n\n" +
		extensionObject("Server Auth CA", ekuValue(t, oidServerAuth, codeSigning), serverKey) +
		certObject("Server Auth CA", "trusted: true\nnss-mozilla-ca-policy: true\nmodifiable: false\n", server) +
		extensionObject("Code Signing Only CA", ekuValue(t, codeSigning), codeOnlyKey) +
		certObject("Code Signing Only CA", "trusted: true\nmodifiable: false\n", codeOnly) +
		certObject("Explicitly Distrusted CA", "x-distrusted: true\nnss-mozilla-ca-policy: true\nmodifiable: false\n", distrusted) +
		certObject("Unrestricted CA", "trusted: true\nmodifiable: false\n", plain) +
		certObject("Present But Not Trusted CA", "modifiable: false\n", untrusted)

	bundle, err := extractServerAnchors(strings.NewReader(source), fixedNow)
	if err != nil {
		t.Fatalf("extractServerAnchors: %v", err)
	}

	var got []string
	for rest := bundle; len(rest) > 0; {
		block, tail := pem.Decode(rest)
		if block == nil {
			t.Fatalf("bundle has trailing non-PEM bytes: %q", rest)
		}
		if block.Type != "CERTIFICATE" {
			t.Errorf("bundle carries a %q block; only CERTIFICATE belongs in tls-ca-bundle.pem", block.Type)
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, cert.Subject.CommonName)
		rest = tail
	}
	want := []string{"Server Auth CA", "Unrestricted CA"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("bundle anchors %v, want %v", got, want)
	}
}

// TestSubjectPublicKeyInfo_NegativeSerial: Mozilla's EC-ACC root has a
// negative serial number. Go's x509 parser refuses it; the bundle must not.
func TestSubjectPublicKeyInfo_NegativeSerial(t *testing.T) {
	der, spkiPEM := selfSigned(t, "EC-ACC")
	// The TBSCertificate opens with `[0] EXPLICIT { INTEGER 2 }` followed
	// by the serial, INTEGER 1 as selfSigned issues it. Setting the high
	// bit of its value byte makes the serial negative.
	versionAndSerial := []byte{0xa0, 0x03, 0x02, 0x01, 0x02, 0x02, 0x01, 0x01}
	i := bytes.Index(der, versionAndSerial)
	if i < 0 {
		t.Fatalf("fixture has no version + serial 1 prefix: % x", der[:24])
	}
	der[i+len(versionAndSerial)-1] = 0xff
	if _, err := x509.ParseCertificate(der); err == nil {
		t.Fatalf("fixture is not a negative-serial certificate Go refuses")
	}

	got, err := subjectPublicKeyInfo(der)
	if err != nil {
		t.Fatalf("subjectPublicKeyInfo: %v", err)
	}
	block, _ := pem.Decode([]byte(spkiPEM))
	if !bytes.Equal(got, block.Bytes) {
		t.Errorf("subjectPublicKeyInfo returned %x, want %x", got, block.Bytes)
	}
}

func TestDecodeP11Value(t *testing.T) {
	got, err := decodeP11Value(`"0%16%06%03U%1d%25%01%01%ff"`)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x30, 0x16, 0x06, 0x03, 0x55, 0x1d, 0x25, 0x01, 0x01, 0xff}
	if !bytes.Equal(got, want) {
		t.Errorf("decodeP11Value = %x, want %x", got, want)
	}
	for _, bad := range []string{`unquoted`, `"%1"`, `"%zz"`} {
		if _, err := decodeP11Value(bad); err == nil {
			t.Errorf("decodeP11Value(%q) accepted", bad)
		}
	}
}

// TestRun_WritesBundleAndSymlinks walks the tool end to end from a content
// tar shaped like rpm-extract's, and checks the tar the static layer will
// flatten: the bundle where the rpm's symlinks point, and OpenSSL's two
// default paths pointing at it too.
func TestRun_WritesBundleAndSymlinks(t *testing.T) {
	der, _ := selfSigned(t, "Only CA")
	source := certObject("Only CA", "trusted: true\n", der)

	tmp := t.TempDir()
	contentTar := filepath.Join(tmp, "content.tar")
	f, err := os.Create(contentTar)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(f)
	if err := tw.WriteHeader(&tar.Header{Name: trustSourcePath, Mode: 0o644, Size: int64(len(source)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(source)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()

	out := filepath.Join(tmp, "cacerts.tar")
	if err := run(contentTar, out, fixedNow); err != nil {
		t.Fatalf("run: %v", err)
	}

	of, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer of.Close()
	entries := map[string]*tar.Header{}
	var bundle []byte
	tr := tar.NewReader(of)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		entries[hdr.Name] = hdr
		if hdr.Name == bundlePath {
			bundle, _ = io.ReadAll(tr)
		}
		if !hdr.ModTime.Equal(time.Unix(0, 0)) || hdr.Uid != 0 || hdr.Gid != 0 {
			t.Errorf("%s: mtime %v uid %d gid %d, want epoch and root", hdr.Name, hdr.ModTime, hdr.Uid, hdr.Gid)
		}
	}
	if !bytes.Contains(bundle, []byte("-----BEGIN CERTIFICATE-----")) {
		t.Errorf("%s does not carry the bundle", bundlePath)
	}
	for _, link := range []string{"./etc/pki/tls/certs/ca-bundle.crt", "./etc/pki/tls/cert.pem"} {
		hdr := entries[link]
		if hdr == nil || hdr.Typeflag != tar.TypeSymlink || hdr.Linkname != bundleTarget {
			t.Errorf("%s: want symlink to %s, got %+v", link, bundleTarget, hdr)
		}
	}
	// No directory entries: the rpm ships every parent, and a second copy
	// with other metadata survives flatten's dedupe as a duplicate path,
	// which dockerd refuses to load.
	for name, hdr := range entries {
		if hdr.Typeflag == tar.TypeDir {
			t.Errorf("%s: directory entry; parents belong to the rpm", name)
		}
	}
	if len(entries) != 3 {
		t.Errorf("tar has %d entries, want the bundle and two symlinks", len(entries))
	}
}

// TestRun_MissingTrustSource: a content tar without the trust source is a
// wrong input, not an empty bundle.
func TestRun_MissingTrustSource(t *testing.T) {
	tmp := t.TempDir()
	contentTar := filepath.Join(tmp, "content.tar")
	f, err := os.Create(contentTar)
	if err != nil {
		t.Fatal(err)
	}
	if err := tar.NewWriter(f).Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := run(contentTar, filepath.Join(tmp, "out.tar"), fixedNow); err == nil {
		t.Errorf("run accepted a content tar without %s", trustSourcePath)
	}
}

// TestExtractServerAnchors_DistrustAfter: nss-server-distrust-after says
// leaves issued after that date are distrusted, while earlier ones stay
// valid until they expire. A PEM bundle cannot carry that rule, so the
// bundle keeps such an anchor until no leaf a browser would accept can
// still chain to it: the date plus the 398-day maximum leaf lifetime.
func TestExtractServerAnchors_DistrustAfter(t *testing.T) {
	dead, _ := selfSigned(t, "Distrust Date Over A Year Ago")
	draining, _ := selfSigned(t, "Distrust Date Last Month")
	future, _ := selfSigned(t, "Distrust Date Next Year")
	unset, _ := selfSigned(t, "No Distrust Date")
	generalized, _ := selfSigned(t, "Distrust Date In GeneralizedTime")

	source := certObject("Distrust Date Over A Year Ago", "trusted: true\nnss-server-distrust-after: \"241130235959Z\"\n", dead) +
		certObject("Distrust Date Last Month", "trusted: true\nnss-server-distrust-after: \"260801000000Z\"\n", draining) +
		certObject("Distrust Date Next Year", "trusted: true\nnss-server-distrust-after: \"270101000000Z\"\n", future) +
		certObject("No Distrust Date", "trusted: true\nnss-server-distrust-after: \"%00\"\n", unset) +
		certObject("Distrust Date In GeneralizedTime", "trusted: true\nnss-server-distrust-after: \"20240630000000Z\"\n", generalized)

	bundle, err := extractServerAnchors(strings.NewReader(source), fixedNow)
	if err != nil {
		t.Fatalf("extractServerAnchors: %v", err)
	}
	var got []string
	for rest := bundle; len(rest) > 0; {
		block, tail := pem.Decode(rest)
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, cert.Subject.CommonName)
		rest = tail
	}
	want := []string{"Distrust Date Last Month", "Distrust Date Next Year", "No Distrust Date"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("bundle anchors %v, want %v", got, want)
	}

	for _, bad := range []string{`"2024-11-30"`, `"241130Z"`} {
		_, err := extractServerAnchors(strings.NewReader(certObject("x", "trusted: true\nnss-server-distrust-after: "+bad+"\n", unset)), fixedNow)
		if err == nil {
			t.Errorf("distrust date %s was accepted", bad)
		}
	}
}
