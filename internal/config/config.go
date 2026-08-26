package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Messages struct {
	Green    string `json:"green,omitempty"`
	Draining string `json:"draining,omitempty"`
	Critical string `json:"critical,omitempty"`
}

type Usage struct {
	DrainingRemaining float64  `json:"draining_remaining"`
	CriticalRemaining float64  `json:"critical_remaining"`
	Messages          Messages `json:"messages"`
	WatchIntervalSec  int      `json:"watch_interval_seconds"`
}

type Config struct {
	Usage Usage `json:"usage"`
}

func Default() Config {
	return Config{Usage: Usage{
		DrainingRemaining: 25,
		CriticalRemaining: 10,
		WatchIntervalSec:  60,
		Messages: Messages{
			Green:    "Usage headroom is sufficient for normal operation.",
			Draining: "Finish the current coherent unit of work, avoid substantial new work or delegation, then checkpoint before continuing later.",
			Critical: "Stop implementation at the nearest safe point. Make the current state coherent, record a handoff/checkpoint, and do not start new work.",
		},
	}}
}

func DefaultPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(base, "agent-informant", "config.json"), nil
}

func Load(path string) (Config, string, error) {
	cfg := Default()
	if path == "" {
		var err error
		path, err = DefaultPath()
		if err != nil {
			return Config{}, "", err
		}
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, path, nil
	}
	if err != nil {
		return Config{}, path, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, path, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := Validate(cfg); err != nil {
		return Config{}, path, fmt.Errorf("invalid config %s: %w", path, err)
	}
	return cfg, path, nil
}

func Validate(cfg Config) error {
	if cfg.Usage.CriticalRemaining < 0 || cfg.Usage.DrainingRemaining < 0 || cfg.Usage.CriticalRemaining > cfg.Usage.DrainingRemaining || cfg.Usage.DrainingRemaining > 100 {
		return fmt.Errorf("require 0 <= usage.critical_remaining <= usage.draining_remaining <= 100")
	}
	if cfg.Usage.WatchIntervalSec < 1 {
		return fmt.Errorf("usage.watch_interval_seconds must be at least 1")
	}
	return nil
}

func WriteDefault(path string, force bool) (string, error) {
	if path == "" {
		var err error
		path, err = DefaultPath()
		if err != nil {
			return "", err
		}
	}
	if !force {
		if _, err := os.Stat(path); err == nil {
			return path, fmt.Errorf("config already exists: %s (use --force to replace it)", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return path, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return path, err
	}
	data, err := json.MarshalIndent(Default(), "", "  ")
	if err != nil {
		return path, err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return path, err
	}
	return path, nil
}
