package service

import (
	"testing"
)

func TestShellEscape(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "''",
		},
		{
			name:     "simple path",
			input:    "/path/to/file.txt",
			expected: "/path/to/file.txt",
		},
		{
			name:     "path with spaces",
			input:    "/path/to/my file.txt",
			expected: "'/path/to/my file.txt'",
		},
		{
			name:     "path with single quote",
			input:    "/path/to/file's.txt",
			expected: "'/path/to/file'\\''s.txt'",
		},
		{
			name:     "path with special chars",
			input:    "/path/to/file$name.txt",
			expected: "'/path/to/file$name.txt'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shellEscape(tt.input)
			if result != tt.expected {
				t.Errorf("shellEscape(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSplitShellCommand(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{
			name:  "simple command",
			input: "echo hello",
			want:  []string{"echo", "hello"},
		},
		{
			name:  "command with single quotes",
			input: "code '/path/to/my file.txt'",
			want:  []string{"code", "/path/to/my file.txt"},
		},
		{
			name:  "command with double quotes",
			input: `code "/path/to/my file.txt"`,
			want:  []string{"code", "/path/to/my file.txt"},
		},
		{
			name:  "command with escaped single quote",
			input: "code '/path/to/file'\\''s.txt'",
			want:  []string{"code", "/path/to/file's.txt"},
		},
		{
			name:  "complex command",
			input: "code '/path with spaces/file.txt':10:5",
			want:  []string{"code", "/path with spaces/file.txt:10:5"},
		},
		{
			name:    "unclosed single quote",
			input:   "code '/path/to/file.txt",
			wantErr: true,
		},
		{
			name:    "unclosed double quote",
			input:   `code "/path/to/file.txt`,
			wantErr: true,
		},
		{
			name:  "escaped backslash",
			input: `code "path\\file.txt"`,
			want:  []string{"code", `path\file.txt`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := splitShellCommand(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("splitShellCommand() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("splitShellCommand() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitShellCommand() arg[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestExpandCommandPlaceholdersWithSpaces(t *testing.T) {
	tests := []struct {
		name         string
		template     string
		worktreePath string
		absPath      string
		line         int
		column       int
		expected     string
	}{
		{
			name:         "template with path containing spaces",
			template:     "code {file}:{line}:{column}",
			worktreePath: "/workspace",
			absPath:      "/workspace/my file.txt",
			line:         10,
			column:       5,
			expected:     "code '/workspace/my file.txt':10:5",
		},
		{
			name:         "template with path containing single quote",
			template:     "code {file}",
			worktreePath: "/workspace",
			absPath:      "/workspace/user's file.txt",
			line:         0,
			column:       0,
			expected:     "code '/workspace/user'\\''s file.txt'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandCommandPlaceholders(tt.template, tt.worktreePath, tt.absPath, tt.line, tt.column)
			if result != tt.expected {
				t.Errorf("expandCommandPlaceholders() = %q, want %q", result, tt.expected)
			}
		})
	}
}
