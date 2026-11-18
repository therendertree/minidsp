package bidding

import "sync"

// Counter counts no-bids per reason. In-memory only for Milestone 1; durable
// bid records arrive with PostgreSQL in Milestone 2. Safe for concurrent use.
type Counter struct {
	mu sync.Mutex
	m  map[Reason]int64
}

func NewCounter() *Counter { return &Counter{m: make(map[Reason]int64)} }

func (c *Counter) Inc(r Reason) {
	c.mu.Lock()
	c.m[r]++
	c.mu.Unlock()
}

// Snapshot returns a copy of the current counts.
func (c *Counter) Snapshot() map[Reason]int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[Reason]int64, len(c.m))
	for k, v := range c.m {
		out[k] = v
	}
	return out
}
