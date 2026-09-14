package concurrency

import "testing"

func TestSemaphore_TryAcquireRelease(t *testing.T) {
	s := NewSemaphore(2)

	if !s.TryAcquire(1) {
		t.Fatal("TryAcquire(1) = false, want true (slot available)")
	}
	if !s.TryAcquire(1) {
		t.Fatal("TryAcquire(1) = false, want true (second of two slots)")
	}
	if s.TryAcquire(1) {
		t.Fatal("TryAcquire(1) = true, want false (over capacity)")
	}

	s.Release(1)
	if !s.TryAcquire(1) {
		t.Fatal("TryAcquire(1) = false after Release, want true (slot freed)")
	}
}

func TestSemaphore_ReleaseClampsAtZero(t *testing.T) {
	s := NewSemaphore(1)
	s.Release(5) // no matching Acquire -- must not go negative

	if !s.TryAcquire(1) {
		t.Fatal("TryAcquire(1) = false, want true")
	}
	if s.TryAcquire(1) {
		t.Fatal("TryAcquire(1) = true, want false (still capped at max despite the bogus Release)")
	}
}
