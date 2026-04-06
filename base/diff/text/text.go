// Package text provides string-oriented helpers on top of the generic diff algorithm.
package text

import (
	"strings"

	"github.com/arkeros/senku/base/diff"
)

// DiffLines splits strings by newline and feeds them to the generic diff algorithm.
func DiffLines(a, b string) []diff.Edit[string] {
	return diff.Compare(splitLines(a), splitLines(b))
}

// Unified returns a unified diff string comparing a and b line by line.
func Unified(a, b string, fromFile, toFile string, context int) string {
	aNewline := strings.HasSuffix(a, "\n")
	bNewline := strings.HasSuffix(b, "\n")
	edits := DiffLines(a, b)
	return formatUnified(edits, fromFile, toFile, context, aNewline, bNewline)
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}
