package featureflag

import (
	"context"
	"sync"
	"time"
)

// DefaultReloadInterval bounds how long a stale flag value can be served
// from the in-memory cache before the next background refresh picks up a
// change made through the admin API.
const DefaultReloadInterval = 30 * time.Second

// Cache is a read-through, periodically-refreshed view of feature flags,
// meant to be shared by middleware and services that need to check a flag
// on every request without hitting the database each time. Flags toggled
// through Service.Set take effect for this process either immediately (via
// Refresh) or within one reload interval.
type Cache struct {
	service  Service
	interval time.Duration

	mu    sync.RWMutex
	flags map[string]bool

	stopOnce sync.Once
	stop     chan struct{}
}

// NewCache builds a Cache backed by svc. It starts empty — call Start (or
// Refresh) to populate it before relying on IsEnabled.
func NewCache(svc Service, interval time.Duration) *Cache {
	if interval <= 0 {
		interval = DefaultReloadInterval
	}
	return &Cache{
		service:  svc,
		interval: interval,
		flags:    make(map[string]bool),
		stop:     make(chan struct{}),
	}
}

// IsEnabled reports whether flag is enabled according to the last
// successful reload. An unknown flag is treated as disabled.
func (c *Cache) IsEnabled(flag string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.flags[flag]
}

// Refresh reloads all flags from the underlying service immediately. It
// leaves the previous snapshot in place if the reload fails, so a transient
// DB error never wipes out an otherwise-healthy cache.
func (c *Cache) Refresh(ctx context.Context) error {
	all, err := c.service.List(ctx)
	if err != nil {
		return err
	}
	next := make(map[string]bool, len(all))
	for _, f := range all {
		next[f.Flag] = f.Enabled
	}
	c.mu.Lock()
	c.flags = next
	c.mu.Unlock()
	return nil
}

// Start performs an initial synchronous Refresh and then reloads on a
// background ticker every interval until ctx is done or Stop is called.
func (c *Cache) Start(ctx context.Context) error {
	if err := c.Refresh(ctx); err != nil {
		return err
	}
	go func() {
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = c.Refresh(ctx)
			case <-c.stop:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	return nil
}

// Stop halts the background reload goroutine started by Start. Safe to call
// more than once.
func (c *Cache) Stop() {
	c.stopOnce.Do(func() { close(c.stop) })
}
