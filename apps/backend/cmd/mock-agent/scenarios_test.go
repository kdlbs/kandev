package main

import "testing"

func TestRetryableGitSetupOutput(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		retryable bool
	}{
		{name: "index lock", output: "fatal: Unable to create '.git/index.lock': File exists.", retryable: true},
		{name: "concurrent process", output: "fatal: another git process seems to be running", retryable: true},
		{name: "missing path", output: "fatal: pathspec 'missing.txt' did not match any files", retryable: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := retryableGitSetupOutput(tt.output); got != tt.retryable {
				t.Fatalf("retryableGitSetupOutput() = %v, want %v", got, tt.retryable)
			}
		})
	}
}
