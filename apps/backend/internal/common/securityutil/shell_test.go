package securityutil

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
			result := ShellEscape(tt.input)
			if result != tt.expected {
				t.Errorf("ShellEscape(%q) = %q, want %q", tt.input, result, tt.expected)
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
			got, err := SplitShellCommand(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("SplitShellCommand() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("SplitShellCommand() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("SplitShellCommand() arg[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
