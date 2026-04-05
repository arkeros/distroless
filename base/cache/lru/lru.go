// Package lru provides a generic, fixed-capacity LRU cache.
package lru

import (
	"container/list"
	"sync"
)

// Cache is a generic, thread-safe LRU cache with a fixed capacity.
type Cache[K comparable, V any] struct {
	mu    sync.Mutex
	cap   int
	items map[K]*list.Element
	order *list.List
}

type entry[K comparable, V any] struct {
	key K
	val V
}

// New creates an LRU cache that holds at most cap entries.
// It panics if cap < 1.
func New[K comparable, V any](cap int) *Cache[K, V] {
	if cap < 1 {
		panic("lru: capacity must be positive")
	}
	return &Cache[K, V]{
		cap:   cap,
		items: make(map[K]*list.Element, cap),
		order: list.New(),
	}
}

// Get retrieves a value and marks it as recently used.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		var zero V
		return zero, false
	}
	c.order.MoveToFront(el)
	return el.Value.(*entry[K, V]).val, true
}

// Put inserts or updates a key-value pair.
// If the cache is at capacity, the least recently used entry is evicted.
func (c *Cache[K, V]) Put(key K, val V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		el.Value.(*entry[K, V]).val = val
		c.order.MoveToFront(el)
		return
	}
	if c.order.Len() >= c.cap {
		back := c.order.Back()
		c.order.Remove(back)
		delete(c.items, back.Value.(*entry[K, V]).key)
	}
	el := c.order.PushFront(&entry[K, V]{key: key, val: val})
	c.items[key] = el
}

// Len returns the number of entries in the cache.
func (c *Cache[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}
