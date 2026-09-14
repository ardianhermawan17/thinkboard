package pipeline

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"

	"thinkboard-backend/internal/shared/clock"
	"thinkboard-backend/internal/shared/kernel"
)

// ErrContentRequired is returned by StartRun when cmd.Content is empty.
var ErrContentRequired = errors.New("pipeline: content is required to start a run")

// StartRunCmd is the input to Service.StartRun. Content seeds the analytic stage — there is no
// memory/message-fetch layer yet to source it from automatically.
type StartRunCmd struct {
	SessionID kernel.SessionID
	Mode      Mode
	Content   string
}

type runRecord struct {
	run    *Run
	points []Point
}

// Service is the pipeline write-side entrypoint (03-backend-folder-architecture.md §5.1). This
// build order step 4 slice only wires StartRun through the analytic stage; AdvanceStage,
// RetryStage, CancelRun, and Render are added as later stages/steps need them. State is kept
// in-memory (no Store interface yet — nothing but this task's own tests reads it back).
type Service struct {
	llm   LLM
	clock clock.Clock

	mu   sync.Mutex
	runs map[kernel.RunID]*runRecord
}

// NewService constructs a Service with its dependencies.
func NewService(llm LLM, clk clock.Clock) *Service {
	return &Service{llm: llm, clock: clk, runs: make(map[kernel.RunID]*runRecord)}
}

// StartRun creates a Run, advances it past scope_anchor (no handler yet — the context-warning
// gate is a later task), runs the stubbed analytic stage, and advances past it on success.
func (s *Service) StartRun(ctx context.Context, cmd StartRunCmd) (kernel.RunID, error) {
	if cmd.Content == "" {
		return "", ErrContentRequired
	}

	id := kernel.RunID(uuid.NewString())
	run := NewRun(id, cmd.SessionID, cmd.Mode)
	now := s.clock.Now()

	if err := run.Start(now); err != nil {
		return "", err
	}
	if err := run.Advance(now); err != nil { // scope_anchor -> analytic
		return "", err
	}

	points, err := runAnalytic(ctx, s.llm, id, cmd.Content)
	if err != nil {
		_ = run.Fail(s.clock.Now())
		s.save(id, run, nil)
		return id, err
	}

	if err := run.Advance(s.clock.Now()); err != nil { // analytic -> research
		return "", err
	}
	s.save(id, run, points)
	return id, nil
}

func (s *Service) save(id kernel.RunID, run *Run, points []Point) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[id] = &runRecord{run: run, points: points}
}

// RunState returns the recorded Run and its Points, for callers (and tests) that need to
// observe StartRun's result until a real read side exists.
func (s *Service) RunState(id kernel.RunID) (*Run, []Point, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.runs[id]
	if !ok {
		return nil, nil, false
	}
	return rec.run, rec.points, true
}
