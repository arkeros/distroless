package mirror_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/arkeros/distroless/oci/ocitest"
	"github.com/arkeros/distroless/web/internal/attestation"
	"github.com/arkeros/distroless/web/internal/mirror"
)

const (
	bundleMediaType = "application/vnd.dev.sigstore.bundle.v0.3+json"
)

var sbomPredicate = map[string]any{
	"bomFormat":   "CycloneDX",
	"specVersion": "1.6",
	"components": []any{
		map[string]any{
			"name":     "openssl",
			"version":  "3.0.11-1~deb12u2",
			"purl":     "pkg:deb/debian/openssl@3.0.11-1~deb12u2?arch=amd64&distro=debian-unstable",
			"licenses": []any{map[string]any{"license": map[string]any{"id": "Apache-2.0"}}},
		},
		// A generic purl has no scanner matcher, so the silent-zero gate in
		// //oci:supply_chain.bzl forces a cpe onto it. Published SBOMs
		// therefore always look like this, never like a bare pkg:generic.
		map[string]any{
			"name":     "node",
			"version":  "22.1.0",
			"purl":     "pkg:generic/node@22.1.0",
			"cpe":      "cpe:2.3:a:nodejs:node.js:22.1.0:*:*:*:*:*:*:*",
			"licenses": []any{map[string]any{"expression": "MIT AND Apache-2.0"}},
		},
	},
}

// pushIndex publishes a multi-arch Index and returns the descriptor an
// attestation would name as its subject.
func pushIndex(t *testing.T, server *ocitest.Server, repository, tag string) (name.Repository, v1.Descriptor) {
	t.Helper()
	ref, err := name.ParseReference(server.Listener.Addr().String()+"/"+repository+":"+tag, name.Insecure)
	if err != nil {
		t.Fatal(err)
	}
	index, err := random.Index(256, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := remote.WriteIndex(ref, index); err != nil {
		t.Fatal(err)
	}

	digest, err := index.Digest()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := index.RawManifest()
	if err != nil {
		t.Fatal(err)
	}
	mediaType, err := index.MediaType()
	if err != nil {
		t.Fatal(err)
	}
	return ref.Context(), v1.Descriptor{MediaType: mediaType, Digest: digest, Size: int64(len(raw))}
}

// attest publishes a referrer carrying a Sigstore bundle whose DSSE payload is
// an in-toto statement — the shape `cosign attest --new-bundle-format` writes.
func attest(t *testing.T, repository name.Repository, subject v1.Descriptor, predicateType attestation.PredicateType, predicate any) {
	t.Helper()
	statement, err := json.Marshal(map[string]any{
		"_type":         "https://in-toto.io/Statement/v1",
		"predicateType": string(predicateType),
		"predicate":     predicate,
	})
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := json.Marshal(map[string]any{
		"mediaType": bundleMediaType,
		"dsseEnvelope": map[string]any{
			"payloadType": "application/vnd.in-toto+json",
			"payload":     base64.StdEncoding.EncodeToString(statement),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	image, err := mutate.AppendLayers(empty.Image, static.NewLayer(bundle, bundleMediaType))
	if err != nil {
		t.Fatal(err)
	}
	referrer := mutate.Subject(
		mutate.ConfigMediaType(
			mutate.MediaType(image, types.OCIManifestSchema1),
			types.MediaType("application/vnd.oci.empty.v1+json"),
		),
		subject,
	).(v1.Image)

	digest, err := referrer.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if err := remote.Write(repository.Digest(digest.String()), referrer); err != nil {
		t.Fatal(err)
	}
}

// unverified unwraps an attestation without checking its signature.
//
// mirror's responsibility is the referrer walk — resolving a tag to a Digest,
// finding the attestations on it, and picking out the SBOM. Whether a bundle
// is genuinely signed by the workflow allowed to publish is
// //web/internal/attestation's job, and its tests cover that against a virtual
// Sigstore. Stubbing it here keeps each test about one thing.
type unverified struct{}

func (unverified) Verify(blob []byte, _ string) (*attestation.Statement, error) {
	var envelope struct {
		DSSEEnvelope struct {
			Payload string `json:"payload"`
		} `json:"dsseEnvelope"`
	}
	if err := json.Unmarshal(blob, &envelope); err != nil {
		return nil, err
	}
	payload, err := base64.StdEncoding.DecodeString(envelope.DSSEEnvelope.Payload)
	if err != nil {
		return nil, err
	}
	var statement struct {
		PredicateType string          `json:"predicateType"`
		Predicate     json.RawMessage `json:"predicate"`
	}
	if err := json.Unmarshal(payload, &statement); err != nil {
		return nil, err
	}
	return &attestation.Statement{
		Type:      attestation.PredicateType(statement.PredicateType),
		Predicate: statement.Predicate,
	}, nil
}

func newClient(t *testing.T, server *ocitest.Server) *mirror.Client {
	t.Helper()
	return mirror.New(server.Listener.Addr().String(), "", unverified{}, mirror.Insecure())
}

func TestSBOMReturnsComponentsFromAttestation(t *testing.T) {
	server := ocitest.NewServer(t)
	repository, subject := pushIndex(t, server, "node", "latest")
	attest(t, repository, subject, attestation.CycloneDX, sbomPredicate)

	digest, components, err := newClient(t, server).SBOM(context.Background(), "node:latest")
	if err != nil {
		t.Fatalf("SBOM: %v", err)
	}
	if digest != subject.Digest.String() {
		t.Errorf("digest = %q, want the Index digest %q", digest, subject.Digest)
	}
	if len(components) != 2 {
		t.Fatalf("got %d components, want 2: %+v", len(components), components)
	}

	openssl := components[0]
	if openssl.Name != "openssl" || openssl.Version != "3.0.11-1~deb12u2" {
		t.Errorf("component = %+v, want openssl 3.0.11-1~deb12u2", openssl)
	}
	if openssl.License != "Apache-2.0" {
		t.Errorf("license = %q, want Apache-2.0", openssl.License)
	}
	if openssl.Type != "deb" {
		t.Errorf("type = %q, want deb (the purl ecosystem)", openssl.Type)
	}
	// The Index SBOM merges every architecture, so the arch qualifier is the
	// only thing separating two otherwise identical rows.
	if openssl.Arch != "amd64" {
		t.Errorf("arch = %q, want amd64 from the purl qualifier", openssl.Arch)
	}
	node := components[1]
	if node.Type != "generic" {
		t.Errorf("type = %q, want generic", node.Type)
	}
	// Without the cpe there is no way to tell a Routable component from a
	// silent-zero hazard, so it has to survive the projection.
	if node.CPE == "" {
		t.Error("cpe dropped; a generic component is only Routable because of it")
	}
	// CycloneDX licenses are a union of SPDX id, name and expression.
	if node.License != "MIT AND Apache-2.0" {
		t.Errorf("license = %q, want the SPDX expression", node.License)
	}
}

// The download hands a reader evidence, so what comes back is the predicate
// the signature covers — not the projection the page renders, and not a
// re-marshalling of it.
func TestDocumentReturnsTheAttestedPredicate(t *testing.T) {
	server := ocitest.NewServer(t)
	repository, subject := pushIndex(t, server, "node", "latest")
	attest(t, repository, subject, attestation.CycloneDX, sbomPredicate)

	digest, document, err := newClient(t, server).Document(context.Background(), "node:latest")
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	if digest != subject.Digest.String() {
		t.Errorf("digest = %q, want the Index digest %q", digest, subject.Digest)
	}

	var got any
	if err := json.Unmarshal(document, &got); err != nil {
		t.Fatalf("document is not JSON: %v\n%s", err, document)
	}
	// Compared through JSON on both sides: what matters is that nothing was
	// added, dropped or rewritten, not how Go happened to order the bytes.
	encoded, err := json.Marshal(sbomPredicate)
	if err != nil {
		t.Fatal(err)
	}
	var want any
	if err := json.Unmarshal(encoded, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("document = %s,\nwant %s", document, encoded)
	}

	// The page drops the purl once it has read the ecosystem and arch off it.
	// Its presence here is what says this is the document and not the table.
	if !bytes.Contains(document, []byte("pkg:deb/debian/openssl")) {
		t.Error("document does not carry the purl; this looks like the projection, not the evidence")
	}
}

// Cosign writes provenance and SBOM as the same artifactType, so the predicate
// type inside the envelope is the only thing that tells them apart.
func TestSBOMIgnoresOtherAttestationTypes(t *testing.T) {
	server := ocitest.NewServer(t)
	repository, subject := pushIndex(t, server, "node", "latest")
	attest(t, repository, subject, attestation.SLSAProvenance, map[string]any{"buildDefinition": map[string]any{}})
	attest(t, repository, subject, attestation.CycloneDX, sbomPredicate)

	_, components, err := newClient(t, server).SBOM(context.Background(), "node:latest")
	if err != nil {
		t.Fatalf("SBOM: %v", err)
	}
	if len(components) != 2 {
		t.Fatalf("got %d components, want the 2 from the CycloneDX attestation", len(components))
	}
}

func TestSBOMErrorsWhenNothingIsAttested(t *testing.T) {
	server := ocitest.NewServer(t)
	pushIndex(t, server, "node", "latest")

	if _, _, err := newClient(t, server).SBOM(context.Background(), "node:latest"); err == nil {
		t.Error("SBOM succeeded on an Index with no attestations, want an error")
	}
}

func TestSBOMErrorsOnUnknownRepository(t *testing.T) {
	server := ocitest.NewServer(t)

	if _, _, err := newClient(t, server).SBOM(context.Background(), "nope:latest"); err == nil {
		t.Error("SBOM succeeded on an unpublished repository, want an error")
	}
}

// rejecting stands in for an attestation that is unsigned, signed by the wrong
// workflow, or bound to another image.
type rejecting struct{}

func (rejecting) Verify([]byte, string) (*attestation.Statement, error) {
	return nil, errors.New("certificate identity does not match policy")
}

// The whole point of verifying: a well-formed SBOM that does not verify is not
// evidence, and must not reach the page.
func TestSBOMRefusesAnAttestationThatDoesNotVerify(t *testing.T) {
	server := ocitest.NewServer(t)
	repository, subject := pushIndex(t, server, "node", "latest")
	attest(t, repository, subject, attestation.CycloneDX, sbomPredicate)

	client := mirror.New(server.Listener.Addr().String(), "", rejecting{}, mirror.Insecure())

	_, components, err := client.SBOM(context.Background(), "node:latest")
	if err == nil {
		t.Error("served an SBOM whose attestation failed verification")
	}
	if components != nil {
		t.Errorf("returned %d components from an unverified attestation", len(components))
	}
}
