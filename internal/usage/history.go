package usage

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	historyMaxAge  = 24 * time.Hour
	historyMaxRows = 5000
)

type HistoryStore struct {
	Path string
}

func DefaultHistoryPath() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache directory: %w", err)
	}
	return filepath.Join(base, "agent-informant", "usage-history.jsonl"), nil
}

func NewHistoryStore() (*HistoryStore, error) {
	path := os.Getenv("AGENT_INFORMANT_HISTORY")
	if path == "" {
		var err error
		path, err = DefaultHistoryPath()
		if err != nil {
			return nil, err
		}
	}
	return &HistoryStore{Path: path}, nil
}

func (s *HistoryStore) Append(snapshot Snapshot) error {
	if snapshot.CollectedAt.IsZero() {
		snapshot.CollectedAt = time.Now().UTC()
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(s.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	err = enc.Encode(snapshot)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}

	if info, statErr := os.Stat(s.Path); statErr == nil && info.Size() > 2*1024*1024 {
		_ = s.Prune(time.Now().UTC())
	}
	return nil
}

func (s *HistoryStore) Load(now time.Time) ([]Snapshot, error) {
	f, err := os.Open(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cutoff := now.Add(-historyMaxAge)
	out := make([]Snapshot, 0)
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		var snapshot Snapshot
		if err := json.Unmarshal(scanner.Bytes(), &snapshot); err != nil {
			continue
		}
		if snapshot.CollectedAt.Before(cutoff) {
			continue
		}
		out = append(out, snapshot)
		if len(out) > historyMaxRows {
			out = out[len(out)-historyMaxRows:]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *HistoryStore) Prune(now time.Time) error {
	rows, err := s.Load(now)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	tmp := s.Path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	for _, row := range rows {
		if err := enc.Encode(row); err != nil {
			f.Close()
			_ = os.Remove(tmp)
			return err
		}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, s.Path)
}

type RecordingCollector struct {
	Collector Collector
	Store     *HistoryStore
}

func (c RecordingCollector) Name() string { return c.Collector.Name() }

func (c RecordingCollector) Collect(ctx context.Context, provider string) (Snapshot, error) {
	snapshot, err := c.Collector.Collect(ctx, provider)
	if err != nil {
		return Snapshot{}, err
	}
	if c.Store != nil {
		_ = c.Store.Append(snapshot)
	}
	return snapshot, nil
}
