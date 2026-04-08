// Package stringslice provides a flag.Value for repeated string flags.
package stringslice

import "fmt"

// Value implements flag.Value for repeated string flags.
// Usage: flag.Var(&v, "name", "usage")
type Value []string

func (v *Value) String() string { return fmt.Sprintf("%v", *v) }

func (v *Value) Set(s string) error {
	*v = append(*v, s)
	return nil
}
