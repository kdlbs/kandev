package worktree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasGitCryptFilter(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "file with git-crypt filter",
			content:  "secretfile filter=git-crypt diff=git-crypt\n*.key filter=git-crypt diff=git-crypt",
			expected: true,
		},
		{
			name:     "file without git-crypt filter",
			content:  "*.txt text\n*.bin binary",
			expected: false,
		},
		{
			name:     "empty file",
			content:  "",
			expected: false,
		},
		{
			name:     "filter with different name",
			content:  "*.lfs filter=lfs diff=lfs",
			expected: false,
		},
		{
			name:     "git-crypt in comment",
			content:  "# This uses filter=git-crypt for encryption\n*.txt text",
			expected: true, // We still detect it - conservative approach
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, ".gitattributes")
			if err := os.WriteFile(tmpFile, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to write temp file: %v", err)
			}

			result := hasGitCryptFilter(tmpFile)
			if result != tt.expected {
				t.Errorf("hasGitCryptFilter() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestHasGitCryptFilter_NonexistentFile(t *testing.T) {
	result := hasGitCryptFilter("/nonexistent/path/.gitattributes")
	if result {
		t.Error("hasGitCryptFilter() should return false for nonexistent file")
	}
}

func TestIsGitCryptSmudgeError(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected bool
	}{
		{
			name:     "smudge filter git-crypt failed",
			output:   "fatal: tools/secrets/config.yml: smudge filter git-crypt failed",
			expected: true,
		},
		{
			name:     "external filter git-crypt smudge failed",
			output:   "error: external filter '\"git-crypt\" smudge' failed 1",
			expected: true,
		},
		{
			name:     "external filter without quotes",
			output:   "error: external filter 'git-crypt smudge' failed",
			expected: true,
		},
		{
			name:     "unrelated git error",
			output:   "fatal: pathspec 'foo' did not match any files",
			expected: false,
		},
		{
			name:     "empty output",
			output:   "",
			expected: false,
		},
		{
			name:     "branch checkout error",
			output:   "fatal: 'main' is already checked out at '/path/to/worktree'",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isGitCryptSmudgeError(tt.output)
			if result != tt.expected {
				t.Errorf("isGitCryptSmudgeError(%q) = %v, want %v", tt.output, result, tt.expected)
			}
		})
	}
}

func TestUsesGitCrypt(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a fake git repo structure
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "info"), 0755); err != nil {
		t.Fatalf("failed to create .git/info: %v", err)
	}

	log := newTestLogger()
	m := &Manager{logger: log}

	// Test 1: No .gitattributes
	if m.usesGitCrypt(tmpDir) {
		t.Error("usesGitCrypt() should return false when no .gitattributes exists")
	}

	// Test 2: .gitattributes without git-crypt
	gitattributes := filepath.Join(tmpDir, ".gitattributes")
	if err := os.WriteFile(gitattributes, []byte("*.txt text\n"), 0644); err != nil {
		t.Fatalf("failed to write .gitattributes: %v", err)
	}
	if m.usesGitCrypt(tmpDir) {
		t.Error("usesGitCrypt() should return false when .gitattributes has no git-crypt")
	}

	// Test 3: .gitattributes with git-crypt
	if err := os.WriteFile(gitattributes, []byte("secrets/* filter=git-crypt diff=git-crypt\n"), 0644); err != nil {
		t.Fatalf("failed to write .gitattributes: %v", err)
	}
	if !m.usesGitCrypt(tmpDir) {
		t.Error("usesGitCrypt() should return true when .gitattributes has git-crypt")
	}
}

