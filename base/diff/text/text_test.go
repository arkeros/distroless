package text

import (
	"strings"
	"testing"
)

func TestUnified_NoDiff(t *testing.T) {
	t.Parallel()
	got := Unified("a\nb\n", "a\nb\n", "want", "got", 3)
	if got != "--- want\n+++ got\n" {
		t.Fatalf("expected header only, got:\n%s", got)
	}
}

func TestUnified_SingleChange(t *testing.T) {
	t.Parallel()
	a := "line1\nline2\nline3\n"
	b := "line1\nchanged\nline3\n"
	got := Unified(a, b, "want", "got", 3)

	if !strings.Contains(got, "-line2") {
		t.Fatalf("expected -line2 in diff:\n%s", got)
	}
	if !strings.Contains(got, "+changed") {
		t.Fatalf("expected +changed in diff:\n%s", got)
	}
	if !strings.Contains(got, " line1") {
		t.Fatalf("expected context line1 in diff:\n%s", got)
	}
}

func TestUnified_CompleteReplacement(t *testing.T) {
	t.Parallel()
	got := Unified("gibberish\n", "real content\n", "want", "got", 3)

	if !strings.Contains(got, "-gibberish") {
		t.Fatalf("expected -gibberish:\n%s", got)
	}
	if !strings.Contains(got, "+real content") {
		t.Fatalf("expected +real content:\n%s", got)
	}
}

func TestUnified_ContextLines(t *testing.T) {
	t.Parallel()
	a := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"
	b := "1\n2\n3\n4\nFIVE\n6\n7\n8\n9\n10\n"
	got := Unified(a, b, "want", "got", 2)

	// Should have context of 2 lines around the change
	if !strings.Contains(got, " 3\n") {
		t.Fatalf("expected context line 3:\n%s", got)
	}
	if !strings.Contains(got, " 7\n") {
		t.Fatalf("expected context line 7:\n%s", got)
	}
	// Line 1 should NOT appear (too far from change with context=2)
	if strings.Contains(got, " 1\n") {
		t.Fatalf("line 1 should not be in context:\n%s", got)
	}
}

func TestUnified_NoNewlineAtEnd(t *testing.T) {
	t.Parallel()
	a := "line1\nline2"
	b := "line1\nchanged"
	got := Unified(a, b, "want", "got", 3)

	if !strings.Contains(got, `\ No newline at end of file`) {
		t.Fatalf("expected no-newline marker:\n%s", got)
	}
}

func TestUnified_NoNewlineOnDeletedSide(t *testing.T) {
	t.Parallel()
	a := "line1\nold"
	b := "line1\nnew\n"
	got := Unified(a, b, "want", "got", 3)

	if !strings.Contains(got, "-old") {
		t.Fatalf("expected -old:\n%s", got)
	}
	if !strings.Contains(got, `\ No newline at end of file`) {
		t.Fatalf("expected no-newline marker on deleted side:\n%s", got)
	}
}
