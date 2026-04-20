package prebuilts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/google/go-github/v85/github"
	"github.com/spf13/cobra"

	"github.com/arkeros/senku/bazel/pkg/toolversions"
)

// toolConfig describes how a given CLI's prebuilt artifacts are published
// and where its versions.bzl lives in this workspace.
type toolConfig struct {
	// Name is the lowercase tool name; also the tag prefix (e.g. "bifrost"
	// matches tags "bifrost/vX.Y.Z") and the asset filename prefix.
	Name string
	// Owner is the GitHub owner of the releases repo.
	Owner string
	// Repo is the GitHub repo name.
	Repo string
	// OutPath is the versions.bzl path relative to the workspace root.
	OutPath string
	// Platforms lists artifact platform suffixes, e.g. "linux-amd64".
	Platforms []string
}

func (c toolConfig) URLTemplate() string {
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/v%%s/%%s", c.Owner, c.Repo, c.Name)
}

// defaultPlatforms matches the matrix in .github/workflows/release-cli.yaml.
var defaultPlatforms = []string{"darwin-amd64", "darwin-arm64", "linux-amd64", "linux-arm64"}

var tools = map[string]toolConfig{
	"bifrost": {
		Name:      "bifrost",
		Owner:     "arkeros",
		Repo:      "senku",
		OutPath:   "devtools/bifrost/toolchain/versions.bzl",
		Platforms: defaultPlatforms,
	},
}

type updateOptions struct {
	Tool string
	Out  string
}

func newCmdUpdate() *cobra.Command {
	o := &updateOptions{}
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Regenerate versions.bzl from GitHub Releases",
		Long: `Fetch every release of a prebuilt CLI tool, compute SHA-256 for each
platform artifact, and rewrite its versions.bzl with buildtools formatting.

Example:
  knife prebuilts update --tool bifrost

Authenticates to GitHub via GITHUB_TOKEN or GH_TOKEN if set; otherwise uses
anonymous API access (subject to lower rate limits).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.Run(cmd.Context())
		},
	}
	cmd.Flags().StringVar(&o.Tool, "tool", "", "tool name (required; known: "+strings.Join(knownTools(), ", ")+")")
	cmd.Flags().StringVar(&o.Out, "out", "", "override for versions.bzl path (default: the tool's registered path)")
	_ = cmd.MarkFlagRequired("tool")
	return cmd
}

func knownTools() []string {
	out := make([]string, 0, len(tools))
	for k := range tools {
		out = append(out, k)
	}
	return out
}

func (o *updateOptions) Run(ctx context.Context) error {
	cfg, ok := tools[o.Tool]
	if !ok {
		return fmt.Errorf("unknown tool %q (known: %s)", o.Tool, strings.Join(knownTools(), ", "))
	}

	out := o.Out
	if out == "" {
		out = cfg.OutPath
	}
	if wsDir := os.Getenv("BUILD_WORKSPACE_DIRECTORY"); wsDir != "" && !filepath.IsAbs(out) {
		out = filepath.Join(wsDir, out)
	}

	gh := github.NewClient(nil)
	if tok := firstNonEmpty(os.Getenv("GITHUB_TOKEN"), os.Getenv("GH_TOKEN")); tok != "" {
		gh = gh.WithAuthToken(tok)
	}

	slog.Info("Listing releases", "tool", cfg.Name, "owner", cfg.Owner, "repo", cfg.Repo)
	ghReleases, err := listReleases(ctx, gh, cfg)
	if err != nil {
		return err
	}
	if len(ghReleases) == 0 {
		return fmt.Errorf("no releases found for %s in %s/%s", cfg.Name, cfg.Owner, cfg.Repo)
	}
	slog.Info("Found releases", "count", len(ghReleases))

	releases, err := collectReleases(ctx, gh, cfg, ghReleases)
	if err != nil {
		return err
	}

	defaultVersion := slices.MaxFunc(releases, func(a, b toolversions.Release) int {
		return toolversions.CompareVersions(a.Version, b.Version)
	}).Version

	if err := toolversions.Write(out, toolversions.Config{
		Tool:        cfg.Name,
		URLTemplate: cfg.URLTemplate(),
	}, releases, defaultVersion); err != nil {
		return err
	}

	fmt.Printf("✓ Wrote %d releases to %s (default %s)\n", len(releases), out, defaultVersion)
	return nil
}

// listReleases paginates through every release of the repo and returns only
// those whose tag matches the tool's prefix.
func listReleases(ctx context.Context, gh *github.Client, cfg toolConfig) ([]*github.RepositoryRelease, error) {
	prefix := cfg.Name + "/"
	var out []*github.RepositoryRelease
	opt := &github.ListOptions{PerPage: 100}
	for {
		page, resp, err := gh.Repositories.ListReleases(ctx, cfg.Owner, cfg.Repo, opt)
		if err != nil {
			return nil, fmt.Errorf("list releases: %w", err)
		}
		for _, r := range page {
			if strings.HasPrefix(r.GetTagName(), prefix) {
				out = append(out, r)
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return out, nil
}

func collectReleases(ctx context.Context, gh *github.Client, cfg toolConfig, ghReleases []*github.RepositoryRelease) ([]toolversions.Release, error) {
	releases := make([]toolversions.Release, 0, len(ghReleases))
	for _, rel := range ghReleases {
		version := strings.TrimPrefix(rel.GetTagName(), cfg.Name+"/v")
		slog.Info("Processing release", "tag", rel.GetTagName())

		byName := make(map[string]*github.ReleaseAsset, len(rel.Assets))
		for _, a := range rel.Assets {
			byName[a.GetName()] = a
		}

		var arts []toolversions.Artifact
		for _, plat := range cfg.Platforms {
			filename := cfg.Name + "-" + plat
			asset, ok := byName[filename]
			if !ok {
				slog.Warn("Artifact missing", "tag", rel.GetTagName(), "filename", filename)
				continue
			}
			sum, err := downloadAndHash(ctx, gh, cfg, asset.GetID())
			if err != nil {
				return nil, fmt.Errorf("%s asset %s: %w", rel.GetTagName(), filename, err)
			}
			arts = append(arts, toolversions.Artifact{
				Platform: strings.ReplaceAll(plat, "-", "_"),
				Filename: filename,
				SHA256:   sum,
			})
		}
		if len(arts) == 0 {
			slog.Warn("Skipping release with no artifacts", "tag", rel.GetTagName())
			continue
		}
		releases = append(releases, toolversions.Release{
			Version:   version,
			Artifacts: arts,
		})
	}
	return releases, nil
}

func downloadAndHash(ctx context.Context, gh *github.Client, cfg toolConfig, assetID int64) (string, error) {
	// Passing a non-nil follow-redirect client makes DownloadReleaseAsset
	// always return a reader, so we never need the redirect-URL fallback.
	rc, _, err := gh.Repositories.DownloadReleaseAsset(ctx, cfg.Owner, cfg.Repo, assetID, http.DefaultClient)
	if err != nil {
		return "", err
	}
	defer rc.Close()

	h := sha256.New()
	if _, err := io.Copy(h, rc); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
