package cf

import (
	"sync"
	"time"
)

type cacheEntry struct {
	guid   string
	expiry time.Time
}

type guidCache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry
	ttl     time.Duration
}

func newGUIDCache(ttl time.Duration) *guidCache {
	return &guidCache{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
	}
}

func (c *guidCache) get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || time.Now().After(e.expiry) {
		return "", false
	}
	return e.guid, true
}

func (c *guidCache) set(key, guid string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = cacheEntry{guid: guid, expiry: time.Now().Add(c.ttl)}
}

func orgCacheKey(name string) string {
	return "org:" + name
}

func spaceCacheKey(orgGUID, name string) string {
	return "space:" + orgGUID + ":" + name
}
