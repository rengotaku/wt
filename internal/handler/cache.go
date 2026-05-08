package handler

import (
	"sync"
	"time"
)

type cacheEntry struct {
	val     any
	expires time.Time
}

type ttlCache struct {
	mu    sync.Mutex
	items map[string]cacheEntry
}

func newTTLCache() *ttlCache {
	return &ttlCache{items: make(map[string]cacheEntry)}
}

func (c *ttlCache) get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[key]
	if !ok || time.Now().After(e.expires) {
		delete(c.items, key)
		return nil, false
	}
	return e.val, true
}

func (c *ttlCache) set(key string, val any, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = cacheEntry{val: val, expires: time.Now().Add(ttl)}
}

func (c *ttlCache) del(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}
