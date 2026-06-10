package sqlinspect

import (
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/flanksource/arch-unit/models/uir"
	"golang.org/x/sync/singleflight"
)

const CacheTTL = 24 * time.Hour

type Extractor func() (uir.UIR, error)

type Cache struct {
	ttl     time.Duration
	now     func() time.Time
	mu      sync.Mutex
	entries map[string]cacheEntry
	group   singleflight.Group
}

type cacheEntry struct {
	uir uir.UIR
	at  time.Time
}

func NewCache(ttl time.Duration) *Cache {
	if ttl <= 0 {
		ttl = CacheTTL
	}
	return &Cache{
		ttl:     ttl,
		now:     time.Now,
		entries: map[string]cacheEntry{},
	}
}

var DefaultCache = NewCache(CacheTTL)

func CacheKey(configItemID, database string) string {
	return url.QueryEscape(configItemID) + "/" + url.QueryEscape(database)
}

func (c *Cache) Load(key string, refresh bool, extract Extractor) (uir.UIR, error) {
	if c == nil {
		c = DefaultCache
	}
	if key == "" {
		return uir.UIR{}, fmt.Errorf("cache key is required")
	}
	if extract == nil {
		return uir.UIR{}, fmt.Errorf("extractor is required")
	}
	if refresh {
		c.Invalidate(key)
	} else if u, ok := c.get(key); ok {
		return u, nil
	}

	v, err, _ := c.group.Do(key, func() (any, error) {
		if !refresh {
			if u, ok := c.get(key); ok {
				return u, nil
			}
		}
		u, err := extract()
		if err != nil {
			return uir.UIR{}, err
		}
		c.set(key, u, c.now())
		return u, nil
	})
	if err != nil {
		return uir.UIR{}, err
	}
	return v.(uir.UIR), nil
}

func (c *Cache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

func (c *Cache) get(key string) (uir.UIR, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || c.now().Sub(e.at) > c.ttl {
		return uir.UIR{}, false
	}
	return e.uir, true
}

func (c *Cache) set(key string, u uir.UIR, at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]cacheEntry{}
	}
	c.entries[key] = cacheEntry{uir: u, at: at}
}
