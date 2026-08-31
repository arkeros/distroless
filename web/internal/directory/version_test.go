package directory_test

import (
	"testing"

	"github.com/arkeros/distroless/web/internal/directory"
)

func TestCompareVersion(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{"equal", "1.0", "1.0", 0},
		{"minor", "1.0", "1.1", -1},
		// The reason this comparator exists: lexically, "1.10" < "1.9".
		{"natural numeric order", "1.9", "1.10", -1},
		{"leading zeros are not significant", "1.007", "1.7", 0},
		{"longer upstream wins", "1.0", "1.0.0", -1},
		{"tilde sorts before end of string", "1.0~beta", "1.0", -1},
		{"tilde sorts before tilde-less suffix", "1.0~rc1", "1.0a", -1},
		{"double tilde sorts first", "1.0~~", "1.0~", -1},
		{"letter sorts after end of string", "1.0", "1.0a", -1},
		{"non-alnum sorts after letters", "1.0a", "1.0+deb12u1", -1},
		{"revision breaks upstream tie", "1.0-1", "1.0-2", -1},
		{"absent revision sorts first", "1.0", "1.0-1", -1},
		{"epoch dominates upstream", "2.0", "1:1.0", -1},
		{"equal epochs fall through", "1:2.0", "1:1.0", 1},
		{"absent epoch is zero", "0:1.0", "1.0", 0},
		// Not every Component is a deb; the algorithm has to degrade sanely.
		{"semver", "v1.2.9", "v1.2.10", -1},
		{"empty is smallest", "", "1", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sign(directory.CompareVersion(tt.a, tt.b)); got != tt.want {
				t.Errorf("CompareVersion(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
			// A comparator that is not antisymmetric produces an unstable sort.
			if got := sign(directory.CompareVersion(tt.b, tt.a)); got != -tt.want {
				t.Errorf("CompareVersion(%q, %q) = %d, want %d", tt.b, tt.a, got, -tt.want)
			}
		})
	}
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}
