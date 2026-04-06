// Package diff implements the Myers diff algorithm for comparable sequences.
package diff

// Op represents a diff operation.
type Op int

const (
	Keep   Op = iota // Item is present in both sequences.
	Insert           // Item was added in sequence B.
	Delete           // Item was removed from sequence A.
)

// Edit represents a single diff operation on an item.
type Edit[T comparable] struct {
	Op   Op
	Item T
}

// Compare returns the shortest edit script to transform a into b
// using the Myers diff algorithm (An O(ND) Difference Algorithm, Myers 1986).
func Compare[T comparable](a, b []T) []Edit[T] {
	n, m := len(a), len(b)
	if n == 0 && m == 0 {
		return nil
	}

	max := n + m
	v := make([]int, 2*max+1)
	trace := make([][]int, 0, max+1)

	for d := 0; d <= max; d++ {
		snapshot := make([]int, len(v))
		copy(snapshot, v)
		trace = append(trace, snapshot)

		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[k-1+max] < v[k+1+max]) {
				x = v[k+1+max]
			} else {
				x = v[k-1+max] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[k+max] = x
			if x >= n && y >= m {
				return backtrack(trace, a, b, d, max)
			}
		}
	}
	return nil
}

func backtrack[T comparable](trace [][]int, a, b []T, d, max int) []Edit[T] {
	x, y := len(a), len(b)
	var edits []Edit[T]

	for d := d; d > 0; d-- {
		v := trace[d]
		k := x - y

		var prevK int
		if k == -d || (k != d && v[k-1+max] < v[k+1+max]) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}
		prevX := v[prevK+max]
		prevY := prevX - prevK

		for x > prevX && y > prevY {
			x--
			y--
			edits = append(edits, Edit[T]{Op: Keep, Item: a[x]})
		}

		if prevK == k+1 {
			y--
			edits = append(edits, Edit[T]{Op: Insert, Item: b[y]})
		} else {
			x--
			edits = append(edits, Edit[T]{Op: Delete, Item: a[x]})
		}
	}

	for x > 0 && y > 0 {
		x--
		y--
		edits = append(edits, Edit[T]{Op: Keep, Item: a[x]})
	}

	// Reverse to get forward order.
	for i, j := 0, len(edits)-1; i < j; i, j = i+1, j-1 {
		edits[i], edits[j] = edits[j], edits[i]
	}
	return edits
}
