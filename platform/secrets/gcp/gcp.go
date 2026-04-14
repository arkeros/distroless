package gcp

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
)

// SecretPattern validates the GCP Secret Manager resource name format.
var SecretPattern = regexp.MustCompile(`^projects/[^/]+/secrets/[^/]+/versions/\d+$`)

// URI returns a gcp:// provider URI for the given secret.
func URI(project, name, version string) string {
	return (&url.URL{
		Scheme: "gcp",
		Path:   "/projects/" + project + "/secrets/" + name + "/versions/" + version,
	}).String()
}

// SecretRef holds the parsed components of a GCP Secret Manager reference.
type SecretRef struct {
	Project string
	Name    string
	Version string
}

// NewSecretRef parses a gcp:// URI string into its components.
func NewSecretRef(uri string) (SecretRef, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return SecretRef{}, fmt.Errorf("invalid URI %q: %v", uri, err)
	}
	return NewSecretRefFromURL(u)
}

// NewSecretRefFromURL parses a pre-parsed gcp:// URL into its components.
func NewSecretRefFromURL(u *url.URL) (SecretRef, error) {
	if u.Scheme != "gcp" {
		return SecretRef{}, fmt.Errorf("expected gcp:// scheme, got %q", u.Scheme)
	}
	ref := strings.TrimPrefix(u.Path, "/")
	if !SecretPattern.MatchString(ref) {
		return SecretRef{}, fmt.Errorf("GCP secret reference %q does not match pattern projects/{project}/secrets/{name}/versions/{number}", ref)
	}
	parts := strings.Split(ref, "/")
	return SecretRef{
		Project: parts[1],
		Name:    parts[3],
		Version: parts[5],
	}, nil
}

// NewProvider returns a provider that fetches secrets from GCP Secret Manager,
// and a cleanup function that closes the underlying client.
// URI: gcp:///projects/{project}/secrets/{name}/versions/{number}
func NewProvider() (func(context.Context, *url.URL) ([]byte, error), func()) {
	var client *secretmanager.Client
	cleanup := func() {
		if client != nil {
			client.Close()
		}
	}
	provider := func(ctx context.Context, u *url.URL) ([]byte, error) {
		ref := strings.TrimPrefix(u.Path, "/")
		if !SecretPattern.MatchString(ref) {
			return nil, fmt.Errorf("GCP secret reference %q does not match pattern projects/{project}/secrets/{name}/versions/{number} (numeric version required, no 'latest')", ref)
		}
		if client == nil {
			c, err := secretmanager.NewClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("create GCP Secret Manager client: %v", err)
			}
			client = c
		}
		resp, err := client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{Name: ref})
		if err != nil {
			return nil, err
		}
		return resp.GetPayload().GetData(), nil
	}
	return provider, cleanup
}
