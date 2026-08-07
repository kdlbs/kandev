package lsp

import "testing"

func TestLSPExecutorCapabilityFailsClosed(t *testing.T) {
	tests := map[string]bool{
		"local":         true,
		"worktree":      true,
		"local_pc":      true,
		"local_docker":  true,
		"remote_docker": false,
		"ssh":           false,
		"sprites":       false,
		"mock_remote":   false,
		"":              false,
		"future":        false,
	}
	for executorType, want := range tests {
		if got := ExecutorSupportsLSP(executorType); got != want {
			t.Fatalf("ExecutorSupportsLSP(%q) = %v, want %v", executorType, got, want)
		}
	}
}
