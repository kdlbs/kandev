package worktree

import (
	"errors"
	"fmt"
	"testing"
)

func TestClassifyGitError_BranchCheckedOut(t *testing.T) {
	output := "fatal: 'feature/pr-branch' is already checked out at '/tmp/worktree-123'"
	err := ClassifyGitError(output, fmt.Errorf("exit status 128"))
	if !errors.Is(err, ErrBranchCheckedOut) {
		t.Fatalf("expected ErrBranchCheckedOut, got: %v", err)
	}
}

func TestClassifyGitError_AuthFailed(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{"could not read username", "fatal: could not read Username for 'https://github.com': terminal prompts disabled"},
		{"authentication failed", "fatal: Authentication failed for 'https://github.com/repo.git'"},
		{"askpass", "fatal: could not read Password via askpass"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ClassifyGitError(tt.output, fmt.Errorf("exit status 128"))
			if !errors.Is(err, ErrAuthFailed) {
				t.Fatalf("expected ErrAuthFailed, got: %v", err)
			}
		})
	}
}

func TestClassifyGitError_NonFastForward(t *testing.T) {
	output := "! [rejected]        main -> main (non-fast-forward)"
	err := ClassifyGitError(output, fmt.Errorf("exit status 1"))
	if !errors.Is(err, ErrNonFastForward) {
		t.Fatalf("expected ErrNonFastForward, got: %v", err)
	}
}

func TestClassifyGitError_GenericFailure(t *testing.T) {
	output := "fatal: some unknown git error"
	err := ClassifyGitError(output, fmt.Errorf("exit status 1"))
	if !errors.Is(err, ErrGitCommandFailed) {
		t.Fatalf("expected ErrGitCommandFailed, got: %v", err)
	}
}

func TestContainsAuthFailure(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"authentication failed", true},
		{"terminal prompts disabled", true},
		{"could not read username", true},
		{"username for 'https://github.com'", true},
		{"askpass error", true},
		{"branch not found", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := containsAuthFailure(tt.input); got != tt.expected {
				t.Fatalf("containsAuthFailure(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseCheckedOutPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			"standard git output",
			"fatal: 'feature/pr-branch' is already checked out at '/tmp/worktrees/my-worktree'",
			"/tmp/worktrees/my-worktree",
		},
		{
			"no match",
			"fatal: some other error",
			"",
		},
		{
			"no closing quote",
			"fatal: 'branch' is already checked out at '/path",
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCheckedOutPath(tt.input)
			if got != tt.expected {
				t.Fatalf("parseCheckedOutPath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
