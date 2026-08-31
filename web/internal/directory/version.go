package directory

import (
	"strconv"
	"strings"
)

// CompareVersion orders two version strings the way dpkg does, returning a
// negative number, zero or a positive number as a is less than, equal to or
// greater than b.
//
// A plain string compare is wrong here: it puts 1.10 before 1.9, and it has no
// notion of `~`, which Debian uses to make a pre-release sort *before* the
// release it leads to. Most Components in an SBOM are debs, and for the rest —
// Go modules, npm packages, upstream tarballs — the algorithm degrades into a
// natural ordering that is still what a reader expects.
func CompareVersion(a, b string) int {
	aEpoch, aUpstream, aRevision := splitVersion(a)
	bEpoch, bUpstream, bRevision := splitVersion(b)

	if aEpoch != bEpoch {
		return aEpoch - bEpoch
	}
	if c := compareFragment(aUpstream, bUpstream); c != 0 {
		return c
	}
	return compareFragment(aRevision, bRevision)
}

// splitVersion breaks `[epoch:]upstream[-revision]` apart. A missing epoch is
// zero and a missing revision is empty, which is also how they compare.
func splitVersion(v string) (epoch int, upstream, revision string) {
	if i := strings.IndexByte(v, ':'); i >= 0 {
		if e, err := strconv.Atoi(v[:i]); err == nil {
			epoch, v = e, v[i+1:]
		}
	}
	// The revision is whatever follows the *last* hyphen; upstream versions are
	// allowed to contain hyphens themselves.
	if i := strings.LastIndexByte(v, '-'); i >= 0 {
		return epoch, v[:i], v[i+1:]
	}
	return epoch, v, ""
}

// compareFragment compares one upstream or revision fragment, alternating
// between runs of non-digits (compared by rank) and runs of digits (compared
// as numbers).
func compareFragment(a, b string) int {
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		for (i < len(a) && !isDigit(a[i])) || (j < len(b) && !isDigit(b[j])) {
			ac, bc := 0, 0
			if i < len(a) {
				ac = rank(a[i])
			}
			if j < len(b) {
				bc = rank(b[j])
			}
			if ac != bc {
				return ac - bc
			}
			i++
			j++
		}

		for i < len(a) && a[i] == '0' {
			i++
		}
		for j < len(b) && b[j] == '0' {
			j++
		}

		// With leading zeros gone, the longer run of digits is the larger
		// number; only if both runs are the same length does the first
		// differing digit decide.
		firstDiff := 0
		for i < len(a) && isDigit(a[i]) && j < len(b) && isDigit(b[j]) {
			if firstDiff == 0 {
				firstDiff = int(a[i]) - int(b[j])
			}
			i++
			j++
		}
		if i < len(a) && isDigit(a[i]) {
			return 1
		}
		if j < len(b) && isDigit(b[j]) {
			return -1
		}
		if firstDiff != 0 {
			return firstDiff
		}
	}
	return 0
}

// rank orders a non-digit byte. Everything is ordered by its byte value except
// that `~` sorts before the end of the string (so 1.0~rc1 < 1.0) and letters
// sort before every other punctuation character (so 1.0a < 1.0+deb12u1).
func rank(c byte) int {
	switch {
	case c == '~':
		return -1
	case isLetter(c):
		return int(c)
	default:
		return int(c) + 256
	}
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isLetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
