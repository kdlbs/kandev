package acp

import (
	"path/filepath"
	"testing"
)

func TestResolvePath(t *testing.T) {
	client := NewClient(WithWorkspaceRoot("/workspace/project"))

	tests := []struct {
		name      string
		input     string
		expected  string
		expectErr bool
	}{
		{
			name:     "absolute path within workspace",
			input:    "/workspace/project/src/main.go",
			expected: "/workspace/project/src/main.go",
		},
		{
			name:     "relative path resolves within workspace",
			input:    "src/main.go",
			expected: filepath.Join("/workspace/project", "src/main.go"),
		},
		{
			name:     "workspace root itself is allowed",
			input:    "/workspace/project",
			expected: "/workspace/project",
		},
		{
			name:     "dot path resolves to workspace root",
			input:    ".",
			expected: "/workspace/project",
		},
		{
			name:      "path traversal with relative path is rejected",
			input:     "../../etc/passwd",
			expectErr: true,
		},
		{
			name:      "path traversal with dot-dot in middle is rejected",
			input:     "src/../../etc/passwd",
			expectErr: true,
		},
		{
			name:      "absolute path outside workspace is rejected",
			input:     "/etc/passwd",
			expectErr: true,
		},
		{
			name:      "absolute path with parent traversal is rejected",
			input:     "/workspace/project/../../../etc/passwd",
			expectErr: true,
		},
		{
			name:     "nested relative path within workspace",
			input:    "src/pkg/handler.go",
			expected: filepath.Join("/workspace/project", "src/pkg/handler.go"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := client.resolvePath(tt.input)
			if tt.expectErr {
				if err == nil {
					t.Errorf("resolvePath(%q) expected error, got path %q", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("resolvePath(%q) unexpected error: %v", tt.input, err)
				return
			}
			if got != tt.expected {
				t.Errorf("resolvePath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
