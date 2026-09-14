package concurrency

import "sync"

// Semaphore is a simple counting semaphore bounding concurrent work (03-backend-folder-architecture.md
// §7.1 rule 4, §7.7 admission control). Only non-blocking TryAcquire is needed today -- nothing in
// this codebase waits for a slot to free; over the limit, callers admit the work as pending instead
// (§7.7 "prefer enqueueing to rejecting").
type Semaphore struct {
	mu  sync.Mutex
	cur int64
	max int64
}

// NewSemaphore constructs a Semaphore that allows up to max concurrent holders.
func NewSemaphore(max int64) *Semaphore {
	return &Semaphore{max: max}
}

// TryAcquire reports whether n slots were available and, if so, reserves them.
func (s *Semaphore) TryAcquire(n int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur+n > s.max {
		return false
	}
	s.cur += n
	return true
}

// Release returns n previously-acquired slots. Releasing more than was ever acquired for a run is
// a caller bug; Release clamps at zero rather than going negative so a duplicate Release can't
// hand out slots that were never real.
func (s *Semaphore) Release(n int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cur -= n
	if s.cur < 0 {
		s.cur = 0
	}
}
