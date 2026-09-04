// Package attestation establishes that an Attestation really was signed by
// this project's CI before anything renders what it claims.
//
// The point is narrow but load-bearing: an unverified attestation is a
// document that looks like evidence. Rendering one would be the same category
// of mistake as a Silent zero — presenting an absence of checking as a result.
//
// The Sigstore trusted root is embedded rather than fetched from TUF at
// runtime. It carries no expiry, so it does not age out; and a page whose
// subject is supply-chain evidence should not stop rendering because
// tuf-repo-cdn.sigstore.dev is having a bad day. See
// //bazel/include:oci.MODULE.bazel for how it is pinned and refreshed.
package attestation

import (
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

//go:embed trusted_root.json
var trustedRootJSON []byte

// PredicateType identifies what an Attestation asserts about its subject.
// Cosign gives every attestation the same artifactType on the registry, so
// this — read from inside the signed envelope — is the only thing that
// distinguishes an SBOM from build provenance.
type PredicateType string

const (
	// CycloneDX is the SBOM `mirror_push` attaches via `cosign attest
	// --type=cyclonedx`.
	CycloneDX PredicateType = "https://cyclonedx.org/bom"
	// SLSAProvenance is the platform provenance attached to the same Digest.
	// The generator emits SLSA v0.2, which is also what `cosign
	// --type=slsaprovenance` names; the Directory does not render it yet.
	SLSAProvenance PredicateType = "https://slsa.dev/provenance/v0.2"
	// Vuln is the vulnerability scan record `mirror_push` attaches via
	// `cosign attest --type=vuln`: cosign's own predicate, which wraps a
	// scanner's report in who ran it, with what database, and when.
	Vuln PredicateType = "https://cosign.sigstore.dev/attestation/vuln/v1"
	// OpenVEX is the VEX document attached via `cosign attest --type=openvex`
	// — this project's statements about which findings do not affect the
	// image. Cosign names the predicate by the OpenVEX namespace, without a
	// version; the document's own `@context` carries that.
	OpenVEX PredicateType = "https://openvex.dev/ns"
)

// Statement is a verified in-toto statement.
//
// Predicate stays encoded on purpose: this package establishes *that* an
// Attestation is trustworthy and *what kind* it is, and is deliberately
// incurious about its contents. Callers dispatch on Type and decode the
// predicate with a schema they own.
type Statement struct {
	Type      PredicateType
	Predicate json.RawMessage
	// SignedAt is when the signature was observed — the earliest verified
	// timestamp, from the transparency log or a timestamp authority. It is
	// the one date about an attestation that its author did not write, which
	// makes it the way to order two attestations of the same kind on one
	// digest: the newer VEX document supersedes the older. Zero when the
	// verifier was configured to require no timestamp at all.
	SignedAt time.Time
}

// Verifier checks Sigstore bundles against the Mirror's signing policy.
type Verifier struct {
	verifier *verify.Verifier
	identity verify.CertificateIdentity
}

// New builds a Verifier against the embedded Sigstore trusted root.
//
// certificateIdentityRegexp and oidcIssuer come from //oci:cosign_policy.bzl —
// the same constants `mirror_push` signs with and CI verifies with, so the
// page cannot drift from the policy the images were published under.
func New(certificateIdentityRegexp, oidcIssuer string) (*Verifier, error) {
	trustedRoot, err := root.NewTrustedRootFromJSON(trustedRootJSON)
	if err != nil {
		return nil, fmt.Errorf("loading embedded Sigstore trusted root: %w", err)
	}
	return NewWithTrustedMaterial(trustedRoot, certificateIdentityRegexp, oidcIssuer)
}

// Checks is the evidence a bundle must carry before its contents are shown:
// logged to Rekor, timestamped by an observer, and signed under a certificate
// that was itself logged to a CT log.
//
// Deliberately the same set `cosign verify-attestation` applies by default, so
// this page cannot accept an attestation that CI would reject.
func Checks() []verify.VerifierOption {
	return []verify.VerifierOption{
		verify.WithTransparencyLog(1),
		verify.WithObserverTimestamps(1),
		verify.WithSignedCertificateTimestamps(1),
	}
}

// NewWithTrustedMaterial builds a Verifier against trust material supplied by
// the caller, so tests can drive the real verification path with a virtual
// Sigstore instead of the public good instance.
//
// Passing no checks applies Checks(). Tests override them because the virtual
// Sigstore issues no SCTs; the SCT check itself belongs to sigstore-go and is
// covered by that library's own suite, not re-tested here.
func NewWithTrustedMaterial(material root.TrustedMaterial, certificateIdentityRegexp, oidcIssuer string, checks ...verify.VerifierOption) (*Verifier, error) {
	if len(checks) == 0 {
		checks = Checks()
	}
	verifier, err := verify.NewVerifier(material, checks...)
	if err != nil {
		return nil, fmt.Errorf("building verifier: %w", err)
	}

	identity, err := verify.NewShortCertificateIdentity(oidcIssuer, "", "", certificateIdentityRegexp)
	if err != nil {
		return nil, fmt.Errorf("building certificate identity policy: %w", err)
	}

	return &Verifier{verifier: verifier, identity: identity}, nil
}

// Verify parses a Sigstore bundle blob and establishes that it was signed by
// the permitted identity for subjectDigest.
func (v *Verifier) Verify(blob []byte, subjectDigest string) (*Statement, error) {
	var parsed bundle.Bundle
	if err := parsed.UnmarshalJSON(blob); err != nil {
		return nil, fmt.Errorf("parsing Sigstore bundle: %w", err)
	}
	return v.VerifyEntity(&parsed, subjectDigest)
}

// VerifyEntity verifies an already-parsed signed entity.
//
// Verification is bound to subjectDigest as well as to the signing identity: a
// bundle can be perfectly valid and still be an attestation about a different
// image, which would be evidence about something the reader did not ask for.
func (v *Verifier) VerifyEntity(entity verify.SignedEntity, subjectDigest string) (*Statement, error) {
	digest, err := hex.DecodeString(strings.TrimPrefix(subjectDigest, "sha256:"))
	if err != nil {
		return nil, fmt.Errorf("parsing subject digest %q: %w", subjectDigest, err)
	}

	result, err := v.verifier.Verify(entity, verify.NewPolicy(
		verify.WithArtifactDigest("sha256", digest),
		verify.WithCertificateIdentity(v.identity),
	))
	if err != nil {
		return nil, fmt.Errorf("verifying attestation for %s: %w", subjectDigest, err)
	}
	if result.Statement == nil {
		return nil, fmt.Errorf("verified bundle for %s carries no in-toto statement", subjectDigest)
	}

	predicate, err := result.Statement.Predicate.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("re-encoding predicate of %s: %w", subjectDigest, err)
	}
	var signedAt time.Time
	for _, verified := range result.VerifiedTimestamps {
		if signedAt.IsZero() || verified.Timestamp.Before(signedAt) {
			signedAt = verified.Timestamp
		}
	}
	return &Statement{
		Type:      PredicateType(result.Statement.PredicateType),
		Predicate: predicate,
		SignedAt:  signedAt,
	}, nil
}
