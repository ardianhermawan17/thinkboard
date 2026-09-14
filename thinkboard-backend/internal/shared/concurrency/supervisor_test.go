package concurrency

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestSupervisorGo_RunsFn(t *testing.T) {
	s := NewSupervisor()
	done := make(chan struct{})
	s.Go(context.Background(), func(ctx context.Context) {
		close(done)
	})
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Go's fn to run")
	}
}

// TestSupervisorGo_RecoversPanic proves a panic inside fn cannot crash the process: if Supervisor's
// recover didn't work, this test binary would abort outright rather than reach any assertion.
func TestSupervisorGo_RecoversPanic(t *testing.T) {
	s := NewSupervisor()
	s.Go(context.Background(), func(ctx context.Context) {
		panic("boom")
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v, want nil (a panicking fn must still let the supervisor drain)", err)
	}
}

func TestSupervisorShutdown_CancelsTrackedContexts(t *testing.T) {
	s := NewSupervisor()
	const n = 5
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		s.Go(context.Background(), func(ctx context.Context) {
			<-ctx.Done()
			wg.Done()
		})
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() error = %v, want nil", err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for all tracked goroutines to observe ctx.Done()")
	}
}

func TestSupervisorShutdown_DeadlineExceeded(t *testing.T) {
	s := NewSupervisor()
	blockForever := make(chan struct{})
	defer close(blockForever) // let the leaked goroutine exit at test teardown

	s.Go(context.Background(), func(ctx context.Context) {
		<-blockForever // deliberately ignores ctx.Done(), simulating a stuck fn
	})

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := s.Shutdown(shutdownCtx); err == nil {
		t.Fatal("Shutdown() error = nil, want a deadline error (fn never returns)")
	}
}

func TestSupervisor_ConcurrentGoAndShutdown(t *testing.T) {
	s := NewSupervisor()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Go(context.Background(), func(ctx context.Context) {
				<-ctx.Done()
			})
		}()
	}
	wg.Wait()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v, want nil", err)
	}
}
