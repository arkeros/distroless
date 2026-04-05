package lru_test

import (
	"fmt"
	"testing"

	"github.com/arkeros/senku/base/cache/lru"
)

func TestGet_miss(t *testing.T) {
	c := lru.New[string, int](2)
	if _, ok := c.Get("x"); ok {
		t.Fatal("expected miss")
	}
}

func TestPutAndGet(t *testing.T) {
	c := lru.New[string, int](2)
	c.Put("a", 1)
	v, ok := c.Get("a")
	if !ok || v != 1 {
		t.Fatalf("got (%v, %v), want (1, true)", v, ok)
	}
}

func TestEvictsLRU(t *testing.T) {
	c := lru.New[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3) // evicts "a"

	if _, ok := c.Get("a"); ok {
		t.Fatal("expected 'a' to be evicted")
	}
	if v, ok := c.Get("b"); !ok || v != 2 {
		t.Fatal("expected 'b' = 2")
	}
	if v, ok := c.Get("c"); !ok || v != 3 {
		t.Fatal("expected 'c' = 3")
	}
}

func TestGetPromotes(t *testing.T) {
	c := lru.New[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Get("a")    // promote "a"
	c.Put("c", 3) // evicts "b", not "a"

	if _, ok := c.Get("b"); ok {
		t.Fatal("expected 'b' to be evicted")
	}
	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Fatal("expected 'a' = 1")
	}
}

func TestPutUpdatesValue(t *testing.T) {
	c := lru.New[string, int](2)
	c.Put("a", 1)
	c.Put("a", 2)
	v, ok := c.Get("a")
	if !ok || v != 2 {
		t.Fatalf("got (%v, %v), want (2, true)", v, ok)
	}
}

func TestNew_panicsOnNonPositiveCap(t *testing.T) {
	for _, cap := range []int{0, -1, -100} {
		t.Run(fmt.Sprintf("cap=%d", cap), func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatal("expected panic")
				}
			}()
			lru.New[string, int](cap)
		})
	}
}

func TestLen(t *testing.T) {
	c := lru.New[string, int](2)
	if c.Len() != 0 {
		t.Fatal("expected 0")
	}
	c.Put("a", 1)
	if c.Len() != 1 {
		t.Fatal("expected 1")
	}
	c.Put("b", 2)
	c.Put("c", 3)
	if c.Len() != 2 {
		t.Fatal("expected 2 after eviction")
	}
}
