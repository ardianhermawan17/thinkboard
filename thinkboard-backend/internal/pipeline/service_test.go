package pipeline

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeLLM struct {
	out string
	err error
}

func (f fakeLLM) Complete(ctx context.Context, prompt string) (string, error) {
	return f.out, f.err
}

type fakeClock struct{ now time.Time }

func (f fakeClock) Now() time.Time { return f.now }

func TestServiceStartRun_Success(t *testing.T) {
	svc := NewService(fakeLLM{out: "decomposed point"}, fakeClock{now: t0})

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

func TestServiceStartRun_LLMError(t *testing.T) {
	wantErr := errors.New("llm unavailable")
	svc := NewService(fakeLLM{err: wantErr}, fakeClock{now: t0})

	id, err := svc.StartRun(context.Background(), StartRunCmd{
		SessionID: "session-1",
		Mode:      ModePlanning,
		Content:   "raw input",
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("StartRun() error = %v, want %v", err, wantErr)
	}
	if id == "" {
		t.Fatal("StartRun() returned empty RunID on LLM failure; want the created (now failed) run's id")
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
	svc := NewService(fakeLLM{out: "unused"}, fakeClock{now: t0})

	id, err := svc.StartRun(context.Background(), StartRunCmd{SessionID: "session-1", Mode: ModePlanning})
	if err != ErrContentRequired {
		t.Fatalf("StartRun() error = %v, want ErrContentRequired", err)
	}
	if id != "" {
		t.Errorf("StartRun() id = %q, want empty (no run created)", id)
	}
}
