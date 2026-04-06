package diff

import (
	"testing"
)

func TestCompare_Empty(t *testing.T) {
	t.Parallel()
	edits := Compare([]string{}, []string{})
	if len(edits) != 0 {
		t.Fatalf("expected no edits, got %d", len(edits))
	}
}

func TestCompare_Equal(t *testing.T) {
	t.Parallel()
	edits := Compare([]string{"a", "b", "c"}, []string{"a", "b", "c"})
	for _, e := range edits {
		if e.Op != Keep {
			t.Fatalf("expected all Keep, got %v", edits)
		}
	}
}

func TestCompare_Insert(t *testing.T) {
	t.Parallel()
	edits := Compare([]string{"a", "c"}, []string{"a", "b", "c"})
	want := []Edit[string]{
		{Keep, "a"},
		{Insert, "b"},
		{Keep, "c"},
	}
	assertEdits(t, edits, want)
}

func TestCompare_Delete(t *testing.T) {
	t.Parallel()
	edits := Compare([]string{"a", "b", "c"}, []string{"a", "c"})
	want := []Edit[string]{
		{Keep, "a"},
		{Delete, "b"},
		{Keep, "c"},
	}
	assertEdits(t, edits, want)
}

func TestCompare_Replace(t *testing.T) {
	t.Parallel()
	edits := Compare([]string{"a", "b"}, []string{"a", "c"})
	want := []Edit[string]{
		{Keep, "a"},
		{Delete, "b"},
		{Insert, "c"},
	}
	assertEdits(t, edits, want)
}

func TestCompare_CompletelyDifferent(t *testing.T) {
	t.Parallel()
	edits := Compare([]string{"a", "b"}, []string{"c", "d"})
	deletes, inserts := 0, 0
	for _, e := range edits {
		switch e.Op {
		case Delete:
			deletes++
		case Insert:
			inserts++
		}
	}
	if deletes != 2 || inserts != 2 {
		t.Fatalf("expected 2 deletes and 2 inserts, got %v", edits)
	}
}

func TestCompare_Integers(t *testing.T) {
	t.Parallel()
	edits := Compare([]int{1, 2, 3}, []int{1, 3})
	want := []Edit[int]{
		{Keep, 1},
		{Delete, 2},
		{Keep, 3},
	}
	if len(edits) != len(want) {
		t.Fatalf("expected %d edits, got %d: %v", len(want), len(edits), edits)
	}
	for i := range edits {
		if edits[i] != want[i] {
			t.Fatalf("edit[%d] = %v, want %v", i, edits[i], want[i])
		}
	}
}

func TestCompare_InsertAtStart(t *testing.T) {
	t.Parallel()
	edits := Compare([]string{"b"}, []string{"a", "b"})
	want := []Edit[string]{
		{Insert, "a"},
		{Keep, "b"},
	}
	assertEdits(t, edits, want)
}

func TestCompare_DeleteAll(t *testing.T) {
	t.Parallel()
	edits := Compare([]string{"a", "b"}, []string{})
	want := []Edit[string]{
		{Delete, "a"},
		{Delete, "b"},
	}
	assertEdits(t, edits, want)
}

func TestCompare_InsertAll(t *testing.T) {
	t.Parallel()
	edits := Compare([]string{}, []string{"a", "b"})
	want := []Edit[string]{
		{Insert, "a"},
		{Insert, "b"},
	}
	assertEdits(t, edits, want)
}

func assertEdits[T comparable](t *testing.T, got, want []Edit[T]) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %d edits, got %d: %v", len(want), len(got), got)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("edit[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}
