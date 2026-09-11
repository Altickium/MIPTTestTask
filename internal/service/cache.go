package service

import (
	"context"
	"sync"
	"time"

	"pollservice/internal/domain"
)

type cacheEntry struct {
	mu      sync.Mutex
	poll    domain.Poll
	expires time.Time
	loaded  bool
}
type PollCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]*cacheEntry
}

func NewPollCache(ttl time.Duration) *PollCache {
	return &PollCache{ttl: ttl, entries: make(map[string]*cacheEntry)}
}

func (c *PollCache) Get(ctx context.Context, id string, loader func(context.Context, string) (domain.Poll, error)) (domain.Poll, error) {
	c.mu.Lock()
	e := c.entries[id]
	if e == nil {
		e = &cacheEntry{}
		c.entries[id] = e
	}
	c.mu.Unlock()
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.loaded && time.Now().Before(e.expires) {
		return e.poll, nil
	}
	p, err := loader(ctx, id)
	if err != nil {
		return domain.Poll{}, err
	}
	e.poll = p
	e.expires = time.Now().Add(c.ttl)
	e.loaded = true
	return p, nil
}
func (c *PollCache) Put(p domain.Poll) {
	c.mu.Lock()
	e := c.entries[p.ID]
	if e == nil {
		e = &cacheEntry{}
		c.entries[p.ID] = e
	}
	c.mu.Unlock()
	e.mu.Lock()
	e.poll = p
	e.expires = time.Now().Add(c.ttl)
	e.loaded = true
	e.mu.Unlock()
}
func (c *PollCache) Delete(id string) { c.mu.Lock(); delete(c.entries, id); c.mu.Unlock() }
