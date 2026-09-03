package mirror_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

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

	digest, components, err := newClient(t, server).SBOM(context.Background(), "node", "latest")
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

	digest, document, err := newClient(t, server).Document(context.Background(), "node", "latest")
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

	_, components, err := newClient(t, server).SBOM(context.Background(), "node", "latest")
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

	if _, _, err := newClient(t, server).SBOM(context.Background(), "node", "latest"); err == nil {
		t.Error("SBOM succeeded on an Index with no attestations, want an error")
	}
}

func TestSBOMErrorsOnUnknownRepository(t *testing.T) {
	server := ocitest.NewServer(t)

	if _, _, err := newClient(t, server).SBOM(context.Background(), "nope", "latest"); err == nil {
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

	_, components, err := client.SBOM(context.Background(), "node", "latest")
	if err == nil {
		t.Error("served an SBOM whose attestation failed verification")
	}
	if components != nil {
		t.Errorf("returned %d components from an unverified attestation", len(components))
	}
}

// counting wraps a registry and records the request paths that reach it.
type counting struct {
	next  http.Handler
	mu    sync.Mutex
	paths []string
}

func (c *counting) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	c.paths = append(c.paths, r.URL.Path)
	c.mu.Unlock()
	c.next.ServeHTTP(w, r)
}

func (c *counting) requested(substring string) bool { return c.count(substring) > 0 }

func (c *counting) count(substring string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	seen := 0
	for _, path := range c.paths {
		if strings.Contains(path, substring) {
			seen++
		}
	}
	return seen
}

// A digest reference already names what a tag lookup would return, and the
// component cache is keyed by that digest — so resolving one should cost no
// manifest lookup at all. These are exactly the URLs served with a long
// max-age, and the ones a permalink sends readers to.
func TestSBOMResolvesADigestWithoutAManifestLookup(t *testing.T) {
	server := ocitest.NewServer(t)
	repository, subject := pushIndex(t, server, "node", "latest")
	attest(t, repository, subject, attestation.CycloneDX, sbomPredicate)

	requests := &counting{next: server.Config.Handler}
	server.Config.Handler = requests

	digest, _, err := newClient(t, server).SBOM(context.Background(), "node", subject.Digest.String())
	if err != nil {
		t.Fatalf("SBOM: %v", err)
	}
	if digest != subject.Digest.String() {
		t.Errorf("digest = %q, want %q", digest, subject.Digest)
	}
	// Without this the assertions below would hold just as well if the counter
	// never saw a request at all.
	if !requests.requested("/referrers/") {
		t.Fatal("no request reached the counting registry, so it observed nothing")
	}
	if requests.requested("/manifests/" + subject.Digest.String()) {
		t.Error("resolved a digest reference by asking the registry for the manifest it already names")
	}
	if requests.requested("/manifests/latest") {
		t.Error("resolved a digest reference by looking up a tag")
	}
}

// tagIndex publishes an Index at a tag with a `created` on every child config,
// the way //oci:created_timestamp.bzl sets it, and returns the Index digest.
func tagIndex(t *testing.T, server *ocitest.Server, repository, tag string, created time.Time) string {
	t.Helper()
	ref, err := name.ParseReference(server.Listener.Addr().String()+"/"+repository+":"+tag, name.Insecure)
	if err != nil {
		t.Fatal(err)
	}
	index, err := random.Index(256, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !created.IsZero() {
		index = withCreated(t, index, created)
	}
	if err := remote.WriteIndex(ref, index); err != nil {
		t.Fatal(err)
	}
	digest, err := index.Digest()
	if err != nil {
		t.Fatal(err)
	}
	return digest.String()
}

// withCreated stamps every child image of an Index with a config `created`.
func withCreated(t *testing.T, index v1.ImageIndex, created time.Time) v1.ImageIndex {
	t.Helper()
	manifest, err := index.IndexManifest()
	if err != nil {
		t.Fatal(err)
	}
	var stamped v1.ImageIndex = empty.Index
	for _, descriptor := range manifest.Manifests {
		image, err := index.Image(descriptor.Digest)
		if err != nil {
			t.Fatal(err)
		}
		config, err := image.ConfigFile()
		if err != nil {
			t.Fatal(err)
		}
		config.Created = v1.Time{Time: created}
		image, err = mutate.ConfigFile(image, config)
		if err != nil {
			t.Fatal(err)
		}
		stamped = mutate.AppendManifests(stamped, mutate.IndexAddendum{Add: image})
	}
	return stamped
}

// publishArches publishes an Index whose children differ in size, arm64 named
// first so that picking "the first child" is visibly not the same as picking
// amd64. Returns the compressed size of each.
func publishArches(t *testing.T, server *ocitest.Server, repository, tag string) map[string]int64 {
	t.Helper()
	ref, err := name.ParseReference(server.Listener.Addr().String()+"/"+repository+":"+tag, name.Insecure)
	if err != nil {
		t.Fatal(err)
	}

	sizes := map[string]int64{}
	var index v1.ImageIndex = empty.Index
	// arm64 first, and with more layers, so a wrong pick reports a wrong size.
	for _, arch := range []struct {
		name   string
		layers int
	}{{"arm64", 3}, {"amd64", 1}} {
		image, err := random.Image(1024, int64(arch.layers))
		if err != nil {
			t.Fatal(err)
		}
		config, err := image.ConfigFile()
		if err != nil {
			t.Fatal(err)
		}
		config.Architecture, config.OS = arch.name, "linux"
		if image, err = mutate.ConfigFile(image, config); err != nil {
			t.Fatal(err)
		}
		manifest, err := image.Manifest()
		if err != nil {
			t.Fatal(err)
		}
		for _, layer := range manifest.Layers {
			sizes[arch.name] += layer.Size
		}
		index = mutate.AppendManifests(index, mutate.IndexAddendum{
			Add:        image,
			Descriptor: v1.Descriptor{Platform: &v1.Platform{Architecture: arch.name, OS: "linux"}},
		})
	}
	if err := remote.WriteIndex(ref, index); err != nil {
		t.Fatal(err)
	}
	if sizes["amd64"] == sizes["arm64"] {
		t.Fatalf("both architectures are %d bytes; the test cannot tell them apart", sizes["amd64"])
	}
	return sizes
}

// Every architecture, not a chosen one: they are different bytes, and
// reporting a single number would drop the rest silently.
func TestVersionsReportsASizePerArchitecture(t *testing.T) {
	server := ocitest.NewServer(t)
	sizes := publishArches(t, server, "node", "latest")

	versions, err := newClient(t, server).Versions(context.Background(), "node")
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}

	if len(versions) != 1 {
		t.Fatalf("listed %d versions, want 1", len(versions))
	}
	got := versions[0].Sizes
	if len(got) != 2 {
		t.Fatalf("reported %d architectures, want 2: %v", len(got), got)
	}
	for architecture, want := range sizes {
		if got[architecture].Bytes() != want {
			t.Errorf("%s = %d, want %d", architecture, got[architecture].Bytes(), want)
		}
	}
}

// The tags a family publishes are what the versions page is for.
func TestVersionsListsPublishedTags(t *testing.T) {
	server := ocitest.NewServer(t)
	shared := tagIndex(t, server, "node", "latest", time.Time{})
	tagIndex(t, server, "node", "22.1.0", time.Time{})

	versions, err := newClient(t, server).Versions(context.Background(), "node")
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}

	found := map[string]string{}
	for _, version := range versions {
		found[version.Tag] = version.Digest
	}
	if len(found) != 2 {
		t.Fatalf("listed %d tags, want 2: %+v", len(found), versions)
	}
	if found["latest"] != shared {
		t.Errorf("latest names %q, want %q", found["latest"], shared)
	}
}

// cosign pushes `sha256-<hex>` fallback tags for attachments. They are not
// versions, and unfiltered they bury the tags that are.
func TestVersionsSkipsCosignFallbackTags(t *testing.T) {
	server := ocitest.NewServer(t)
	subject := tagIndex(t, server, "node", "latest", time.Time{})
	fallback := strings.Replace(subject, ":", "-", 1)
	tagIndex(t, server, "node", fallback, time.Time{})
	tagIndex(t, server, "node", fallback+".sig", time.Time{})

	versions, err := newClient(t, server).Versions(context.Background(), "node")
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}

	for _, version := range versions {
		if strings.HasPrefix(version.Tag, "sha256-") {
			t.Errorf("listed cosign fallback tag %q as a version", version.Tag)
		}
	}
	if len(versions) != 1 {
		t.Errorf("listed %d versions, want 1: %+v", len(versions), versions)
	}
}

// Not a push date: //oci:created_timestamp.bzl sets `created` to the
// upstream-snapshot anchor, which is what a build-horizon policy measures.
func TestVersionsReadsTheBuildHorizonFromTheConfig(t *testing.T) {
	server := ocitest.NewServer(t)
	snapshot := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	tagIndex(t, server, "node", "latest", snapshot)

	versions, err := newClient(t, server).Versions(context.Background(), "node")
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}

	if len(versions) != 1 {
		t.Fatalf("listed %d versions, want 1", len(versions))
	}
	if got := versions[0].Created; !got.Equal(snapshot) {
		t.Errorf("created = %s, want %s", got, snapshot)
	}
}

// Several tags on one digest is the normal case — a whole tag_list lands on a
// single build — and the config behind it should be read once, not once per
// tag naming it.
func TestVersionsReadsEachBuildHorizonOnce(t *testing.T) {
	server := ocitest.NewServer(t)
	snapshot := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	shared := tagIndex(t, server, "node", "latest", snapshot)
	alias, err := name.ParseReference(server.Listener.Addr().String()+"/node:22.1.0", name.Insecure)
	if err != nil {
		t.Fatal(err)
	}
	source, err := name.ParseReference(server.Listener.Addr().String()+"/node@"+shared, name.Insecure)
	if err != nil {
		t.Fatal(err)
	}
	index, err := remote.Index(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := remote.WriteIndex(alias, index); err != nil {
		t.Fatal(err)
	}

	requests := &counting{next: server.Config.Handler}
	server.Config.Handler = requests

	versions, err := newClient(t, server).Versions(context.Background(), "node")
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}

	if len(versions) != 2 {
		t.Fatalf("listed %d versions, want 2: %+v", len(versions), versions)
	}
	for _, version := range versions {
		if version.Digest != shared || !version.Created.Equal(snapshot) {
			t.Errorf("version %+v does not name the shared build at %s", version, snapshot)
		}
	}
	if got := requests.count("/manifests/" + shared); got != 1 {
		t.Errorf("read the shared build's manifest %d times, want once", got)
	}
	for _, version := range versions {
		if len(version.Sizes) == 0 {
			t.Errorf("tag %q reports no size", version.Tag)
		}
	}
}
