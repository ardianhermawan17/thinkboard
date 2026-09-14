package pipeline

import (
	"context"
	"sync"
	"time"

	"thinkboard-backend/internal/shared/kernel"
)

// DefaultRunBudget is the per-run timeout used by RunContext when no other value applies
// (03-backend-folder-architecture.md §7.5, "e.g. 15 min per run").
const DefaultRunBudget = 15 * time.Minute

// RunContext derives a context for a run that survives the originating request's cancellation
// but carries its own timeout budget — "not the request context" (§7.5).
func RunContext(reqCtx context.Context, budget time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(reqCtx), budget)
}

const (
	// DefaultReplayBuffer is how many recent events a topic keeps for replay on reattach.
	DefaultReplayBuffer = 64
	// DefaultSubscriberCap is the per-subscriber channel capacity (§7.5 "SSEBufferSize, e.g. 64").
	DefaultSubscriberCap = 64
)

// Event is one message published to a run's topic. Payload is intentionally opaque — Hub only
// fans out and replays events, it doesn't interpret them; real event shapes arrive once
// orchestrator.go actually publishes to a Hub (a later task).
type Event struct {
	Seq     uint64
	Payload any
}

// Hub is the per-run event broker for SSE attach/reattach (§5.1 hub.go, §7.5). It spawns no
// goroutines of its own: Publish/Subscribe/Close are synchronous, mutex-protected operations,
// which keeps it outside §7.1's "no bare go outside shared/concurrency" rule entirely — a pure
// fan-out hub doesn't need one.
type Hub struct {
	replayBuffer  int
	subscriberCap int

	mu     sync.Mutex
	topics map[kernel.RunID]*topic
}

type topic struct {
	mu        sync.Mutex
	buf       []Event
	seq       uint64
	subs      map[uint64]chan Event
	nextSubID uint64
}

// NewHub constructs a Hub. replayBuffer and subscriberCap fall back to their Default* constants
// when <= 0.
func NewHub(replayBuffer, subscriberCap int) *Hub {
	if replayBuffer <= 0 {
		replayBuffer = DefaultReplayBuffer
	}
	if subscriberCap <= 0 {
		subscriberCap = DefaultSubscriberCap
	}
	return &Hub{
		replayBuffer:  replayBuffer,
		subscriberCap: subscriberCap,
		topics:        make(map[kernel.RunID]*topic),
	}
}

func (h *Hub) topicFor(id kernel.RunID) *topic {
	h.mu.Lock()
	defer h.mu.Unlock()
	t, ok := h.topics[id]
	if !ok {
		t = &topic{subs: make(map[uint64]chan Event)}
		h.topics[id] = t
	}
	return t
}

// Publish appends an event to the run's topic (assigning it the next Seq) and delivers it to
// every current subscriber. A subscriber whose channel is full is dropped — its channel closed,
// removed from the topic — instead of blocking Publish; a stalled SSE client must not stall the
// run (§7.5).
func (h *Hub) Publish(id kernel.RunID, payload any) Event {
	t := h.topicFor(id)
	t.mu.Lock()
	defer t.mu.Unlock()

	t.seq++
	ev := Event{Seq: t.seq, Payload: payload}

	t.buf = append(t.buf, ev)
	if len(t.buf) > h.replayBuffer {
		t.buf = t.buf[len(t.buf)-h.replayBuffer:]
	}

	for subID, ch := range t.subs {
		select {
		case ch <- ev:
		default:
			close(ch)
			delete(t.subs, subID)
		}
	}
	return ev
}

// Subscribe attaches to the run's topic. It returns buffered events with Seq > afterSeq (pass 0
// for a fresh attach) as replay, a channel for subsequent live events, and an unsubscribe func
// that is safe to call more than once — including after Publish already dropped the subscriber
// for being slow.
func (h *Hub) Subscribe(id kernel.RunID, afterSeq uint64) (replay []Event, live <-chan Event, unsubscribe func()) {
	t := h.topicFor(id)
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, ev := range t.buf {
		if ev.Seq > afterSeq {
			replay = append(replay, ev)
		}
	}

	subID := t.nextSubID
	t.nextSubID++
	ch := make(chan Event, h.subscriberCap)
	t.subs[subID] = ch

	unsubscribe = func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		if existing, ok := t.subs[subID]; ok {
			close(existing)
			delete(t.subs, subID)
		}
	}
	return replay, ch, unsubscribe
}

// Close tears down a run's topic: every live subscriber's channel is closed and the topic is
// removed from the hub. Safe to call on a run with no topic (no-op) or more than once.
func (h *Hub) Close(id kernel.RunID) {
	h.mu.Lock()
	t, ok := h.topics[id]
	if ok {
		delete(h.topics, id)
	}
	h.mu.Unlock()
	if !ok {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	for subID, ch := range t.subs {
		close(ch)
		delete(t.subs, subID)
	}
}
