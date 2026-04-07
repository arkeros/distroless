package internal

import (
	"os"
	"strings"

	bifrost "github.com/arkeros/senku/devtools/bifrost/api"
)

// LoadEnvironment reads and parses an Environment file from disk.
func LoadEnvironment(path string) (bifrost.Environment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return bifrost.Environment{}, err
	}
	return bifrost.ParseEnvironment(strings.NewReader(string(data)))
}
