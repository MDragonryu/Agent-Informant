package usage

import (
	"context"
	"time"
)

type WatchEventType string

const (
	WatchInitial     WatchEventType = "initial"
	WatchStateChange WatchEventType = "state_changed"
	WatchError       WatchEventType = "error"
)

type WatchEvent struct {
	Type          WatchEventType `json:"type"`
	ObservedAt    time.Time      `json:"observed_at"`
	PreviousState *State         `json:"previous_state,omitempty"`
	Advice        *Advice        `json:"advice,omitempty"`
	Error         string         `json:"error,omitempty"`
}

type Watcher struct {
	Collector Collector
	Policy    Policy
	Provider  string
	Interval  time.Duration
	Timeout   time.Duration
}

func (w Watcher) Run(ctx context.Context, emit func(WatchEvent) error) error {
	interval := w.Interval
	if interval <= 0 {
		interval = time.Minute
	}
	timeout := w.Timeout
	if timeout <= 0 {
		timeout = 90 * time.Second
	}

	var lastState *State
	collect := func() error {
		collectCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		snapshot, err := w.Collector.Collect(collectCtx, w.Provider)
		if err != nil {
			return emit(WatchEvent{Type: WatchError, ObservedAt: time.Now().UTC(), Error: err.Error()})
		}
		advice, err := w.Policy.Evaluate(snapshot)
		if err != nil {
			return emit(WatchEvent{Type: WatchError, ObservedAt: time.Now().UTC(), Error: err.Error()})
		}

		if lastState == nil {
			state := advice.State
			lastState = &state
			return emit(WatchEvent{Type: WatchInitial, ObservedAt: time.Now().UTC(), Advice: &advice})
		}
		if advice.State != *lastState {
			previous := *lastState
			state := advice.State
			lastState = &state
			return emit(WatchEvent{Type: WatchStateChange, ObservedAt: time.Now().UTC(), PreviousState: &previous, Advice: &advice})
		}
		return nil
	}

	if err := collect(); err != nil {
		return err
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := collect(); err != nil {
				return err
			}
		}
	}
}
