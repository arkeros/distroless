package stringslice_test

import (
	"flag"
	"testing"

	"github.com/arkeros/senku/base/flag/stringslice"
)

func TestSet(t *testing.T) {
	var ss stringslice.Value
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.Var(&ss, "item", "repeated item")

	err := fs.Parse([]string{"--item=a", "--item=b", "--item=c"})
	if err != nil {
		t.Fatal(err)
	}

	if len(ss) != 3 {
		t.Fatalf("len = %d, want 3", len(ss))
	}
	if ss[0] != "a" || ss[1] != "b" || ss[2] != "c" {
		t.Errorf("values = %v, want [a b c]", []string(ss))
	}
}

func TestString(t *testing.T) {
	ss := stringslice.Value{"x", "y"}
	if got := ss.String(); got != "[x y]" {
		t.Errorf("String() = %q, want %q", got, "[x y]")
	}
}

func TestEmpty(t *testing.T) {
	var ss stringslice.Value
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.Var(&ss, "item", "repeated item")

	err := fs.Parse([]string{})
	if err != nil {
		t.Fatal(err)
	}

	if len(ss) != 0 {
		t.Fatalf("len = %d, want 0", len(ss))
	}
}
