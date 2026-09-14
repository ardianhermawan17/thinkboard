package pipeline

import (
	"context"
	"sync"
	"testing"
	"time"

	"thinkboard-backend/internal/shared/kernel"
)

func TestHubPublishSubscribe_BasicDelivery(t *testing.T) {
	h := NewHub(0, 0)
	runID := kernel.RunID("run-1")

	_, live, unsubscribe := h.Subscribe(runID, 0)
	defer unsubscribe()

	ev := h.Publish(runID, "hello")
	if ev.Seq != 1 {
		t.Fatalf("Publish() Seq = %d, want 1", ev.Seq)
	}

	select {
	case got := <-live:
		if got.Payload != "hello" || got.Seq != 1 {
			t.Errorf("live event = %+v, want {Seq:1 Payload:hello}", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for live event")
	}
}

func TestHubSubscribe_ReplayAfterSeq(t *testing.T) {
	h := NewHub(0, 0)
	runID := kernel.RunID("run-1")

	h.Publish(runID, "a")
	h.Publish(runID, "b")
	h.Publish(runID, "c")

	replay, _, unsubscribe := h.Subscribe(runID, 0)
	defer unsubscribe()
	if len(replay) != 3 {
		t.Fatalf("replay from seq 0 = %d events, want 3", len(replay))
	}

	replay2, _, unsubscribe2 := h.Subscribe(runID, 1)
	defer unsubscribe2()
	if len(replay2) != 2 {
		t.Fatalf("replay from seq 1 = %d events, want 2", len(replay2))
	}
	if replay2[0].Payload != "b" || replay2[1].Payload != "c" {
		t.Errorf("replay2 = %+v, want [b c]", replay2)
	}
}

func TestHubPublish_RingBufferEviction(t *testing.T) {
	h := NewHub(2, 0) // capacity 2
	runID := kernel.RunID("run-1")

	h.Publish(runID, "a")
	h.Publish(runID, "b")
	h.Publish(runID, "c") // evicts "a"

	replay, _, unsubscribe := h.Subscribe(runID, 0)
	defer unsubscribe()
	if len(replay) != 2 {
		t.Fatalf("replay = %d events, want 2 (ring buffer capacity)", len(replay))
	}
	if replay[0].Payload != "b" || replay[1].Payload != "c" {
		t.Errorf("replay = %+v, want [b c]", replay)
	}
}

func TestHubPublish_SlowSubscriberDropped(t *testing.T) {
	h := NewHub(0, 1) // subscriber channel capacity 1
	runID := kernel.RunID("run-1")

	_, live, unsubscribe := h.Subscribe(runID, 0)
	defer unsubscribe()

	h.Publish(runID, "a") // fills the channel (capacity 1), not yet drained
	h.Publish(runID, "b") // channel full -> subscriber dropped, must not block

	select {
	case _, ok := <-live:
		if !ok {
			t.Fatal("live channel closed before delivering the first buffered event")
		}
	default:
		t.Fatal("expected the first event to already be buffered in the channel")
	}

	// The subscriber was dropped on the second Publish; the channel should now be closed
	// once drained (or already closed with nothing left).
	_, ok := <-live
	if ok {
		t.Fatal("expected channel to be closed/drained after subscriber was dropped for being slow")
	}
}

func TestHubSubscribe_UnsubscribeStopsDelivery(t *testing.T) {
	h := NewHub(0, 0)
	runID := kernel.RunID("run-1")

	_, live, unsubscribe := h.Subscribe(runID, 0)
	unsubscribe()
	unsubscribe() // double-unsubscribe must not panic

	h.Publish(runID, "a")

	_, ok := <-live
	if ok {
		t.Fatal("expected channel closed after unsubscribe, got an open channel with data")
	}
}

func TestHubClose_ClosesAllSubscribers(t *testing.T) {
	h := NewHub(0, 0)
	runID := kernel.RunID("run-1")

	_, live1, _ := h.Subscribe(runID, 0)
	_, live2, _ := h.Subscribe(runID, 0)

	h.Close(runID)
	h.Close(runID) // must not panic on an already-closed/absent topic

	for i, ch := range []<-chan Event{live1, live2} {
		if _, ok := <-ch; ok {
			t.Fatalf("subscriber %d channel not closed after Hub.Close", i)
		}
	}
}

func TestHub_ConcurrentPublishSubscribe(t *testing.T) {
	h := NewHub(16, 16)
	runID := kernel.RunID("run-1")

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, live, unsubscribe := h.Subscribe(runID, 0)
			defer unsubscribe()
			for {
				select {
				case _, ok := <-live:
					if !ok {
						return
					}
				case <-time.After(50 * time.Millisecond):
					return
				}
			}
		}(i)
	}

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			h.Publish(runID, n)
		}(i)
	}
	wg.Wait()
}

func TestRunContext_SurvivesParentCancel(t *testing.T) {
	reqCtx, cancelReq := context.WithCancel(context.Background())
	runCtx, cancelRun := RunContext(reqCtx, time.Second)
	defer cancelRun()

	cancelReq()
	time.Sleep(10 * time.Millisecond)

	if err := runCtx.Err(); err != nil {
		t.Fatalf("runCtx.Err() = %v, want nil (detached from parent cancellation)", err)
	}
}

func TestRunContext_OwnTimeout(t *testing.T) {
	runCtx, cancel := RunContext(context.Background(), 10*time.Millisecond)
	defer cancel()

	select {
	case <-runCtx.Done():
		if runCtx.Err() != context.DeadlineExceeded {
			t.Errorf("runCtx.Err() = %v, want DeadlineExceeded", runCtx.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for run context's own budget to expire")
	}
}

func TestRunContext_OwnCancel(t *testing.T) {
	runCtx, cancel := RunContext(context.Background(), time.Minute)
	cancel()

	select {
	case <-runCtx.Done():
		if runCtx.Err() != context.Canceled {
			t.Errorf("runCtx.Err() = %v, want Canceled", runCtx.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for explicit cancel to take effect")
	}
}
