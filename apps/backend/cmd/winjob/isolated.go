package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type isolatedIdentity struct {
	PID          int    `json:"pid"`
	StartedTicks string `json:"started_ticks"`
}

// Publish a complete record, so readers never observe partially written JSON.
func writeIsolatedIdentity(path string, identity isolatedIdentity) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".isolated-identity-*")
	if err != nil {
		return fmt.Errorf("create identity: %w", err)
	}
	defer func() { _ = os.Remove(file.Name()) }()
	if err := json.NewEncoder(file).Encode(identity); err != nil {
		_ = file.Close()
		return fmt.Errorf("encode identity: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close identity: %w", err)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("publish identity: %w", err)
	}
	return nil
}
