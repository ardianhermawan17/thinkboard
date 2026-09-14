package pipeline

import "thinkboard-backend/internal/shared/kernel"

// RunEventType names a lifecycle event published to a run's hub topic.
type RunEventType string

const (
	EventRunStarted     RunEventType = "run.started"
	EventStageCompleted RunEventType = "stage.completed"
	EventRunFailed      RunEventType = "run.failed"
	EventRunCancelled   RunEventType = "run.cancelled"
	// EventRunCompleted is defined for the full lifecycle vocabulary but is not published by this
	// build-order step -- it requires all 7 stages wired (§9 step 8), not just analytic.
	EventRunCompleted RunEventType = "run.completed"
)

// RunEvent is the concrete shape Hub.Publish carries as its Payload for pipeline run lifecycle
// events (§7.5). Hub itself stays agnostic (Event.Payload any); this is what the orchestrator
// publishes and, later, the SSE layer (task 012) marshals to the client.
type RunEvent struct {
	RunID   kernel.RunID `json:"run_id"`
	Type    RunEventType `json:"type"`
	Stage   Stage        `json:"stage,omitempty"`
	Message string       `json:"message,omitempty"`
}
