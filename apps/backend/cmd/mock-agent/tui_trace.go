package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// Explicit fixture tracing records the process that accepted each test prompt.
func recordTUITrace(kind, prompt string) {
	if !slices.Contains(os.Args, "--tui-trace") {
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	if err := os.MkdirAll(".kandev", 0o700); err != nil {
		return
	}
	file, err := os.OpenFile(filepath.Join(".kandev", "e2e-passthrough.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()
	err = json.NewEncoder(file).Encode(struct {
		Kind   string `json:"kind"`
		PID    int    `json:"pid"`
		CWD    string `json:"cwd"`
		Prompt string `json:"prompt,omitempty"`
	}{kind, os.Getpid(), cwd, prompt})
	if err != nil {
		fmt.Fprintf(os.Stderr, "mock-agent trace: %v\n", err)
	}
}
