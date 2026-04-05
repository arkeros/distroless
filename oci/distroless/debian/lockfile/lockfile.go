package lockfile

import (
	"encoding/json"
	"fmt"
	"os"
)

type Package struct {
	Arch    string `json:"arch"`
	Key     string `json:"key"`
	Name    string `json:"name"`
	SHA256  string `json:"sha256"`
	URLs    []string `json:"urls"`
	Version string `json:"version"`
}

type LockFile struct {
	Packages []Package `json:"packages"`
}

func ParseFile(path string) (*LockFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read lock file: %w", err)
	}

	var lock LockFile
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("failed to parse lock file: %w", err)
	}

	return &lock, nil
}

// Versions returns a map of package name to version. When a package has the
// same version across all architectures, it appears once under its plain name.
// When versions differ by architecture, separate entries are created with
// "name (arch)" keys.
func (l *LockFile) Versions() map[string]string {
	byArch := l.VersionsByArch()

	// Collect all unique versions per package across arches
	pkgVersions := make(map[string]map[string]string) // name -> arch -> version
	for arch, packages := range byArch {
		for name, version := range packages {
			if pkgVersions[name] == nil {
				pkgVersions[name] = make(map[string]string)
			}
			pkgVersions[name][arch] = version
		}
	}

	versions := make(map[string]string)
	for name, archVersions := range pkgVersions {
		// Check if all arches have the same version
		var first string
		allSame := true
		for _, v := range archVersions {
			if first == "" {
				first = v
			} else if v != first {
				allSame = false
				break
			}
		}

		if allSame {
			versions[name] = first
		} else {
			for arch, v := range archVersions {
				versions[fmt.Sprintf("%s (%s)", name, arch)] = v
			}
		}
	}
	return versions
}

// VersionsByArch returns package versions grouped by architecture.
func (l *LockFile) VersionsByArch() map[string]map[string]string {
	byArch := make(map[string]map[string]string)
	for _, pkg := range l.Packages {
		if byArch[pkg.Arch] == nil {
			byArch[pkg.Arch] = make(map[string]string)
		}
		byArch[pkg.Arch][pkg.Name] = pkg.Version
	}
	return byArch
}
