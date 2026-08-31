package apt

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/arkeros/distroless/oci/debian/copyright"
	"github.com/arkeros/distroless/oci/debian/deb"
	"github.com/arkeros/distroless/oci/debian/lockfile"
)

// packageLicense is what one package declares about itself. Both fields empty
// means the package was inspected and its licence could not be determined —
// which is deliberately different from the package being absent, where nobody
// looked at all.
type packageLicense struct {
	SPDX string `json:"spdx,omitempty"`
	Name string `json:"name,omitempty"`
}

type licenseFile struct {
	Packages map[string]packageLicense `json:"packages"`
}

type licensesOptions struct {
	Paths   []string
	Output  string
	Workers int
}

func newCmdLicenses() *cobra.Command {
	o := &licensesOptions{}

	cmd := &cobra.Command{
		Use:   "licenses <lock-file>...",
		Short: "Resolve package licenses from apt lock files",
		Long: `Downloads each package in the given apt lock files and reads the license it
declares in usr/share/doc/<name>/copyright.

Several lock files may be given, and they are resolved into one table keyed by
package name: a Debian package declares the same license wherever it is pulled
from, so one table serves every image.

Debian's package index carries no license field, so the copyright file is the
only in-band source, and only about three quarters of packages ship it in the
machine-readable DEP-5 format. Packages whose license cannot be determined are
recorded with an empty entry, so that "we looked and could not tell" is
distinguishable from "we never looked".

Run this when the lock file changes; the result is checked in so a license
change shows up as a reviewable diff.

Examples:
  knife apt licenses images/debian.lock.json
  knife apt licenses -o images/debian.licenses.json images/*.lock.json images/nginx/*.lock.json`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o.Paths = args
			return o.Run()
		},
	}

	cmd.Flags().StringVarP(&o.Output, "output", "o", "", "write to this file instead of stdout")
	cmd.Flags().IntVar(&o.Workers, "workers", 8, "how many packages to download at once")

	return cmd
}

func (o *licensesOptions) Run() error {
	// The copyright file is identical across architectures, and a package
	// declares the same license wherever it is pulled from — so resolve each
	// name once, however many locks and arches mention it.
	unique := make(map[string]lockfile.Package)
	for _, path := range o.Paths {
		lock, err := lockfile.ParseFile(path)
		if err != nil {
			return err
		}
		for _, pkg := range lock.Packages {
			if _, seen := unique[pkg.Name]; !seen {
				unique[pkg.Name] = pkg
			}
		}
	}
	names := make([]string, 0, len(unique))
	for name := range unique {
		names = append(names, name)
	}
	sort.Strings(names)

	var (
		mu       sync.Mutex
		resolved = make(map[string]packageLicense, len(names))
		work     = make(chan string)
		wg       sync.WaitGroup
		failures []error
	)
	client := &http.Client{Timeout: 5 * time.Minute}

	for range o.Workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for name := range work {
				license, err := resolveLicense(client, unique[name])
				mu.Lock()
				if err != nil {
					failures = append(failures, fmt.Errorf("%s: %w", name, err))
				} else {
					resolved[name] = license
				}
				mu.Unlock()
			}
		}()
	}
	for _, name := range names {
		work <- name
	}
	close(work)
	wg.Wait()

	// A download or checksum failure is fatal: a partial file would silently
	// drop licences we do know, and the result is checked in.
	if len(failures) > 0 {
		return fmt.Errorf("resolving licenses: %w", errors.Join(failures...))
	}

	// json.Marshal sorts map keys, so the output is stable across runs and
	// diffs cleanly.
	encoded, err := json.MarshalIndent(licenseFile{Packages: resolved}, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')

	if o.Output == "" {
		_, err := os.Stdout.Write(encoded)
		return err
	}
	return os.WriteFile(o.Output, encoded, 0o644)
}

// resolveLicense downloads one package and reads the licence it declares.
//
// An undeterminable licence is not an error: it is the expected outcome for
// roughly a quarter of Debian packages, and is recorded as an empty entry.
func resolveLicense(client *http.Client, pkg lockfile.Package) (packageLicense, error) {
	archive, err := download(client, pkg)
	if err != nil {
		return packageLicense{}, err
	}

	text, err := deb.Copyright(bytes.NewReader(archive))
	if errors.Is(err, deb.ErrNoCopyright) {
		return packageLicense{}, nil
	}
	if err != nil {
		return packageLicense{}, err
	}

	license, err := copyright.Parse(bytes.NewReader(text))
	if errors.Is(err, copyright.ErrNotMachineReadable) || errors.Is(err, copyright.ErrNoOverallLicense) {
		return packageLicense{}, nil
	}
	if err != nil {
		return packageLicense{}, err
	}
	return packageLicense{SPDX: license.SPDX, Name: license.Name}, nil
}

// download fetches a package and checks it against the digest the lock file
// pins, so a mirror cannot substitute different bytes than the ones the image
// is built from.
func download(client *http.Client, pkg lockfile.Package) ([]byte, error) {
	var lastErr error
	for _, url := range pkg.URLs {
		response, err := client.Get(url)
		if err != nil {
			lastErr = err
			continue
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if response.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("GET %s: %s", url, response.Status)
			continue
		}

		digest := sha256.Sum256(body)
		if got := hex.EncodeToString(digest[:]); got != pkg.SHA256 {
			return nil, fmt.Errorf("GET %s: sha256 %s, lock pins %s", url, got, pkg.SHA256)
		}
		return body, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no download URLs in lock file")
	}
	return nil, lastErr
}
