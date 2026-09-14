package pipeline

import (
	"context"

	"thinkboard-backend/internal/shared/kernel"
)

// Orchestrator drives a single run after Service.StartRun has detached it from the request and
// handed it to the supervisor (03-backend-folder-architecture.md §5.1, §7.2). It only ever sees a
// context.Context and a RunID -- never *gin.Context, never anything HTTP-shaped.
//
// The full 7-stage loop over a stages.Registry() (§9 build order step 8) doesn't exist yet -- only
// the analytic stage has a real handler (step 4, task 009). Run drives exactly that slice: publish
// run.started, run the analytic stage, publish stage.completed or run.failed, and stop. It does not
// claim run.completed, because the run's Status genuinely never reaches Done with six of seven
// stages unimplemented. Whoever implements step 8 extends or replaces this loop; this task does not
// invent that structure speculatively (see agent-history/011-task-detached-pipeline-execution/analyze.json
// open_questions).
type Orchestrator struct {
	svc *Service
	hub *Hub
}

// NewOrchestrator constructs an Orchestrator over svc's run state and hub.
func NewOrchestrator(svc *Service, hub *Hub) *Orchestrator {
	return &Orchestrator{svc: svc, hub: hub}
}

// Run executes runID's currently-implemented pipeline work on ctx (already detached + bounded by
// the caller) and publishes every lifecycle event it reaches to the hub, regardless of whether a
// subscriber is attached (§7.5). A panic during execution is recovered here, marks the run failed,
// and is published as a run.failed event instead of propagating -- Supervisor.Go's own recover is
// a last-resort net for bugs in this method itself, not the primary handler for stage failures.
func (o *Orchestrator) Run(ctx context.Context, runID kernel.RunID) {
	defer func() {
		if r := recover(); r != nil {
			_ = o.svc.mutateRun(runID, func(run *Run) error { return run.Fail(o.svc.clock.Now()) })
			o.hub.Publish(runID, RunEvent{RunID: runID, Type: EventRunFailed, Message: "internal error"})
		}
	}()

	o.hub.Publish(runID, RunEvent{RunID: runID, Type: EventRunStarted})

	if err := o.svc.mutateRun(runID, func(run *Run) error { return run.Start(o.svc.clock.Now()) }); err != nil {
		o.hub.Publish(runID, RunEvent{RunID: runID, Type: EventRunFailed, Message: err.Error()})
		return
	}
	if err := o.svc.mutateRun(runID, func(run *Run) error { return run.Advance(o.svc.clock.Now()) }); err != nil { // scope_anchor -> analytic
		o.hub.Publish(runID, RunEvent{RunID: runID, Type: EventRunFailed, Message: err.Error()})
		return
	}

	select {
	case <-ctx.Done():
		_ = o.svc.mutateRun(runID, func(run *Run) error { return run.Cancel(o.svc.clock.Now()) })
		o.hub.Publish(runID, RunEvent{RunID: runID, Type: EventRunCancelled, Message: ctx.Err().Error()})
		return
	default:
	}

	content, _ := o.svc.contentFor(runID)
	points, err := runAnalytic(ctx, o.svc.llm, runID, content)
	if err != nil {
		_ = o.svc.mutateRun(runID, func(run *Run) error { return run.Fail(o.svc.clock.Now()) })
		o.hub.Publish(runID, RunEvent{RunID: runID, Type: EventRunFailed, Stage: Analytic, Message: err.Error()})
		return
	}
	o.svc.setPoints(runID, points)

	if err := o.svc.mutateRun(runID, func(run *Run) error { return run.Advance(o.svc.clock.Now()) }); err != nil { // analytic -> research
		o.hub.Publish(runID, RunEvent{RunID: runID, Type: EventRunFailed, Message: err.Error()})
		return
	}

	o.hub.Publish(runID, RunEvent{RunID: runID, Type: EventStageCompleted, Stage: Analytic})
}
