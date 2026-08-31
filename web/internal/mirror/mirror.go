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
	"strings"

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

// SBOM resolves image to the Digest behind it and returns the Components of
// the verified CycloneDX attestation attached to that Digest.
//
// Attestations are bound to a Digest, never to a tag, so the digest is
// returned alongside: it is what the page was actually rendered from.
func (c *Client) SBOM(ctx context.Context, image string) (string, []directory.Component, error) {
	reference, err := c.reference(image)
	if err != nil {
		return "", nil, fmt.Errorf("parsing %q: %w", image, err)
	}
	options := append([]remote.Option{remote.WithContext(ctx)}, c.remoteOptions...)

	descriptor, err := c.puller.Head(ctx, reference)
	if err != nil {
		return "", nil, fmt.Errorf("resolving %s: %w", reference, err)
	}
	digest := descriptor.Digest.String()

	// The tag is re-resolved every time — tags move, and serving the wrong
	// Digest would be worse than being slow. What is cached is keyed by the
	// Digest itself, which cannot change underneath us.
	if components, ok := c.cache.Get(digest); ok {
		return digest, components, nil
	}

	subject := reference.Context().Digest(digest)

	referrers, err := remote.Referrers(subject, options...)
	if err != nil {
		return "", nil, fmt.Errorf("listing referrers of %s: %w", subject, err)
	}
	manifest, err := referrers.IndexManifest()
	if err != nil {
		return "", nil, fmt.Errorf("reading referrers of %s: %w", subject, err)
	}

	// Cosign gives the SBOM and the SLSA provenance the same artifactType, so
	// the only thing that tells them apart is the predicate type inside the
	// signed envelope. Hence: verify each, keep the one that is an SBOM.
	for _, referrer := range manifest.Manifests {
		bom, err := c.sbom(reference.Context(), referrer, digest, options)
		switch {
		case errors.Is(err, errNotSBOM):
			continue
		case err != nil:
			// One unreadable referrer must not hide an SBOM behind it.
			slog.Warn("skipping referrer", "subject", subject, "referrer", referrer.Digest, "error", err)
			continue
		}
		resolved := components(bom)
		c.cache.Add(digest, resolved)
		return digest, resolved, nil
	}

	return "", nil, fmt.Errorf("no verified CycloneDX attestation attached to %s", subject)
}

func (c *Client) reference(image string) (name.Reference, error) {
	path := c.registry + "/"
	if c.repositoryPrefix != "" {
		path += c.repositoryPrefix + "/"
	}
	return name.ParseReference(path+image, c.nameOptions...)
}

// sbom pulls one referrer, verifies the attestation it carries, and returns
// the CycloneDX document inside it.
func (c *Client) sbom(repository name.Repository, referrer v1.Descriptor, subjectDigest string, options []remote.Option) (*cyclonedx.BOM, error) {
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
		return decodeBOM(statement.Predicate)
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
