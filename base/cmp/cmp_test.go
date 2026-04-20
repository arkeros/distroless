package cmp

import "testing"

func TestSign(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{-42, -1},
		{-1, -1},
		{0, 0},
		{1, 1},
		{42, 1},
	}
	for _, c := range cases {
		if got := Sign(c.in); got != c.want {
			t.Errorf("Sign(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}
