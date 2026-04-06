package text

import (
	"fmt"
	"strings"

	"github.com/arkeros/senku/base/diff"
)

const noNewlineMessage = `\ No newline at end of file`

func formatUnified(
	edits []diff.Edit[string],
	fromFile, toFile string,
	context int,
	aNewline, bNewline bool,
) string {
	var buf strings.Builder
	fmt.Fprintf(&buf, "--- %s\n+++ %s\n", fromFile, toFile)

	// Assign 1-based line numbers to each edit.
	type indexedEdit struct {
		diff.Edit[string]
		idxA, idxB int
	}
	indexed := make([]indexedEdit, len(edits))
	a, b := 0, 0
	for i, e := range edits {
		switch e.Op {
		case diff.Keep:
			a++
			b++
			indexed[i] = indexedEdit{e, a, b}
		case diff.Delete:
			a++
			indexed[i] = indexedEdit{e, a, b}
		case diff.Insert:
			b++
			indexed[i] = indexedEdit{e, a, b}
		}
	}

	totalA, totalB := a, b

	// Find change regions and emit hunks with context.
	i := 0
	for i < len(indexed) {
		if indexed[i].Op == diff.Keep {
			i++
			continue
		}

		// Found a change. Expand to include context.
		hunkStart := i - context
		if hunkStart < 0 {
			hunkStart = 0
		}

		// Find end of this hunk (may merge with nearby changes).
		hunkEnd := i
		for hunkEnd < len(indexed) {
			if indexed[hunkEnd].Op != diff.Keep {
				hunkEnd++
				continue
			}
			keepRun := 0
			for hunkEnd+keepRun < len(indexed) && indexed[hunkEnd+keepRun].Op == diff.Keep {
				keepRun++
			}
			if keepRun > 2*context && hunkEnd+keepRun < len(indexed) {
				hunkEnd += context
				break
			}
			hunkEnd += keepRun
		}
		if hunkEnd > len(indexed) {
			hunkEnd = len(indexed)
		}

		// Compute line ranges for the hunk header.
		var fromStart, fromCount, toStart, toCount int
		for _, e := range indexed[hunkStart:hunkEnd] {
			switch e.Op {
			case diff.Keep:
				if fromCount == 0 {
					fromStart = e.idxA
				}
				if toCount == 0 {
					toStart = e.idxB
				}
				fromCount++
				toCount++
			case diff.Delete:
				if fromCount == 0 {
					fromStart = e.idxA
				}
				if toCount == 0 {
					toStart = e.idxB + 1
				}
				fromCount++
			case diff.Insert:
				if fromCount == 0 {
					fromStart = e.idxA + 1
				}
				if toCount == 0 {
					toStart = e.idxB
				}
				toCount++
			}
		}

		fmt.Fprintf(&buf, "@@ -%d,%d +%d,%d @@\n", fromStart, fromCount, toStart, toCount)
		for _, e := range indexed[hunkStart:hunkEnd] {
			switch e.Op {
			case diff.Keep:
				fmt.Fprintf(&buf, " %s\n", e.Item)
				if e.idxA == totalA && !aNewline && e.idxB == totalB && !bNewline {
					buf.WriteString(noNewlineMessage)
					buf.WriteByte('\n')
				}
			case diff.Delete:
				fmt.Fprintf(&buf, "-%s\n", e.Item)
				if e.idxA == totalA && !aNewline {
					buf.WriteString(noNewlineMessage)
					buf.WriteByte('\n')
				}
			case diff.Insert:
				fmt.Fprintf(&buf, "+%s\n", e.Item)
				if e.idxB == totalB && !bNewline {
					buf.WriteString(noNewlineMessage)
					buf.WriteByte('\n')
				}
			}
		}

		i = hunkEnd
	}

	return buf.String()
}
