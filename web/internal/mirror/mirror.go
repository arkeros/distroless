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

// maxAttestationBytes caps how much of a referrer blob is read. A full SBOM
// for a language runtime runs to a few MB; anything past this is not one.
const maxAttestationBytes = 64 << 20

// errNotSBOM marks a referrer that verified fine and simply asserts something
// else — the SLSA provenance attached to the same Digest, most often.
var errNotSBOM = errors.New("no CycloneDX attestation on this referrer")

// Verifier establishes that an attestation blob was signed by the identity
// permitted to publish to the Mirror, and reports what it asserts.
type Verifier interface {
	Verify(blob []byte, subjectDigest string) (*attestation.Statement, error)
}

// Client reads SBOMs off a registry holding the Mirror's images.
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
}

// Option configures a Client.
type Option func(*Client)

// Insecure resolves references over plain HTTP. For tests.
func Insecure() Option {
	return func(c *Client) {
		c.nameOptions = append(c.nameOptions, name.Insecure)
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
	c := &Client{registry: registry, repositoryPrefix: repositoryPrefix, verifier: verifier}
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

	predicate, err := c.predicate(subject, digest, options)
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
	predicate, err := c.predicate(subject, digest, options)
	if err != nil {
		return "", nil, err
	}
	return digest, predicate, nil
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

// predicate walks what is attached to a Digest and returns the CycloneDX
// document from the first attestation that verifies.
func (c *Client) predicate(subject name.Digest, digest string, options []remote.Option) (json.RawMessage, error) {
	referrers, err := remote.Referrers(subject, options...)
	if err != nil {
		return nil, fmt.Errorf("listing referrers of %s: %w", subject, err)
	}
	manifest, err := referrers.IndexManifest()
	if err != nil {
		return nil, fmt.Errorf("reading referrers of %s: %w", subject, err)
	}

	// Cosign gives the SBOM and the SLSA provenance the same artifactType, so
	// the only thing that tells them apart is the predicate type inside the
	// signed envelope. Hence: verify each, keep the one that is an SBOM.
	for _, referrer := range manifest.Manifests {
		found, err := c.sbom(subject.Context(), referrer, digest, options)
		switch {
		case errors.Is(err, errNotSBOM):
			continue
		case err != nil:
			// One unreadable referrer must not hide an SBOM behind it.
			slog.Warn("skipping referrer", "subject", subject, "referrer", referrer.Digest, "error", err)
			continue
		}
		return found, nil
	}

	return nil, fmt.Errorf("no verified CycloneDX attestation attached to %s", subject)
}

// repository locates a family on the registry behind the Mirror.
func (c *Client) repository(family string) (name.Repository, error) {
	path := c.registry + "/"
	if c.repositoryPrefix != "" {
		path += c.repositoryPrefix + "/"
	}
	return name.NewRepository(path+family, c.nameOptions...)
}

// sbom pulls one referrer, verifies the attestation it carries, and returns
// the CycloneDX predicate inside it — the bytes the signature covers, which is
// what the download serves and what the page is projected from.
func (c *Client) sbom(repository name.Repository, referrer v1.Descriptor, subjectDigest string, options []remote.Option) (json.RawMessage, error) {
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
		if statement.Type != attestation.CycloneDX {
			continue
		}
		return statement.Predicate, nil
	}
	return nil, errNotSBOM
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
