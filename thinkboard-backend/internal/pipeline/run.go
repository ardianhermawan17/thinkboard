package pipeline

import (
	"errors"
	"time"

	"thinkboard-backend/internal/shared/kernel"
)

// RunStatus mirrors the Postgres run_status enum (thinkboard-schema-final.sql) 1:1.
type RunStatus string

const (
	StatusPending   RunStatus = "pending"
	StatusRunning   RunStatus = "running"
	StatusDone      RunStatus = "done"
	StatusFailed    RunStatus = "failed"
	StatusCancelled RunStatus = "cancelled"
)

// Mode mirrors the Postgres session_mode enum (thinkboard-schema-final.sql) 1:1.
type Mode string

const (
	ModePlanning    Mode = "planning"
	ModeDescriptive Mode = "descriptive"
	ModeVisualize   Mode = "visualize"
)

// ErrInvalidTransition is returned by Run's invariant methods when called from a status
// that does not permit the requested transition.
var ErrInvalidTransition = errors.New("pipeline: invalid run transition")

// Run is pipeline_runs in-memory state plus the invariants that guard it. No I/O: callers
// supply "now" rather than Run calling time.Now() itself (03-backend-folder-architecture.md §4).
type Run struct {
	ID        kernel.RunID
	SessionID kernel.SessionID
	Mode      Mode
	Status    RunStatus
	// CurrentStage is the stage cursor. Zero value ("") means the run has not been started.
	CurrentStage Stage
	StartedAt    time.Time
	FinishedAt   time.Time
}

// NewRun constructs a Run in its initial pending state.
func NewRun(id kernel.RunID, sessionID kernel.SessionID, mode Mode) *Run {
	return &Run{ID: id, SessionID: sessionID, Mode: mode, Status: StatusPending}
}

// Start transitions a pending run to running and positions the cursor at the first stage.
func (r *Run) Start(now time.Time) error {
	if r.Status != StatusPending {
		return ErrInvalidTransition
	}
	r.Status = StatusRunning
	r.CurrentStage = Order[0]
	r.StartedAt = now
	return nil
}

// Advance moves the stage cursor to the next stage in Order. Advancing past the last stage
// completes the run (Status -> done) instead of erroring, since there is no stage beyond it.
func (r *Run) Advance(now time.Time) error {
	if r.Status != StatusRunning {
		return ErrInvalidTransition
	}
	i := indexOf(r.CurrentStage)
	if i < 0 {
		return ErrInvalidTransition
	}
	if i == len(Order)-1 {
		r.Status = StatusDone
		r.FinishedAt = now
		return nil
	}
	r.CurrentStage = Order[i+1]
	return nil
}

// Fail moves a non-terminal run to failed.
func (r *Run) Fail(now time.Time) error {
	if r.Status != StatusPending && r.Status != StatusRunning {
		return ErrInvalidTransition
	}
	r.Status = StatusFailed
	r.FinishedAt = now
	return nil
}

// Cancel moves a non-terminal run to cancelled.
func (r *Run) Cancel(now time.Time) error {
	if r.Status != StatusPending && r.Status != StatusRunning {
		return ErrInvalidTransition
	}
	r.Status = StatusCancelled
	r.FinishedAt = now
	return nil
}
