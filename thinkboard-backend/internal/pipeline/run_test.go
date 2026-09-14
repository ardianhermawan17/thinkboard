package pipeline

import (
	"testing"
	"time"

	"thinkboard-backend/internal/shared/kernel"
)

var t0 = time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)

func newTestRun(status RunStatus) *Run {
	r := NewRun(kernel.RunID("run-1"), kernel.SessionID("session-1"), ModePlanning)
	r.Status = status
	return r
}

func TestRunStart(t *testing.T) {
	r := newTestRun(StatusPending)
	if err := r.Start(t0); err != nil {
		t.Fatalf("Start() from pending: unexpected error %v", err)
	}
	if r.Status != StatusRunning {
		t.Errorf("Status = %q, want %q", r.Status, StatusRunning)
	}
	if r.CurrentStage != Order[0] {
		t.Errorf("CurrentStage = %q, want %q", r.CurrentStage, Order[0])
	}
	if !r.StartedAt.Equal(t0) {
		t.Errorf("StartedAt = %v, want %v", r.StartedAt, t0)
	}

	for _, s := range []RunStatus{StatusRunning, StatusDone, StatusFailed, StatusCancelled} {
		t.Run("from "+string(s), func(t *testing.T) {
			r := newTestRun(s)
			if err := r.Start(t0); err != ErrInvalidTransition {
				t.Errorf("Start() from %q = %v, want ErrInvalidTransition", s, err)
			}
		})
	}
}

func TestRunAdvance(t *testing.T) {
	r := newTestRun(StatusRunning)
	r.CurrentStage = Order[0]

	for i := 1; i < len(Order); i++ {
		if err := r.Advance(t0); err != nil {
			t.Fatalf("Advance() step %d: unexpected error %v", i, err)
		}
		if r.CurrentStage != Order[i] {
			t.Fatalf("Advance() step %d: CurrentStage = %q, want %q", i, r.CurrentStage, Order[i])
		}
		if r.Status != StatusRunning {
			t.Fatalf("Advance() step %d: Status = %q, want %q", i, r.Status, StatusRunning)
		}
	}

	t1 := t0.Add(time.Hour)
	if err := r.Advance(t1); err != nil {
		t.Fatalf("Advance() past last stage: unexpected error %v", err)
	}
	if r.Status != StatusDone {
		t.Errorf("Status after final Advance() = %q, want %q", r.Status, StatusDone)
	}
	if !r.FinishedAt.Equal(t1) {
		t.Errorf("FinishedAt = %v, want %v", r.FinishedAt, t1)
	}

	for _, s := range []RunStatus{StatusPending, StatusDone, StatusFailed, StatusCancelled} {
		t.Run("not running: "+string(s), func(t *testing.T) {
			r := newTestRun(s)
			r.CurrentStage = Order[0]
			if err := r.Advance(t0); err != ErrInvalidTransition {
				t.Errorf("Advance() from %q = %v, want ErrInvalidTransition", s, err)
			}
		})
	}
}

func TestRunFail(t *testing.T) {
	for _, s := range []RunStatus{StatusPending, StatusRunning} {
		t.Run("from "+string(s), func(t *testing.T) {
			r := newTestRun(s)
			if err := r.Fail(t0); err != nil {
				t.Fatalf("Fail() from %q: unexpected error %v", s, err)
			}
			if r.Status != StatusFailed {
				t.Errorf("Status = %q, want %q", r.Status, StatusFailed)
			}
			if !r.FinishedAt.Equal(t0) {
				t.Errorf("FinishedAt = %v, want %v", r.FinishedAt, t0)
			}
		})
	}

	for _, s := range []RunStatus{StatusDone, StatusFailed, StatusCancelled} {
		t.Run("from terminal "+string(s), func(t *testing.T) {
			r := newTestRun(s)
			if err := r.Fail(t0); err != ErrInvalidTransition {
				t.Errorf("Fail() from %q = %v, want ErrInvalidTransition", s, err)
			}
		})
	}
}

func TestRunCancel(t *testing.T) {
	for _, s := range []RunStatus{StatusPending, StatusRunning} {
		t.Run("from "+string(s), func(t *testing.T) {
			r := newTestRun(s)
			if err := r.Cancel(t0); err != nil {
				t.Fatalf("Cancel() from %q: unexpected error %v", s, err)
			}
			if r.Status != StatusCancelled {
				t.Errorf("Status = %q, want %q", r.Status, StatusCancelled)
			}
			if !r.FinishedAt.Equal(t0) {
				t.Errorf("FinishedAt = %v, want %v", r.FinishedAt, t0)
			}
		})
	}

	for _, s := range []RunStatus{StatusDone, StatusFailed, StatusCancelled} {
		t.Run("from terminal "+string(s), func(t *testing.T) {
			r := newTestRun(s)
			if err := r.Cancel(t0); err != ErrInvalidTransition {
				t.Errorf("Cancel() from %q = %v, want ErrInvalidTransition", s, err)
			}
		})
	}
}
