// Package cmp provides small comparison helpers.
package cmp

// Sign normalizes an int (e.g. a Compare result) to -1, 0, or 1.
func Sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}
