// Code generated from //oci:cosign_policy.bzl. DO NOT EDIT.

// Package policy pins the identity a Mirror attestation must be signed by.
package policy

const (
	// CertificateIdentityRegexp matches the OIDC subject of the one
	// workflow permitted to sign for the Mirror.
	CertificateIdentityRegexp = `^https://github\.com/arkeros/distroless/\.github/workflows/ci\.yaml@refs/heads/main$`

	// CertificateOIDCIssuer is the issuer that certificate must come from.
	CertificateOIDCIssuer = "https://token.actions.githubusercontent.com"
)
