package mirror_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"slices"
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
	"github.com/arkeros/distroless/web/internal/directory"
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
	attestAt(t, repository, subject, predicateType, predicate, time.Time{})
}

// attestAt is attest with a signing time, which a real bundle carries in its
// log entry and the unverified stub below reads off a field of its own.
func attestAt(t *testing.T, repository name.Repository, subject v1.Descriptor, predicateType attestation.PredicateType, predicate any, signedAt time.Time) {
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
		// Not a bundle field. Stands in for the log entry's integrated time,
		// which only a real verifier could read.
		"signedAt": signedAt,
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
		SignedAt time.Time `json:"signedAt"`
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
		SignedAt:  envelope.SignedAt,
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

// The names alone, for a page that only needs somewhere to link to. One
// registry call: no digest lookup per tag, which is what Versions is for.
func TestTagsListsNamesWithoutResolvingThem(t *testing.T) {
	server := ocitest.NewServer(t)
	subject := tagIndex(t, server, "node", "latest", time.Time{})
	tagIndex(t, server, "node", "22.1.0", time.Time{})
	tagIndex(t, server, "node", strings.Replace(subject, ":", "-", 1), time.Time{})

	requests := &counting{next: server.Config.Handler}
	server.Config.Handler = requests

	tags, err := newClient(t, server).Tags(context.Background(), "node")
	if err != nil {
		t.Fatalf("Tags: %v", err)
	}

	slices.Sort(tags)
	if want := []string{"22.1.0", "latest"}; !slices.Equal(tags, want) {
		t.Errorf("tags = %v, want %v — cosign's attachments are not versions", tags, want)
	}
	if requests.requested("/manifests/") {
		t.Error("resolved a tag to a digest, which is Versions' job and its cost")
	}
}

// grypeMatch is one finding as grype writes it, to the depth the page reads.
func grypeMatch(id, severity, pkg, version, arch string, fixed []string, state string) map[string]any {
	fixVersions := []any{}
	for _, v := range fixed {
		fixVersions = append(fixVersions, v)
	}
	return map[string]any{
		"vulnerability": map[string]any{
			"id":          id,
			"dataSource":  "https://security-tracker.debian.org/tracker/" + id,
			"severity":    severity,
			"description": "Something about " + pkg,
			"fix":         map[string]any{"versions": fixVersions, "state": state},
		},
		"artifact": map[string]any{
			"name":    pkg,
			"version": version,
			"type":    "deb",
			"purl":    "pkg:deb/debian/" + pkg + "@" + version + "?arch=" + arch + "&distro=debian-unstable",
		},
	}
}

// grypeReport is grype's own document: one finding matched on both
// architectures of an Index, and one the project has a VEX statement about.
var grypeReport = map[string]any{
	"matches": []any{
		grypeMatch("CVE-2026-0001", "High", "openssl", "3.0.11-1~deb12u2", "amd64", []string{"3.0.11-1~deb12u3"}, "fixed"),
		grypeMatch("CVE-2026-0001", "High", "openssl", "3.0.11-1~deb12u2", "arm64", []string{"3.0.11-1~deb12u3"}, "fixed"),
		grypeMatch("CVE-2013-0337", "Medium", "nginx", "1.30.4-1~trixie", "amd64", nil, "not-fixed"),
	},
	"descriptor": map[string]any{
		"name":      "grype",
		"version":   "0.118.0",
		"timestamp": "2026-09-03T21:12:53.712036+02:00",
		"db":        map[string]any{"status": map[string]any{"built": "2026-09-03T06:30:55Z"}},
	},
}

// scanRecord wraps a grype report the way //oci:supply_chain.bzl does before
// `cosign attest --type=vuln`: cosign's predicate, with the report as result.
func scanRecord(finished string, report any) map[string]any {
	return map[string]any{
		"invocation": map[string]any{"parameters": []any{}, "uri": "", "event_id": "", "builder.id": ""},
		"scanner": map[string]any{
			"uri":     "pkg:github/anchore/grype@v0.118.0",
			"version": "0.118.0",
			"db": map[string]any{
				"uri":     "https://grype.anchore.io/databases/v6/vulnerability-db_v6.1.9_2026-09-03T00:34:04Z_1788417055.tar.zst",
				"version": "2026-09-03T06:30:55Z",
			},
			"result": report,
		},
		"metadata": map[string]any{"scanStartedOn": finished, "scanFinishedOn": finished},
	}
}

// vexDocument is an OpenVEX document as //oci:vex.bzl writes one.
func vexDocument(statements ...map[string]any) map[string]any {
	list := make([]any, 0, len(statements))
	for _, statement := range statements {
		list = append(list, statement)
	}
	return map[string]any{
		"@context":   "https://openvex.dev/ns/v0.2.0",
		"@id":        "https://github.com/arkeros/distroless/blob/main/images/nginx/BUILD#debian-stable",
		"author":     "rafael@arquero.cat",
		"timestamp":  "2026-08-01T00:00:00Z",
		"version":    1,
		"statements": list,
	}
}

var notAffected = map[string]any{
	"vulnerability":    map[string]any{"name": "CVE-2013-0337"},
	"products":         []any{map[string]any{"@id": "pkg:oci/nginx"}},
	"status":           "not_affected",
	"justification":    "vulnerable_code_not_in_execute_path",
	"impact_statement": "No log files are created on disk.",
	"expires":          "2026-11-12",
}

func TestScanReturnsFindingsFromAttestation(t *testing.T) {
	server := ocitest.NewServer(t)
	repository, subject := pushIndex(t, server, "nginx", "latest")
	attest(t, repository, subject, attestation.Vuln, scanRecord("2026-09-03T21:12:53Z", grypeReport))

	digest, scan, err := newClient(t, server).Scan(context.Background(), "nginx", "latest")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if digest != subject.Digest.String() {
		t.Errorf("digest = %q, want the Index digest %q", digest, subject.Digest)
	}
	if scan.Scanner != "grype 0.118.0" {
		t.Errorf("scanner = %q, want grype 0.118.0", scan.Scanner)
	}
	if want := time.Date(2026, 9, 3, 6, 30, 55, 0, time.UTC); !scan.Database.Equal(want) {
		t.Errorf("database = %v, want %v — when the database was built", scan.Database, want)
	}
	if want := time.Date(2026, 9, 3, 21, 12, 53, 0, time.UTC); !scan.Finished.Equal(want) {
		t.Errorf("finished = %v, want %v", scan.Finished, want)
	}
	if len(scan.Findings) != 3 {
		t.Fatalf("got %d findings, want 3 — one per match, architectures included: %+v", len(scan.Findings), scan.Findings)
	}

	openssl := scan.Findings[0]
	if openssl.ID != "CVE-2026-0001" || openssl.Severity != directory.High {
		t.Errorf("finding = %+v, want CVE-2026-0001 High", openssl)
	}
	if openssl.Package != "openssl" || openssl.Version != "3.0.11-1~deb12u2" {
		t.Errorf("finding matched to %s %s, want openssl 3.0.11-1~deb12u2", openssl.Package, openssl.Version)
	}
	// The Index scan carries every architecture, so the arch qualifier is the
	// only thing separating the two openssl findings.
	if openssl.Arch != "amd64" || scan.Findings[1].Arch != "arm64" {
		t.Errorf("arches = %q, %q, want amd64 then arm64 from the purl qualifier", openssl.Arch, scan.Findings[1].Arch)
	}
	if openssl.Type != "deb" {
		t.Errorf("type = %q, want deb", openssl.Type)
	}
	if !slices.Equal(openssl.FixedIn, []string{"3.0.11-1~deb12u3"}) || openssl.FixState != "fixed" {
		t.Errorf("fix = %v %q, want [3.0.11-1~deb12u3] fixed", openssl.FixedIn, openssl.FixState)
	}
	if openssl.URL != "https://security-tracker.debian.org/tracker/CVE-2026-0001" {
		t.Errorf("url = %q, want the tracker entry the scanner read", openssl.URL)
	}
	if openssl.Suppressed != nil {
		t.Errorf("finding suppressed with no VEX document attached: %+v", openssl.Suppressed)
	}
}

// The scanner is asked for everything it found; the VEX document is this
// project's statement about it. Joined here, by identifier, the way the
// publication gates apply the same statements.
func TestScanMarksFindingsAVEXStatementCovers(t *testing.T) {
	server := ocitest.NewServer(t)
	repository, subject := pushIndex(t, server, "nginx", "latest")
	attest(t, repository, subject, attestation.Vuln, scanRecord("2026-09-03T21:12:53Z", grypeReport))
	attest(t, repository, subject, attestation.OpenVEX, vexDocument(notAffected))

	_, scan, err := newClient(t, server).Scan(context.Background(), "nginx", "latest")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	var nginx, openssl *directory.Suppression
	for _, finding := range scan.Findings {
		switch finding.ID {
		case "CVE-2013-0337":
			nginx = finding.Suppressed
		case "CVE-2026-0001":
			openssl = finding.Suppressed
		}
	}
	if nginx == nil {
		t.Fatal("CVE-2013-0337 not marked suppressed by the not_affected statement")
	}
	want := directory.Suppression{Status: "not_affected", Justification: "vulnerable_code_not_in_execute_path", Impact: "No log files are created on disk."}
	if *nginx != want {
		t.Errorf("suppression = %+v, want %+v", *nginx, want)
	}
	if openssl != nil {
		t.Errorf("CVE-2026-0001 suppressed by a statement about another CVE: %+v", openssl)
	}
}

// Only a statement that says the image is not affected, or already fixed,
// silences anything. "affected" and "under_investigation" are statements too,
// and they leave the finding standing.
func TestScanLeavesFindingsAnAffectedStatementNames(t *testing.T) {
	server := ocitest.NewServer(t)
	repository, subject := pushIndex(t, server, "nginx", "latest")
	attest(t, repository, subject, attestation.Vuln, scanRecord("2026-09-03T21:12:53Z", grypeReport))
	attest(t, repository, subject, attestation.OpenVEX, vexDocument(map[string]any{
		"vulnerability":    map[string]any{"name": "CVE-2013-0337"},
		"products":         []any{map[string]any{"@id": "pkg:oci/nginx"}},
		"status":           "affected",
		"action_statement": "Mount /var/log/nginx read-only.",
	}))

	_, scan, err := newClient(t, server).Scan(context.Background(), "nginx", "latest")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	for _, finding := range scan.Findings {
		if finding.Suppressed != nil {
			t.Errorf("%s suppressed by an 'affected' statement", finding.ID)
		}
	}
}

// A digest may be re-scanned, and then carries two records. The newer one is
// what the page shows; the older is history.
func TestScanPicksTheNewestRecord(t *testing.T) {
	server := ocitest.NewServer(t)
	repository, subject := pushIndex(t, server, "nginx", "latest")
	attest(t, repository, subject, attestation.Vuln, scanRecord("2026-09-10T00:00:00Z", map[string]any{
		"matches":    []any{grypeMatch("CVE-2026-0002", "Critical", "zlib1g", "1.3-1", "amd64", nil, "not-fixed")},
		"descriptor": map[string]any{"name": "grype", "version": "0.119.0"},
	}))
	attest(t, repository, subject, attestation.Vuln, scanRecord("2026-09-03T21:12:53Z", grypeReport))

	_, scan, err := newClient(t, server).Scan(context.Background(), "nginx", "latest")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(scan.Findings) != 1 || scan.Findings[0].ID != "CVE-2026-0002" {
		t.Errorf("findings = %+v, want the single finding of the newer scan", scan.Findings)
	}
	if scan.Scanner != "grype 0.119.0" {
		t.Errorf("scanner = %q, want the newer scan's", scan.Scanner)
	}
}

// An SBOM is not a scan. A build published before scans were attached has one
// and not the other, and the page for the other must say so.
func TestScanErrorsWhenOnlyAnSBOMIsAttested(t *testing.T) {
	server := ocitest.NewServer(t)
	repository, subject := pushIndex(t, server, "nginx", "latest")
	attest(t, repository, subject, attestation.CycloneDX, sbomPredicate)

	if _, _, err := newClient(t, server).Scan(context.Background(), "nginx", "latest"); err == nil {
		t.Error("Scan succeeded on an Index with only an SBOM attached, want an error")
	}
}

func TestScanRefusesAnAttestationThatDoesNotVerify(t *testing.T) {
	server := ocitest.NewServer(t)
	repository, subject := pushIndex(t, server, "nginx", "latest")
	attest(t, repository, subject, attestation.Vuln, scanRecord("2026-09-03T21:12:53Z", grypeReport))

	client := mirror.New(server.Listener.Addr().String(), "", rejecting{}, mirror.Insecure())

	if _, scan, err := client.Scan(context.Background(), "nginx", "latest"); err == nil || scan != nil {
		t.Errorf("served a scan whose attestation failed verification: %+v, %v", scan, err)
	}
}

// The download is the record the signature covers — the newest one, so it is
// the record the page was rendered from.
func TestScanDocumentReturnsTheNewestAttestedPredicate(t *testing.T) {
	server := ocitest.NewServer(t)
	repository, subject := pushIndex(t, server, "nginx", "latest")
	newer := scanRecord("2026-09-10T00:00:00Z", map[string]any{"matches": []any{}, "descriptor": map[string]any{"name": "grype", "version": "0.119.0"}})
	attest(t, repository, subject, attestation.Vuln, scanRecord("2026-09-03T21:12:53Z", grypeReport))
	attest(t, repository, subject, attestation.Vuln, newer)

	digest, document, err := newClient(t, server).ScanDocument(context.Background(), "nginx", "latest")
	if err != nil {
		t.Fatalf("ScanDocument: %v", err)
	}
	if digest != subject.Digest.String() {
		t.Errorf("digest = %q, want %q", digest, subject.Digest)
	}
	var got, want any
	if err := json.Unmarshal(document, &got); err != nil {
		t.Fatalf("document is not JSON: %v\n%s", err, document)
	}
	encoded, _ := json.Marshal(newer)
	json.Unmarshal(encoded, &want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("document = %s,\nwant the newer record %s", document, encoded)
	}
}

// A VEX document is reissued when a statement is added, withdrawn or
// re-justified, and then the digest carries both. Only the newest speaks: a
// withdrawn statement in an older document must not go on silencing.
func TestScanAppliesOnlyTheNewestVEXDocument(t *testing.T) {
	older := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(24 * time.Hour)
	for _, order := range []struct {
		name  string
		first time.Time
	}{
		{"older attached first", older},
		{"newer attached first", newer},
	} {
		t.Run(order.name, func(t *testing.T) {
			server := ocitest.NewServer(t)
			repository, subject := pushIndex(t, server, "nginx", "latest")
			attest(t, repository, subject, attestation.Vuln, scanRecord("2026-09-03T21:12:53Z", grypeReport))
			second := newer
			if order.first.Equal(newer) {
				second = older
			}
			for _, at := range []time.Time{order.first, second} {
				if at.Equal(older) {
					attestAt(t, repository, subject, attestation.OpenVEX, vexDocument(notAffected), at)
				} else {
					attestAt(t, repository, subject, attestation.OpenVEX, vexDocument(), at)
				}
			}

			_, scan, err := newClient(t, server).Scan(context.Background(), "nginx", "latest")
			if err != nil {
				t.Fatalf("Scan: %v", err)
			}
			for _, finding := range scan.Findings {
				if finding.Suppressed != nil {
					t.Errorf("%s still suppressed by a statement the newer VEX document withdrew", finding.ID)
				}
			}
		})
	}
}

// A digest acquires a fresh scan whenever the database pin moves, and a page
// that cached the old one for the life of the process would not show it.
func TestScanIsReReadAfterItsCacheExpires(t *testing.T) {
	server := ocitest.NewServer(t)
	repository, subject := pushIndex(t, server, "nginx", "latest")
	attest(t, repository, subject, attestation.Vuln, scanRecord("2026-09-03T21:12:53Z", grypeReport))
	requests := &counting{next: server.Config.Handler}
	server.Config.Handler = requests

	client := mirror.New(server.Listener.Addr().String(), "", unverified{}, mirror.Insecure(), mirror.ScanTTL(time.Millisecond))
	for range 2 {
		if _, _, err := client.Scan(context.Background(), "nginx", "latest"); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	if got := requests.count("/referrers/"); got != 2 {
		t.Errorf("referrers listed %d times across two reads after expiry, want 2", got)
	}
}

// Within the TTL the answer is the cached one: verifying costs registry round
// trips and a signature check per referrer, per page view otherwise.
func TestScanIsCachedWithinItsTTL(t *testing.T) {
	server := ocitest.NewServer(t)
	repository, subject := pushIndex(t, server, "nginx", "latest")
	attest(t, repository, subject, attestation.Vuln, scanRecord("2026-09-03T21:12:53Z", grypeReport))
	requests := &counting{next: server.Config.Handler}
	server.Config.Handler = requests

	client := newClient(t, server)
	for range 2 {
		if _, _, err := client.Scan(context.Background(), "nginx", "latest"); err != nil {
			t.Fatalf("Scan: %v", err)
		}
	}

	if got := requests.count("/referrers/"); got != 1 {
		t.Errorf("referrers listed %d times across two reads, want 1", got)
	}
}
