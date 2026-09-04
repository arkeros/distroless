package attestation_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/testing/ca"
	"github.com/sigstore/sigstore-go/pkg/verify"

	"github.com/arkeros/distroless/web/internal/attestation"
)

// The policy //oci:cosign_policy.bzl pins. Only this workflow, on this ref,
// may sign for the Mirror.
const (
	issuer          = "https://token.actions.githubusercontent.com"
	signingIdentity = "https://github.com/arkeros/distroless/.github/workflows/ci.yaml@refs/heads/main"
	identityRegexp  = `^https://github\.com/arkeros/distroless/\.github/workflows/ci\.yaml@refs/heads/main$`
)

// statement builds the in-toto statement an SBOM attestation carries, bound to
// the digest of the Index it describes.
func statement(t *testing.T, subjectDigest string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"_type": "https://in-toto.io/Statement/v1",
		"subject": []any{map[string]any{
			"name":   "ghcr.io/arkeros/distroless/node",
			"digest": map[string]string{"sha256": strings.TrimPrefix(subjectDigest, "sha256:")},
		}},
		"predicateType": string(attestation.CycloneDX),
		"predicate": map[string]any{
			"bomFormat":   "CycloneDX",
			"specVersion": "1.6",
			"components":  []any{map[string]any{"name": "openssl", "version": "3.0.11"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// The virtual Sigstore issues no signed certificate timestamps, so the SCT
// leg of Checks() cannot be exercised here. Everything this package actually
// owns — the identity regexp, the issuer, and the subject-digest binding — is.
var virtualChecks = []verify.VerifierOption{
	verify.WithTransparencyLog(1),
	verify.WithObserverTimestamps(1),
}

func digestOf(content string) string {
	sum := sha256.Sum256([]byte(content))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func TestStatementAcceptsAttestationFromTheSigningWorkflow(t *testing.T) {
	sigstore, err := ca.NewVirtualSigstore()
	if err != nil {
		t.Fatal(err)
	}
	digest := digestOf("index")
	entity, err := sigstore.Attest(signingIdentity, issuer, statement(t, digest))
	if err != nil {
		t.Fatal(err)
	}

	verifier, err := attestation.NewWithTrustedMaterial(sigstore, identityRegexp, issuer, virtualChecks...)
	if err != nil {
		t.Fatal(err)
	}

	statement, err := verifier.VerifyEntity(entity, digest)
	if err != nil {
		t.Fatalf("VerifyEntity: %v", err)
	}
	if statement.Type != attestation.CycloneDX {
		t.Errorf("type = %q, want %q", statement.Type, attestation.CycloneDX)
	}
	if !strings.Contains(string(statement.Predicate), "openssl") {
		t.Errorf("predicate did not survive verification: %s", statement.Predicate)
	}
	// The log's word on when this was signed, which is what orders two
	// attestations of the same kind on one digest. A statement carries no
	// date of its own that anyone but its author vouches for.
	if statement.SignedAt.IsZero() {
		t.Error("SignedAt is zero; the verified timestamp was not carried over")
	}
}

// The signature being valid is not enough — it has to be *this project's* CI.
func TestStatementRejectsAnotherSigner(t *testing.T) {
	sigstore, err := ca.NewVirtualSigstore()
	if err != nil {
		t.Fatal(err)
	}
	digest := digestOf("index")
	impostor := "https://github.com/attacker/distroless/.github/workflows/ci.yaml@refs/heads/main"
	entity, err := sigstore.Attest(impostor, issuer, statement(t, digest))
	if err != nil {
		t.Fatal(err)
	}

	verifier, err := attestation.NewWithTrustedMaterial(sigstore, identityRegexp, issuer, virtualChecks...)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := verifier.VerifyEntity(entity, digest); err == nil {
		t.Error("accepted an attestation signed by another repository's workflow")
	}
}

// A genuine attestation about a *different* image is not evidence about this
// one, so the subject digest is part of the policy, not just the lookup.
func TestStatementRejectsAttestationForAnotherDigest(t *testing.T) {
	sigstore, err := ca.NewVirtualSigstore()
	if err != nil {
		t.Fatal(err)
	}
	signed, requested := digestOf("index-a"), digestOf("index-b")
	entity, err := sigstore.Attest(signingIdentity, issuer, statement(t, signed))
	if err != nil {
		t.Fatal(err)
	}

	verifier, err := attestation.NewWithTrustedMaterial(sigstore, identityRegexp, issuer, virtualChecks...)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := verifier.VerifyEntity(entity, requested); err == nil {
		t.Errorf("accepted an attestation whose subject is %s when asked about %s", signed, requested)
	}
}

// The embedded trusted root has to actually parse — if the pinned file ever
// stops being a trusted root, that is a build-time fact, not a 3am one.
func TestEmbeddedTrustedRootLoads(t *testing.T) {
	if _, err := attestation.New(identityRegexp, issuer); err != nil {
		t.Fatalf("embedded Sigstore trusted root did not load: %v", err)
	}
}
