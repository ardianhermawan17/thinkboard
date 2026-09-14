package concurrency

import (
	"context"
	"log"
	"sync"
)

// Supervisor is the only sanctioned owner of a goroutine that outlives its caller's stack frame
// (03-backend-folder-architecture.md §7.1 rule 1, §7.10). Go starts fn in a tracked, panic-safe
// goroutine; Shutdown cancels every tracked goroutine's context and waits for them to drain.
type Supervisor struct {
	mu      sync.Mutex
	cancels map[uint64]context.CancelFunc
	nextID  uint64
	wg      sync.WaitGroup
}

// NewSupervisor constructs an empty Supervisor.
func NewSupervisor() *Supervisor {
	return &Supervisor{cancels: make(map[uint64]context.CancelFunc)}
}

// Go starts fn in a new goroutine derived from ctx. The goroutine is tracked by the supervisor's
// WaitGroup, so Shutdown can drain it, and its own cancel func is registered, so Shutdown can
// signal it independently of ctx's own cancellation. A panic inside fn is recovered here as a
// last-resort guard so it can never crash the process -- callers that need domain-specific panic
// handling (e.g. marking a run failed and publishing a hub event) must recover inside fn itself;
// this recover only stops propagation, it has no knowledge of what fn was doing.
func (s *Supervisor) Go(ctx context.Context, fn func(context.Context)) {
	runCtx, cancel := context.WithCancel(ctx)

	s.mu.Lock()
	id := s.nextID
	s.nextID++
	s.cancels[id] = cancel
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			s.mu.Lock()
			delete(s.cancels, id)
			s.mu.Unlock()
			cancel()
		}()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("concurrency: recovered panic in supervised goroutine: %v", r)
			}
		}()
		fn(runCtx)
	}()
}

// Shutdown signals every tracked goroutine's context and blocks until they all finish or ctx's
// own deadline passes, whichever comes first (03-backend-folder-architecture.md §7.10).
func (s *Supervisor) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	for _, cancel := range s.cancels {
		cancel()
	}
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
