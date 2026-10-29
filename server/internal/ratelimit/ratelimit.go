// Package ratelimit counts requests per key in a sliding time window.
package ratelimit

import (
	"sync"
	"time"
)

// sweepEvery is how many calls pass between removals of keys that have gone quiet, which keeps
// memory bounded when many different keys are seen once.
const sweepEvery = 1000

// Limiter allows at most a fixed number of hits per key within a sliding window. It is safe for
// concurrent use. Its counts live in memory, so they restart with the server.
type Limiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	now    func() time.Time
	hits   map[string][]time.Time
	calls  int
}

// New returns a Limiter that allows limit hits per key within window. now supplies the current
// time; pass time.Now outside tests.
func New(limit int, window time.Duration, now func() time.Time) *Limiter {
	return &Limiter{limit: limit, window: window, now: now, hits: make(map[string][]time.Time)}
}

// Allow records a hit for key when it is within the limit and reports whether it was. When it
// is not, it also returns how long until the next hit would be allowed. Refused hits are not
// counted.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.calls++
	if l.calls%sweepEvery == 0 {
		l.sweep(now)
	}

	recent := l.recent(key, now)
	if len(recent) >= l.limit {
		l.hits[key] = recent
		return false, recent[0].Add(l.window).Sub(now)
	}

	l.hits[key] = append(recent, now)
	return true, 0
}

// recent returns key's hits that are still inside the window, oldest first.
func (l *Limiter) recent(key string, now time.Time) []time.Time {
	hits := l.hits[key]
	cutoff := now.Add(-l.window)

	start := 0
	for start < len(hits) && !hits[start].After(cutoff) {
		start++
	}
	return hits[start:]
}

func (l *Limiter) sweep(now time.Time) {
	for key := range l.hits {
		if recent := l.recent(key, now); len(recent) == 0 {
			delete(l.hits, key)
		} else {
			l.hits[key] = recent
		}
	}
}

// Check reports whether a hit for key would be allowed now, without recording one, and when it
// would not, how long until it would. Pair it with Record to count only some outcomes, such as
// failures. Check and Record are not one step, so concurrent callers can go slightly over the limit.
func (l *Limiter) Check(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	recent := l.recent(key, now)
	l.hits[key] = recent
	if len(recent) >= l.limit {
		return false, recent[0].Add(l.window).Sub(now)
	}
	return true, 0
}

// Record counts a hit for key, whether or not it is within the limit.
func (l *Limiter) Record(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.calls++
	if l.calls%sweepEvery == 0 {
		l.sweep(now)
	}
	l.hits[key] = append(l.recent(key, now), now)
}
