package usage

import (
	"context"
	"sync"
	"testing"
	"time"
)

type sequenceCollector struct {
	mu        sync.Mutex
	snapshots []Snapshot
	index     int
}

func (c *sequenceCollector) Name() string { return "sequence" }

func (c *sequenceCollector) Collect(context.Context, string) (Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.index >= len(c.snapshots) {
		return c.snapshots[len(c.snapshots)-1], nil
	}
	s := c.snapshots[c.index]
	c.index++
	return s, nil
}

func TestWatcherEmitsInitialAndStateChangesOnly(t *testing.T) {
	collector := &sequenceCollector{snapshots: []Snapshot{
		{Windows: []Window{{Provider: "codex", Name: "session", PercentRemaining: 60}}},
		{Windows: []Window{{Provider: "codex", Name: "session", PercentRemaining: 55}}},
		{Windows: []Window{{Provider: "codex", Name: "session", PercentRemaining: 20}}},
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	var events []WatchEvent
	watcher := Watcher{Collector: collector, Policy: DefaultPolicy(), Interval: 5 * time.Millisecond}
	err := watcher.Run(ctx, func(event WatchEvent) error {
		events = append(events, event)
		if len(events) == 2 {
			cancel()
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != WatchInitial || events[0].Advice == nil || events[0].Advice.State != StateGreen {
		t.Fatalf("unexpected initial event: %#v", events[0])
	}
	if events[1].Type != WatchStateChange || events[1].PreviousState == nil || *events[1].PreviousState != StateGreen || events[1].Advice == nil || events[1].Advice.State != StateDraining {
		t.Fatalf("unexpected transition event: %#v", events[1])
	}
}
