package service

import (
	"container/list"
	"sync"
	"time"
)

type cacheEntry struct {
	key       string
	value     string
	expiresAt time.Time
}

type lruCache struct {
	mu      sync.Mutex
	maxSize int
	order   *list.List
	items   map[string]*list.Element
}

func newLRUCache(maxSize int) *lruCache {
	if maxSize < 1 {
		maxSize = 1
	}
	return &lruCache{
		maxSize: maxSize,
		order:   list.New(),
		items:   map[string]*list.Element{},
	}
}

func (c *lruCache) Get(key string, now time.Time) (string, time.Duration, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	element, ok := c.items[key]
	if !ok {
		return "", 0, false
	}

	entry := element.Value.(*cacheEntry)
	if now.After(entry.expiresAt) {
		c.order.Remove(element)
		delete(c.items, key)
		return "", 0, false
	}

	c.order.MoveToFront(element)
	return entry.value, entry.expiresAt.Sub(now), true
}

func (c *lruCache) Set(key string, value string, ttl time.Duration, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if element, ok := c.items[key]; ok {
		entry := element.Value.(*cacheEntry)
		entry.value = value
		entry.expiresAt = now.Add(ttl)
		c.order.MoveToFront(element)
		return
	}

	element := c.order.PushFront(&cacheEntry{
		key:       key,
		value:     value,
		expiresAt: now.Add(ttl),
	})
	c.items[key] = element

	if c.order.Len() <= c.maxSize {
		return
	}

	tail := c.order.Back()
	if tail == nil {
		return
	}
	tailEntry := tail.Value.(*cacheEntry)
	delete(c.items, tailEntry.key)
	c.order.Remove(tail)
}

type toolCacheRegistry struct {
	mu      sync.Mutex
	maxSize int
	items   map[string]*lruCache
}

func newToolCacheRegistry(maxSize int) *toolCacheRegistry {
	return &toolCacheRegistry{
		maxSize: maxSize,
		items:   map[string]*lruCache{},
	}
}

func (r *toolCacheRegistry) cacheFor(toolID string, maxSize int) *lruCache {
	r.mu.Lock()
	defer r.mu.Unlock()

	cache, ok := r.items[toolID]
	if ok {
		cache.maxSize = maxSize
		return cache
	}
	size := r.maxSize
	if maxSize > 0 {
		size = maxSize
	}
	cache = newLRUCache(size)
	r.items[toolID] = cache
	return cache
}
