package pipeline

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	"thinkboard-backend/internal/shared/clock"
	"thinkboard-backend/internal/shared/concurrency"
	"thinkboard-backend/internal/shared/kernel"
)

// ErrContentRequired is returned by StartRun when cmd.Content is empty.
var ErrContentRequired = errors.New("pipeline: content is required to start a run")

// errUnknownRun is an internal sentinel for mutateRun/setPoints/contentFor being called with a
// run id the service never created -- should only happen on a programmer error, not user input.
var errUnknownRun = errors.New("pipeline: unknown run")

// StartRunCmd is the input to Service.StartRun. Content seeds the analytic stage -- there is no
// memory/message-fetch layer yet to source it from automatically.
type StartRunCmd struct {
	SessionID kernel.SessionID
	Mode      Mode
	Content   string
}

type runRecord struct {
	run     *Run
	points  []Point
	content string
}

// Service is the pipeline write-side entrypoint (03-backend-folder-architecture.md §5.1). StartRun
// does its synchronous, request-cancellable validation on the caller's context, then detaches
// execution onto its own bounded context and hands it to the Supervisor (§7.5, §7.1 rule 1) --
// client disconnect never cancels a run already admitted. State is kept in-memory (no Store
// interface yet -- nothing but this task's own tests and the future HTTP layer reads it back).
type Service struct {
	llm   LLM
	clock clock.Clock
	hub   *Hub

	supervisor     *concurrency.Supervisor
	runSemaphore   *concurrency.Semaphore
	maxRunDuration time.Duration
	orchestrator   *Orchestrator

	mu   sync.Mutex
	runs map[kernel.RunID]*runRecord
}

// NewService constructs a Service with its dependencies. maxConcurrentRuns and maxRunDuration fall
// back to sensible defaults when <= 0, mirroring NewHub's Default* fallback pattern.
func NewService(llm LLM, clk clock.Clock, hub *Hub, supervisor *concurrency.Supervisor, maxConcurrentRuns int64, maxRunDuration time.Duration) *Service {
	if maxConcurrentRuns <= 0 {
		maxConcurrentRuns = DefaultMaxConcurrentRuns
	}
	if maxRunDuration <= 0 {
		maxRunDuration = DefaultRunBudget
	}
	s := &Service{
		llm:            llm,
		clock:          clk,
		hub:            hub,
		supervisor:     supervisor,
		runSemaphore:   concurrency.NewSemaphore(maxConcurrentRuns),
		maxRunDuration: maxRunDuration,
		runs:           make(map[kernel.RunID]*runRecord),
	}
	s.orchestrator = NewOrchestrator(s, hub)
	return s
}

// DefaultMaxConcurrentRuns is used by NewService when the caller passes <= 0 (03-backend-folder-architecture.md
// §7.9's worked example, per gateway instance).
const DefaultMaxConcurrentRuns = 24

// StartRun admits a new run and returns immediately after handoff -- it never blocks on the run
// finishing (§4 guarantee 8). Only content validation happens on reqCtx; cancelling reqCtx after
// StartRun returns has no effect on the run (§4 guarantee 1).
func (s *Service) StartRun(reqCtx context.Context, cmd StartRunCmd) (kernel.RunID, error) {
	if cmd.Content == "" {
		return "", ErrContentRequired
	}

	id := kernel.RunID(uuid.NewString())
	s.createRun(id, cmd.SessionID, cmd.Mode, cmd.Content)

	if !s.runSemaphore.TryAcquire(1) {
		// Over MaxConcurrentRuns: admitted as pending, queued (§7.7 "prefer enqueueing to
		// rejecting"). No goroutine starts until a future task adds a drain mechanism -- see
		// analyze.json risks.
		return id, nil
	}

	runCtx, cancel := RunContext(reqCtx, s.maxRunDuration)
	s.supervisor.Go(runCtx, func(ctx context.Context) {
		defer cancel()
		defer s.runSemaphore.Release(1)
		s.orchestrator.Run(ctx, id)
	})

	return id, nil
}

func (s *Service) createRun(id kernel.RunID, sessionID kernel.SessionID, mode Mode, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[id] = &runRecord{run: NewRun(id, sessionID, mode), content: content}
}

// mutateRun applies fn to id's Run under the service lock, so concurrent RunState reads never race
// with the orchestrator goroutine mutating run state.
func (s *Service) mutateRun(id kernel.RunID, fn func(*Run) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.runs[id]
	if !ok {
		return errUnknownRun
	}
	return fn(rec.run)
}

func (s *Service) setPoints(id kernel.RunID, points []Point) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec, ok := s.runs[id]; ok {
		rec.points = points
	}
}

func (s *Service) contentFor(id kernel.RunID) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.runs[id]
	if !ok {
		return "", false
	}
	return rec.content, true
}

// RunState returns a snapshot of the recorded Run and its Points, for callers (and tests) that
// need to observe a run's current state until a real read side exists. It returns a copy, not the
// live *Run the orchestrator goroutine mutates, so a caller reading fields off the result after
// RunState returns can never race with an in-flight run (-race clean, §7.1 rule 5). Since StartRun
// returns before the run finishes, callers that need to know when it's done should subscribe to
// the Hub instead of polling RunState.
func (s *Service) RunState(id kernel.RunID) (*Run, []Point, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.runs[id]
	if !ok {
		return nil, nil, false
	}
	snapshot := *rec.run
	return &snapshot, rec.points, true
}
