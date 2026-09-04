// Package mirror reads the supply-chain evidence attached to images on the
// Mirror. It is the read side of what `mirror_push` publishes.
package mirror

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	cyclonedx "github.com/CycloneDX/cyclonedx-go"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/hashicorp/golang-lru/v2/expirable"

	"github.com/arkeros/distroless/web/internal/attestation"
	"github.com/arkeros/distroless/web/internal/directory"
)

// maxCachedImages bounds the component cache. Components are keyed by Digest,
// which is a content address, so an entry can never go stale — only be
// evicted.
const maxCachedImages = 100

// maxTagLookups bounds the fan-out when resolving a family's tags. A family
// publishes tens of tags, not thousands, and the registry is shared with
// every other reader of this process.
const maxTagLookups = 8

// cosignFallbackTag matches the tags cosign pushes when a registry has no
// referrers API — `sha256-<hex>` and its suffixed variants. They are
// attachments rather than published versions, and listing them would bury the
// real tags.
var cosignFallbackTag = regexp.MustCompile(`^sha256-[0-9a-f]{64}(\.[a-z]+)?$`)

// defaultScanTTL bounds how long a Digest's scan is served from memory. Unlike
// components, a scan is not immutable for a digest: CI attaches a fresh one
// whenever the vulnerability database pin moves, about daily, and a reader
// should see it within the hour rather than when this process happens to
// restart.
const defaultScanTTL = time.Hour

// maxAttestationBytes caps how much of a referrer blob is read. A full SBOM
// for a language runtime runs to a few MB; anything past this is not one.
const maxAttestationBytes = 64 << 20

// errUnverified marks a referrer that carried nothing this Verifier would
// accept: a signature rather than an attestation, or an attestation signed by
// someone else or about another image.
var errUnverified = errors.New("no verifiable attestation on this referrer")

// Verifier establishes that an attestation blob was signed by the identity
// permitted to publish to the Mirror, and reports what it asserts.
type Verifier interface {
	Verify(blob []byte, subjectDigest string) (*attestation.Statement, error)
}

// Client reads attestations off a registry holding the Mirror's images.
type Client struct {
	registry         string
	repositoryPrefix string
	verifier         Verifier
	nameOptions      []name.Option
	remoteOptions    []remote.Option

	// puller is shared across requests: it keeps the authenticated transport
	// and its bearer token, so resolving a tag does not re-do the registry
	// auth handshake on every page view.
	puller *remote.Puller

	// components resolved per Digest. Verifying an attestation costs several
	// registry round trips and a signature check; the answer is immutable for
	// a Digest, so it is worth keeping.
	cache *lru.Cache[string, []directory.Component]

	// builds is what a versions page reports about each build beyond its
	// digest, keyed by Digest. Immutable for a digest like the components
	// are, and read once per versions page rather than once per tag on it.
	builds *lru.Cache[string, build]

	// scans resolved per Digest, for the same reason as components — but
	// expiring, because a digest acquires newer scans. See defaultScanTTL.
	scans   *expirable.LRU[string, *directory.Scan]
	scanTTL time.Duration
}

// Option configures a Client.
type Option func(*Client)

// Insecure resolves references over plain HTTP. For tests.
func Insecure() Option {
	return func(c *Client) {
		c.nameOptions = append(c.nameOptions, name.Insecure)
	}
}

// ScanTTL sets how long a Digest's scan is served from memory before the
// registry is asked again. For tests; the default is defaultScanTTL.
func ScanTTL(ttl time.Duration) Option {
	return func(c *Client) {
		c.scanTTL = ttl
	}
}

// WithRemoteOptions adds options to every registry call, e.g. authentication.
func WithRemoteOptions(options ...remote.Option) Option {
	return func(c *Client) {
		c.remoteOptions = append(c.remoteOptions, options...)
	}
}

// New builds a Client. The Verifier is a constructor argument rather than an
// option because a Client that renders unverified attestations is not a
// degraded Client, it is the wrong thing entirely.
func New(registry, repositoryPrefix string, verifier Verifier, options ...Option) *Client {
	c := &Client{registry: registry, repositoryPrefix: repositoryPrefix, verifier: verifier, scanTTL: defaultScanTTL}
	// Only errors on a non-positive size, and maxCachedImages is a positive
	// constant — so this cannot fail at runtime.
	cache, err := lru.New[string, []directory.Component](maxCachedImages)
	if err != nil {
		panic(err)
	}
	c.cache = cache
	builds, err := lru.New[string, build](maxCachedImages)
	if err != nil {
		panic(err)
	}
	c.builds = builds
	for _, option := range options {
		option(c)
	}
	// After the options, so it carries the TTL they may have set.
	c.scans = expirable.NewLRU[string, *directory.Scan](maxCachedImages, nil, c.scanTTL)
	// Built after the options, so it carries them. Only errors on malformed
	// options, which would be a programming error here rather than a runtime
	// condition.
	puller, err := remote.NewPuller(c.remoteOptions...)
	if err != nil {
		panic(err)
	}
	c.puller = puller
	return c
}

// SBOM resolves family and ref to the Digest behind them and returns the
// Components of the verified CycloneDX attestation attached to that Digest.
//
// Attestations are bound to a Digest, never to a tag, so the digest is
// returned alongside: it is what the page was actually rendered from.
func (c *Client) SBOM(ctx context.Context, family, ref string) (string, []directory.Component, error) {
	subject, digest, options, err := c.resolve(ctx, family, ref)
	if err != nil {
		return "", nil, err
	}

	// The tag is re-resolved every time — tags move, and serving the wrong
	// Digest would be worse than being slow. What is cached is keyed by the
	// Digest itself, which cannot change underneath us.
	if components, ok := c.cache.Get(digest); ok {
		return digest, components, nil
	}

	predicate, err := c.predicate(subject, digest, options, attestation.CycloneDX)
	if err != nil {
		return "", nil, err
	}
	bom, err := decodeBOM(predicate)
	if err != nil {
		return "", nil, err
	}
	resolved := components(bom)
	c.cache.Add(digest, resolved)
	return digest, resolved, nil
}

// Document returns the verified CycloneDX attestation for an image exactly as
// it was signed.
//
// Deliberately not cached. The projection the page needs is small and worth
// keeping for a hundred images; the documents it was projected from run to
// several MB each, and a download is rare next to a page view.
func (c *Client) Document(ctx context.Context, family, ref string) (string, []byte, error) {
	subject, digest, options, err := c.resolve(ctx, family, ref)
	if err != nil {
		return "", nil, err
	}
	predicate, err := c.predicate(subject, digest, options, attestation.CycloneDX)
	if err != nil {
		return "", nil, err
	}
	return digest, predicate, nil
}

// Scan resolves family and ref and returns the vulnerability scan attached to
// the Digest behind them: the findings of the newest verified scan record,
// with those a verified VEX document covers marked as suppressed.
//
// Two attestations, joined here rather than at scan time. The scanner is asked
// for its unfiltered report, so the record says what it found; the VEX document
// is this project's statement about that report; and the join is by
// vulnerability identifier, which is how the publication gates apply the same
// statements (//oci:supply_chain.bzl). Applying the VEX inside the scanner
// would also have missed the statements scoped to the image rather than to a
// package — grype has no image identifier to match them against when what it
// scanned is an SBOM.
func (c *Client) Scan(ctx context.Context, family, ref string) (string, *directory.Scan, error) {
	subject, digest, options, err := c.resolve(ctx, family, ref)
	if err != nil {
		return "", nil, err
	}
	if scan, ok := c.scans.Get(digest); ok {
		return digest, scan, nil
	}

	found, err := c.predicates(subject, digest, options)
	if err != nil {
		return "", nil, err
	}
	record, err := newestScan(found[attestation.Vuln])
	if err != nil {
		return "", nil, fmt.Errorf("%w attached to %s", err, subject)
	}
	// Only the newest VEX document speaks. It is reissued whole whenever a
	// statement is added, withdrawn or re-justified, so an older one on the
	// same digest may still name a statement since withdrawn — and a
	// withdrawn statement must stop silencing.
	if vex := newestStatement(found[attestation.OpenVEX]); vex != nil {
		if err := suppress(record.scan, vex.Predicate); err != nil {
			// A VEX document that will not decode silences nothing, which
			// is the safe direction: the finding stays on the page.
			slog.Warn("skipping VEX document", "subject", subject, "error", err)
		}
	}
	c.scans.Add(digest, record.scan)
	return digest, record.scan, nil
}

// ScanDocument returns the newest verified scan record for an image exactly as
// it was signed — the same record Scan was rendered from. Not cached, as
// Document is not.
func (c *Client) ScanDocument(ctx context.Context, family, ref string) (string, []byte, error) {
	subject, digest, options, err := c.resolve(ctx, family, ref)
	if err != nil {
		return "", nil, err
	}
	found, err := c.predicates(subject, digest, options)
	if err != nil {
		return "", nil, err
	}
	record, err := newestScan(found[attestation.Vuln])
	if err != nil {
		return "", nil, fmt.Errorf("%w attached to %s", err, subject)
	}
	return digest, record.raw, nil
}

// Tags lists what a family publishes, and nothing else about it.
//
// One registry call. Versions answers a richer question and pays a lookup per
// tag for it; a page that only needs the names should not.
func (c *Client) Tags(ctx context.Context, family string) ([]string, error) {
	repository, err := c.repository(family)
	if err != nil {
		return nil, fmt.Errorf("parsing %q: %w", family, err)
	}
	return c.published(repository, append([]remote.Option{remote.WithContext(ctx)}, c.remoteOptions...))
}

// published lists a repository's tags with cosign's attachments removed.
func (c *Client) published(repository name.Repository, options []remote.Option) ([]string, error) {
	tags, err := remote.List(repository, options...)
	if err != nil {
		return nil, fmt.Errorf("listing tags of %s: %w", repository, err)
	}
	published := make([]string, 0, len(tags))
	for _, tag := range tags {
		if !cosignFallbackTag.MatchString(tag) {
			published = append(published, tag)
		}
	}
	return published, nil
}

// Versions lists the tags published for a family and the build each one names.
//
// Nothing here is verified, and nothing here can be: which build a tag points
// at is registry metadata that no attestation covers. What it gives a reader
// is the way in — the digest to ask for evidence about.
func (c *Client) Versions(ctx context.Context, family string) ([]directory.Version, error) {
	repository, err := c.repository(family)
	if err != nil {
		return nil, fmt.Errorf("parsing %q: %w", family, err)
	}
	options := append([]remote.Option{remote.WithContext(ctx)}, c.remoteOptions...)

	published, err := c.published(repository, options)
	if err != nil {
		return nil, err
	}

	// One Head per tag, in parallel: they are independent, they share an
	// already-authenticated transport, and done in series a family would cost
	// a page load per tag.
	versions := make([]directory.Version, len(published))
	var wait sync.WaitGroup
	slots := make(chan struct{}, maxTagLookups)
	for i, tag := range published {
		wait.Add(1)
		go func() {
			defer wait.Done()
			slots <- struct{}{}
			defer func() { <-slots }()

			descriptor, err := c.puller.Head(ctx, repository.Tag(tag))
			if err != nil {
				// A tag that vanished between the listing and the lookup is
				// not an error for the other tags. It is left zero and
				// dropped below.
				slog.Warn("skipping tag", "repository", repository, "tag", tag, "error", err)
				return
			}
			versions[i] = directory.Version{Tag: tag, Digest: descriptor.Digest.String()}
		}()
	}
	wait.Wait()

	resolved := make([]directory.Version, 0, len(versions))
	for _, version := range versions {
		if version.Digest != "" {
			resolved = append(resolved, version)
		}
	}
	c.describe(ctx, repository, resolved, options)
	return resolved, nil
}

// describe fills in what each Version reports beyond its digest.
//
// Read per distinct build rather than per tag — a tag_list lands several tags
// on one digest — and immutable for a digest, so it is cached.
func (c *Client) describe(ctx context.Context, repository name.Repository, versions []directory.Version, options []remote.Option) {
	described := make(map[string]build)
	for _, version := range versions {
		if _, known := described[version.Digest]; known {
			continue
		}
		found, err := c.build(ctx, repository.Digest(version.Digest), options)
		if err != nil {
			// A build whose manifest will not read still has tags worth
			// listing; its columns are simply blank.
			slog.Warn("skipping build detail", "repository", repository, "digest", version.Digest, "error", err)
		}
		described[version.Digest] = found
	}
	for i := range versions {
		versions[i].Created = described[versions[i].Digest].created
		versions[i].Sizes = described[versions[i].Digest].sizes
	}
}

// build is what a versions page reports about one build beyond its digest.
type build struct {
	// created is the `created` an image config carries, which
	// //oci:created_timestamp.bzl sets to the upstream-snapshot anchor of the
	// lockfile the build was assembled from — not a build or push time.
	created time.Time
	// sizes is the sum of the compressed layers per architecture: what a pull
	// transfers. Per architecture because they are different bytes — the same
	// nginx build is 23.9 MB on amd64 and 23.3 MB on arm64 — and picking one
	// would drop the other silently.
	sizes map[string]directory.Size
}

// build reads one build's detail: its horizon, and a size for every
// architecture it publishes.
//
// Architecture matters for the size in a way it does not for `created`: every
// architecture of one build is assembled from the same lockfile and carries
// the same anchor, but they are different bytes.
func (c *Client) build(ctx context.Context, subject name.Digest, options []remote.Option) (build, error) {
	if found, ok := c.builds.Get(subject.DigestStr()); ok {
		return found, nil
	}

	descriptor, err := c.puller.Get(ctx, subject)
	if err != nil {
		return build{}, err
	}

	found := build{sizes: map[string]directory.Size{}}

	// Asked of the media type rather than by trying Image() and checking the
	// error: Descriptor.Image() does not fail on an index, it quietly
	// resolves the manifest list to the default platform. Taking that as
	// "this is a single-architecture build" would report one architecture's
	// size for every build and never say so.
	if !descriptor.MediaType.IsIndex() {
		image, err := descriptor.Image()
		if err != nil {
			return build{}, err
		}
		config, err := image.ConfigFile()
		if err != nil {
			return build{}, err
		}
		size, err := compressedSize(image)
		if err != nil {
			return build{}, err
		}
		found.created = config.Created.Time
		found.sizes[config.Architecture] = size
		c.builds.Add(subject.DigestStr(), found)
		return found, nil
	}

	index, err := descriptor.ImageIndex()
	if err != nil {
		return build{}, fmt.Errorf("reading index %s: %w", subject, err)
	}
	manifest, err := index.IndexManifest()
	if err != nil {
		return build{}, err
	}
	if len(manifest.Manifests) == 0 {
		return build{}, fmt.Errorf("index %s carries no manifests", subject)
	}

	for _, child := range manifest.Manifests {
		image, err := remote.Image(subject.Context().Digest(child.Digest.String()), options...)
		if err != nil {
			return build{}, err
		}
		config, err := image.ConfigFile()
		if err != nil {
			return build{}, err
		}
		size, err := compressedSize(image)
		if err != nil {
			return build{}, err
		}
		// Keyed by the index's platform rather than the config's, because
		// that is what a puller matches on: it answers "how big is the image
		// I would get for this architecture". The config is the fallback for
		// an index that declares no platform.
		architecture := ""
		if child.Platform != nil {
			architecture = child.Platform.Architecture
		}
		if architecture == "" {
			architecture = config.Architecture
		}
		found.sizes[architecture] = size
		// Every architecture of one build carries the same anchor, so the
		// last one read is as good as the first.
		found.created = config.Created.Time
	}

	c.builds.Add(subject.DigestStr(), found)
	return found, nil
}

// compressedSize sums an image's layers: what pulling it transfers. Not the
// size on disk, which is the uncompressed rootfs and would cost a download of
// every layer to learn.
func compressedSize(image v1.Image) (directory.Size, error) {
	manifest, err := image.Manifest()
	if err != nil {
		return 0, err
	}
	var size int64
	for _, layer := range manifest.Layers {
		size += layer.Size
	}
	return directory.Size(size), nil
}

// resolve turns a reference into the Digest behind it and the registry options
// to read it with.
func (c *Client) resolve(ctx context.Context, family, ref string) (name.Digest, string, []remote.Option, error) {
	repository, err := c.repository(family)
	if err != nil {
		return name.Digest{}, "", nil, fmt.Errorf("parsing %q: %w", family, err)
	}
	options := append([]remote.Option{remote.WithContext(ctx)}, c.remoteOptions...)

	// A digest reference already names what a tag lookup would return, and
	// what is cached downstream is keyed by that digest — so a warm page at a
	// digest costs no registry round trip at all. A digest that names nothing
	// still fails, one call later, when its referrers are listed.
	if strings.Contains(ref, ":") {
		return repository.Digest(ref), ref, options, nil
	}

	// The tag is re-resolved every time — tags move, and serving the wrong
	// Digest would be worse than being slow.
	descriptor, err := c.puller.Head(ctx, repository.Tag(ref))
	if err != nil {
		return name.Digest{}, "", nil, fmt.Errorf("resolving %s:%s: %w", repository, ref, err)
	}
	digest := descriptor.Digest.String()
	return repository.Digest(digest), digest, options, nil
}

// predicate walks what is attached to a Digest and returns the predicate of
// the first attestation that verifies and asserts what want names.
func (c *Client) predicate(subject name.Digest, digest string, options []remote.Option, want attestation.PredicateType) (json.RawMessage, error) {
	var found json.RawMessage
	err := c.eachStatement(subject, digest, options, func(statement *attestation.Statement) bool {
		if statement.Type != want {
			return false
		}
		found = statement.Predicate
		return true
	})
	if err != nil {
		return nil, err
	}
	if found == nil {
		return nil, fmt.Errorf("no verified %s attestation attached to %s", want, subject)
	}
	return found, nil
}

// predicates collects every verified statement attached to a Digest, by type:
// for the attestations that may legitimately be attached more than once — a
// scan re-run, a VEX document reissued — and for the pages that need two kinds
// at once.
func (c *Client) predicates(subject name.Digest, digest string, options []remote.Option) (map[attestation.PredicateType][]*attestation.Statement, error) {
	found := map[attestation.PredicateType][]*attestation.Statement{}
	err := c.eachStatement(subject, digest, options, func(statement *attestation.Statement) bool {
		found[statement.Type] = append(found[statement.Type], statement)
		return false
	})
	return found, err
}

// newestStatement picks the statement signed last, by the log's clock rather
// than anything the document says about itself. Referrer order breaks a tie,
// later winning; nil when there are none.
func newestStatement(statements []*attestation.Statement) *attestation.Statement {
	var newest *attestation.Statement
	for _, statement := range statements {
		if newest == nil || !statement.SignedAt.Before(newest.SignedAt) {
			newest = statement
		}
	}
	return newest
}

// eachStatement verifies every attestation attached to a Digest and hands each
// one to visit, in referrer order, until visit asks to stop.
//
// Cosign gives every attestation the same artifactType, so the only thing that
// tells an SBOM from a scan from provenance is the predicate type inside the
// signed envelope — hence every referrer is verified, and the caller decides
// which it wanted.
func (c *Client) eachStatement(subject name.Digest, digest string, options []remote.Option, visit func(*attestation.Statement) (stop bool)) error {
	referrers, err := remote.Referrers(subject, options...)
	if err != nil {
		return fmt.Errorf("listing referrers of %s: %w", subject, err)
	}
	manifest, err := referrers.IndexManifest()
	if err != nil {
		return fmt.Errorf("reading referrers of %s: %w", subject, err)
	}

	for _, referrer := range manifest.Manifests {
		statement, err := c.statement(subject.Context(), referrer, digest, options)
		switch {
		case errors.Is(err, errUnverified):
			continue
		case err != nil:
			// One unreadable referrer must not hide an attestation behind it.
			slog.Warn("skipping referrer", "subject", subject, "referrer", referrer.Digest, "error", err)
			continue
		}
		if visit(statement) {
			return nil
		}
	}
	return nil
}

// repository locates a family on the registry behind the Mirror.
func (c *Client) repository(family string) (name.Repository, error) {
	path := c.registry + "/"
	if c.repositoryPrefix != "" {
		path += c.repositoryPrefix + "/"
	}
	return name.NewRepository(path+family, c.nameOptions...)
}

// statement pulls one referrer and returns the verified statement it carries:
// the bytes the signature covers, which is what a download serves and what a
// page is projected from.
func (c *Client) statement(repository name.Repository, referrer v1.Descriptor, subjectDigest string, options []remote.Option) (*attestation.Statement, error) {
	image, err := remote.Image(repository.Digest(referrer.Digest.String()), options...)
	if err != nil {
		return nil, err
	}
	layers, err := image.Layers()
	if err != nil {
		return nil, err
	}

	for _, layer := range layers {
		reader, err := layer.Uncompressed()
		if err != nil {
			return nil, err
		}
		blob, err := io.ReadAll(io.LimitReader(reader, maxAttestationBytes))
		reader.Close()
		if err != nil {
			return nil, err
		}

		statement, err := c.verifier.Verify(blob, subjectDigest)
		if err != nil {
			// Unsigned, signed by someone else, or attesting to a different
			// image. All three mean this is not evidence we may show, and
			// none of them is fatal to the search — keep looking.
			slog.Warn("rejecting unverifiable attestation",
				"referrer", referrer.Digest, "subject", subjectDigest, "error", err)
			continue
		}
		return statement, nil
	}
	return nil, errUnverified
}

// decodeBOM parses the predicate of a verified CycloneDX attestation.
//
// Decoding goes through cyclonedx-go rather than an ad-hoc struct: it already
// models the license union and the `cpe` field, and it is the same library the
// build side writes the document with.
func decodeBOM(predicate json.RawMessage) (*cyclonedx.BOM, error) {
	var bom cyclonedx.BOM
	decoder := cyclonedx.NewBOMDecoder(bytes.NewReader(predicate), cyclonedx.BOMFileFormatJSON)
	if err := decoder.Decode(&bom); err != nil {
		return nil, fmt.Errorf("decoding CycloneDX predicate: %w", err)
	}
	return &bom, nil
}

// components projects a CycloneDX document onto what the page shows. Decoding
// already happened, so there is nothing left here that can fail.
func components(bom *cyclonedx.BOM) []directory.Component {
	if bom.Components == nil {
		return nil
	}

	components := make([]directory.Component, 0, len(*bom.Components))
	for _, component := range *bom.Components {
		components = append(components, directory.Component{
			Name:    component.Name,
			Version: component.Version,
			License: licenses(component.Licenses),
			Type:    ecosystem(component.PackageURL),
			Arch:    qualifier(component.PackageURL, "arch"),
			PURL:    component.PackageURL,
			CPE:     component.CPE,
		})
	}
	return components
}

// licenses flattens CycloneDX's license union — an SPDX id, a free-text name,
// or an SPDX expression — into one cell.
func licenses(choices *cyclonedx.Licenses) string {
	if choices == nil {
		return ""
	}
	names := make([]string, 0, len(*choices))
	for _, choice := range *choices {
		switch {
		case choice.License != nil && choice.License.ID != "":
			names = append(names, choice.License.ID)
		case choice.License != nil && choice.License.Name != "":
			names = append(names, choice.License.Name)
		case choice.Expression != "":
			names = append(names, choice.Expression)
		}
	}
	return strings.Join(names, ", ")
}

// qualifier reads one purl qualifier, e.g. the `arch` a deb was built for.
// An Index SBOM covers every architecture at once, so for most Components this
// is the only field distinguishing two otherwise identical entries.
func qualifier(purl, key string) string {
	_, query, ok := strings.Cut(purl, "?")
	if !ok {
		return ""
	}
	values, err := url.ParseQuery(query)
	if err != nil {
		return ""
	}
	return values.Get(key)
}

// ecosystem is the purl type — the thing that decides whether a scanner can
// route a finding to this component at all. An empty one is a silent-zero
// hazard, so it is a column rather than a detail.
func ecosystem(purl string) string {
	rest, ok := strings.CutPrefix(purl, "pkg:")
	if !ok {
		return ""
	}
	ecosystem, _, _ := strings.Cut(rest, "/")
	return ecosystem
}

// scanRecord is one decoded vulnerability attestation and the bytes it came
// from, kept together so the download is the record the page was rendered
// from.
type scanRecord struct {
	scan *directory.Scan
	raw  json.RawMessage
}

// vulnPredicate is cosign's vulnerability scan record
// (https://github.com/sigstore/cosign/blob/main/specs/COSIGN_VULN_ATTESTATION_SPEC.md),
// to the depth the page reads it: who scanned, with what database, when — and
// the scanner's own report as the result.
//
// The result is decoded as grype's document. The spec leaves it to the
// scanner, and grype is the scanner //oci:supply_chain.bzl runs; a record from
// another scanner would decode to no findings, which is why the scanner is
// named on the page.
type vulnPredicate struct {
	Scanner struct {
		URI     string `json:"uri"`
		Version string `json:"version"`
		DB      struct {
			URI     string `json:"uri"`
			Version string `json:"version"`
		} `json:"db"`
		Result grypeDocument `json:"result"`
	} `json:"scanner"`
	Metadata struct {
		ScanStartedOn  string `json:"scanStartedOn"`
		ScanFinishedOn string `json:"scanFinishedOn"`
	} `json:"metadata"`
}

// grypeDocument is grype's JSON report, to the depth the page reads it.
type grypeDocument struct {
	Matches    []grypeMatch `json:"matches"`
	Descriptor struct {
		Name      string `json:"name"`
		Version   string `json:"version"`
		Timestamp string `json:"timestamp"`
		DB        struct {
			Status struct {
				Built string `json:"built"`
			} `json:"status"`
		} `json:"db"`
	} `json:"descriptor"`
}

type grypeMatch struct {
	Vulnerability struct {
		ID          string `json:"id"`
		DataSource  string `json:"dataSource"`
		Severity    string `json:"severity"`
		Description string `json:"description"`
		Fix         struct {
			Versions []string `json:"versions"`
			State    string   `json:"state"`
		} `json:"fix"`
	} `json:"vulnerability"`
	Artifact struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		PURL    string `json:"purl"`
	} `json:"artifact"`
}

// newestScan decodes every scan record on a Digest and keeps the one that ran
// last: a digest is re-scanned whenever the database pin moves, carries every
// scan it ever had, and the newest is what the page shows.
//
// Ordered by the scan's own clock rather than the log's: the two agree in
// practice, and the scan time is what the page states.
func newestScan(statements []*attestation.Statement) (scanRecord, error) {
	var newest scanRecord
	for _, statement := range statements {
		scan, err := decodeScan(statement.Predicate)
		if err != nil {
			slog.Warn("skipping undecodable scan record", "error", err)
			continue
		}
		if newest.scan == nil || scan.Finished.After(newest.scan.Finished) {
			newest = scanRecord{scan: scan, raw: statement.Predicate}
		}
	}
	if newest.scan == nil {
		return scanRecord{}, errors.New("no verified vulnerability scan")
	}
	return newest, nil
}

// decodeScan projects one scan record onto what the page shows.
func decodeScan(predicate json.RawMessage) (*directory.Scan, error) {
	var record vulnPredicate
	if err := json.Unmarshal(predicate, &record); err != nil {
		return nil, fmt.Errorf("decoding vulnerability predicate: %w", err)
	}
	result := record.Scanner.Result

	scanner := strings.TrimSpace(result.Descriptor.Name + " " + result.Descriptor.Version)
	if scanner == "" {
		scanner = record.Scanner.URI
	}

	findings := make([]directory.Finding, 0, len(result.Matches))
	for _, match := range result.Matches {
		findings = append(findings, directory.Finding{
			ID:          match.Vulnerability.ID,
			Severity:    match.Vulnerability.Severity,
			Package:     match.Artifact.Name,
			Version:     match.Artifact.Version,
			Type:        ecosystem(match.Artifact.PURL),
			Arch:        qualifier(match.Artifact.PURL, "arch"),
			PURL:        match.Artifact.PURL,
			FixedIn:     match.Vulnerability.Fix.Versions,
			FixState:    match.Vulnerability.Fix.State,
			URL:         match.Vulnerability.DataSource,
			Description: match.Vulnerability.Description,
		})
	}

	return &directory.Scan{
		Scanner: scanner,
		// The record names the database by when it was built, which is the
		// freshness a reader needs; grype's own descriptor is the fallback
		// for a record wrapped without it.
		Database: firstTime(record.Scanner.DB.Version, result.Descriptor.DB.Status.Built),
		Finished: firstTime(record.Metadata.ScanFinishedOn, result.Descriptor.Timestamp),
		Findings: findings,
	}, nil
}

// firstTime parses the first of candidates that is an RFC 3339 timestamp. Zero
// when none is, which the page shows as a gap rather than as an error.
func firstTime(candidates ...string) time.Time {
	for _, candidate := range candidates {
		if parsed, err := time.Parse(time.RFC3339Nano, candidate); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

// openVEX is an OpenVEX document to the depth the join reads it.
type openVEX struct {
	Statements []struct {
		Vulnerability struct {
			Name string `json:"name"`
		} `json:"vulnerability"`
		Status          string `json:"status"`
		Justification   string `json:"justification"`
		ImpactStatement string `json:"impact_statement"`
	} `json:"statements"`
}

// suppress marks the findings a VEX document silences.
//
// Only not_affected and fixed silence — the two statuses grype itself drops a
// match for. An affected or under_investigation statement is a statement too,
// and it leaves the finding standing.
func suppress(scan *directory.Scan, document json.RawMessage) error {
	var vex openVEX
	if err := json.Unmarshal(document, &vex); err != nil {
		return fmt.Errorf("decoding OpenVEX document: %w", err)
	}
	byID := make(map[string]directory.Suppression, len(vex.Statements))
	for _, statement := range vex.Statements {
		if statement.Status != "not_affected" && statement.Status != "fixed" {
			continue
		}
		byID[statement.Vulnerability.Name] = directory.Suppression{
			Status:        statement.Status,
			Justification: statement.Justification,
			Impact:        statement.ImpactStatement,
		}
	}
	for i := range scan.Findings {
		if suppression, ok := byID[scan.Findings[i].ID]; ok {
			scan.Findings[i].Suppressed = &suppression
		}
	}
	return nil
}
