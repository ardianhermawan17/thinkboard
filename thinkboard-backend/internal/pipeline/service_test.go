package pipeline

import (
	"context"
	"errors"
	"testing"
	"time"

	"thinkboard-backend/internal/shared/concurrency"
	"thinkboard-backend/internal/shared/kernel"
)

// fakeLLM supports three shapes tests need: a plain success/error stub, one that blocks in
// Complete until gate is closed (to deterministically observe StartRun returning before the run
// finishes), and one that panics (to exercise Orchestrator's recover path). started, if set, is
// closed the moment Complete is entered, before any gating -- tests use it to synchronize without
// sleeping.
type fakeLLM struct {
	out       string
	err       error
	wantPanic bool
	gate      chan struct{}
	started   chan struct{}
}

func (f fakeLLM) Complete(ctx context.Context, prompt string) (string, error) {
	if f.started != nil {
		close(f.started)
	}
	if f.gate != nil {
		<-f.gate
	}
	if f.wantPanic {
		panic("boom: fake llm panic")
	}
	return f.out, f.err
}

type fakeClock struct{ now time.Time }

func (f fakeClock) Now() time.Time { return f.now }

func newTestService(llm LLM, maxConcurrentRuns int64) (*Service, *Hub) {
	hub := NewHub(0, 0)
	sup := concurrency.NewSupervisor()
	svc := NewService(llm, fakeClock{now: t0}, hub, sup, maxConcurrentRuns, time.Minute)
	return svc, hub
}

func isTerminal(t RunEventType) bool {
	switch t {
	case EventStageCompleted, EventRunFailed, EventRunCancelled, EventRunCompleted:
		return true
	default:
		return false
	}
}

// waitForTerminal subscribes to id's topic -- capturing anything already published via replay --
// and blocks until a terminal event is observed, returning every event seen in publish order.
// Synchronization is entirely through the hub's own channel; it never sleeps.
func waitForTerminal(t *testing.T, hub *Hub, id kernel.RunID) []RunEvent {
	t.Helper()
	replay, live, unsubscribe := hub.Subscribe(id, 0)
	defer unsubscribe()

	var events []RunEvent
	for _, ev := range replay {
		re := ev.Payload.(RunEvent)
		events = append(events, re)
		if isTerminal(re.Type) {
			return events
		}
	}

	for {
		select {
		case ev, ok := <-live:
			if !ok {
				t.Fatal("waitForTerminal: hub channel closed before a terminal event arrived")
			}
			re := ev.Payload.(RunEvent)
			events = append(events, re)
			if isTerminal(re.Type) {
				return events
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("waitForTerminal: timed out waiting for a terminal event; got %+v so far", events)
		}
	}
}

func TestServiceStartRun_Success(t *testing.T) {
	svc, hub := newTestService(fakeLLM{out: "decomposed point"}, 0)

	id, err := svc.StartRun(context.Background(), StartRunCmd{
		SessionID: "session-1",
		Mode:      ModePlanning,
		Content:   "raw input",
	})
	if err != nil {
		t.Fatalf("StartRun() unexpected error %v", err)
	}
	if id == "" {
		t.Fatal("StartRun() returned empty RunID")
	}

	events := waitForTerminal(t, hub, id)
	if len(events) < 2 || events[0].Type != EventRunStarted {
		t.Fatalf("events = %+v, want [run.started, ..., stage.completed]", events)
	}
	last := events[len(events)-1]
	if last.Type != EventStageCompleted || last.Stage != Analytic {
		t.Fatalf("terminal event = %+v, want stage.completed{analytic}", last)
	}

	run, points, ok := svc.RunState(id)
	if !ok {
		t.Fatalf("RunState(%q) not found", id)
	}
	if run.Status != StatusRunning {
		t.Errorf("run.Status = %q, want %q", run.Status, StatusRunning)
	}
	if run.CurrentStage != Research {
		t.Errorf("run.CurrentStage = %q, want %q (advanced past analytic)", run.CurrentStage, Research)
	}
	if len(points) != 1 {
		t.Fatalf("len(points) = %d, want 1", len(points))
	}
	if points[0].Content != "decomposed point" {
		t.Errorf("points[0].Content = %q, want %q", points[0].Content, "decomposed point")
	}
	if !points[0].IsRoot() {
		t.Errorf("points[0].IsRoot() = false, want true")
	}
}

// TestServiceStartRun_ReturnsBeforeRunCompletes is acceptance criterion g2/g8: StartRun must hand
// off and return without waiting on the run. The gated LLM call proves the run is still mid-flight
// (parked at the analytic stage) after StartRun has already returned.
func TestServiceStartRun_ReturnsBeforeRunCompletes(t *testing.T) {
	gate := make(chan struct{})
	started := make(chan struct{})
	svc, hub := newTestService(fakeLLM{out: "x", gate: gate, started: started}, 0)

	callStart := time.Now()
	id, err := svc.StartRun(context.Background(), StartRunCmd{SessionID: "s1", Mode: ModePlanning, Content: "c"})
	if err != nil {
		t.Fatalf("StartRun() unexpected error %v", err)
	}
	if elapsed := time.Since(callStart); elapsed > 200*time.Millisecond {
		t.Fatalf("StartRun() took %v, want near-instant return (it must not wait on the LLM call)", elapsed)
	}

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the run to reach the (gated) LLM call")
	}

	run, _, ok := svc.RunState(id)
	if !ok {
		t.Fatalf("RunState(%q) not found", id)
	}
	if run.CurrentStage != Analytic {
		t.Fatalf("run.CurrentStage = %q, want %q (should be mid-analytic, gated on the LLM call)", run.CurrentStage, Analytic)
	}
	if run.Status != StatusRunning {
		t.Fatalf("run.Status = %q, want %q", run.Status, StatusRunning)
	}

	close(gate)
	waitForTerminal(t, hub, id)
}

func TestServiceStartRun_LLMError(t *testing.T) {
	wantErr := errors.New("llm unavailable")
	svc, hub := newTestService(fakeLLM{err: wantErr}, 0)

	id, err := svc.StartRun(context.Background(), StartRunCmd{
		SessionID: "session-1",
		Mode:      ModePlanning,
		Content:   "raw input",
	})
	if err != nil {
		t.Fatalf("StartRun() unexpected synchronous error %v (LLM errors surface asynchronously via run status/hub event, not StartRun's return)", err)
	}
	if id == "" {
		t.Fatal("StartRun() returned empty RunID")
	}

	events := waitForTerminal(t, hub, id)
	last := events[len(events)-1]
	if last.Type != EventRunFailed {
		t.Fatalf("terminal event = %+v, want run.failed", last)
	}

	run, points, ok := svc.RunState(id)
	if !ok {
		t.Fatalf("RunState(%q) not found", id)
	}
	if run.Status != StatusFailed {
		t.Errorf("run.Status = %q, want %q", run.Status, StatusFailed)
	}
	if points != nil {
		t.Errorf("points = %v, want nil on failure", points)
	}
}

func TestServiceStartRun_EmptyContent(t *testing.T) {
	svc, _ := newTestService(fakeLLM{out: "unused"}, 0)

	id, err := svc.StartRun(context.Background(), StartRunCmd{SessionID: "session-1", Mode: ModePlanning})
	if err != ErrContentRequired {
		t.Fatalf("StartRun() error = %v, want ErrContentRequired", err)
	}
	if id != "" {
		t.Errorf("StartRun() id = %q, want empty (no run created)", id)
	}
}

// TestServiceStartRun_CallerCancelAfterReturnDoesNotCancelRun is acceptance criterion g2: the
// 011 prompt's MUST guarantee that cancelling the caller's context after StartRun returns has no
// effect on the run.
func TestServiceStartRun_CallerCancelAfterReturnDoesNotCancelRun(t *testing.T) {
	gate := make(chan struct{})
	svc, hub := newTestService(fakeLLM{out: "decomposed point", gate: gate}, 0)

	reqCtx, cancelReq := context.WithCancel(context.Background())
	id, err := svc.StartRun(reqCtx, StartRunCmd{SessionID: "s1", Mode: ModePlanning, Content: "c"})
	if err != nil {
		t.Fatalf("StartRun() unexpected error %v", err)
	}

	cancelReq() // simulate the client disconnecting right after the 202 was handed off
	close(gate) // let the gated LLM call proceed now that reqCtx is already cancelled

	events := waitForTerminal(t, hub, id)
	last := events[len(events)-1]
	if last.Type != EventStageCompleted {
		t.Fatalf("terminal event = %+v, want stage.completed (caller's cancellation must not affect the detached run)", last)
	}
}

// TestServiceStartRun_PanicRecovered is acceptance criterion g4.
func TestServiceStartRun_PanicRecovered(t *testing.T) {
	svc, hub := newTestService(fakeLLM{wantPanic: true}, 0)

	id, err := svc.StartRun(context.Background(), StartRunCmd{SessionID: "s1", Mode: ModePlanning, Content: "c"})
	if err != nil {
		t.Fatalf("StartRun() unexpected error %v", err)
	}

	events := waitForTerminal(t, hub, id)
	last := events[len(events)-1]
	if last.Type != EventRunFailed {
		t.Fatalf("terminal event = %+v, want run.failed after a panic", last)
	}

	run, _, ok := svc.RunState(id)
	if !ok {
		t.Fatalf("RunState(%q) not found", id)
	}
	if run.Status != StatusFailed {
		t.Errorf("run.Status = %q, want %q after a recovered panic", run.Status, StatusFailed)
	}
}

// TestServiceStartRun_LateSubscriberReplay is acceptance criterion g6.
func TestServiceStartRun_LateSubscriberReplay(t *testing.T) {
	svc, hub := newTestService(fakeLLM{out: "decomposed point"}, 0)

	id, err := svc.StartRun(context.Background(), StartRunCmd{SessionID: "s1", Mode: ModePlanning, Content: "c"})
	if err != nil {
		t.Fatalf("StartRun() unexpected error %v", err)
	}
	waitForTerminal(t, hub, id) // let the run fully finish publishing before subscribing "late"

	replay, _, unsubscribe := hub.Subscribe(id, 0)
	defer unsubscribe()
	if len(replay) < 2 {
		t.Fatalf("replay = %d events, want at least run.started + stage.completed", len(replay))
	}
	first := replay[0].Payload.(RunEvent)
	last := replay[len(replay)-1].Payload.(RunEvent)
	if first.Type != EventRunStarted {
		t.Errorf("replay[0].Type = %q, want run.started", first.Type)
	}
	if last.Type != EventStageCompleted {
		t.Errorf("replay[last].Type = %q, want stage.completed", last.Type)
	}
}

// TestServiceStartRun_MaxConcurrentRunsQueuesOverLimit is acceptance criterion g7.
func TestServiceStartRun_MaxConcurrentRunsQueuesOverLimit(t *testing.T) {
	gate := make(chan struct{})
	svc, hub := newTestService(fakeLLM{out: "x", gate: gate}, 1) // MaxConcurrentRuns = 1

	id1, err := svc.StartRun(context.Background(), StartRunCmd{SessionID: "s1", Mode: ModePlanning, Content: "c1"})
	if err != nil {
		t.Fatalf("StartRun() run 1 unexpected error %v", err)
	}

	id2, err := svc.StartRun(context.Background(), StartRunCmd{SessionID: "s2", Mode: ModePlanning, Content: "c2"})
	if err != nil {
		t.Fatalf("StartRun() run 2 unexpected error %v", err)
	}

	run2, _, ok := svc.RunState(id2)
	if !ok {
		t.Fatalf("RunState(%q) not found", id2)
	}
	if run2.Status != StatusPending {
		t.Fatalf("run2.Status = %q, want %q (admitted but queued, no slot available)", run2.Status, StatusPending)
	}

	close(gate)
	waitForTerminal(t, hub, id1)
}
