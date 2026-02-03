package service

import (
	"context"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestOpenFolder_EmptySessionID(t *testing.T) {
	svc := &Service{}
	err := svc.OpenFolder(context.Background(), "")
	if err != ErrEditorConfigInvalid {
		t.Errorf("expected ErrEditorConfigInvalid, got %v", err)
	}
}

func TestResolveSessionPath_NilSession(t *testing.T) {
	svc := &Service{}
	_, err := svc.resolveSessionPath(context.Background(), nil)
	if err != ErrWorkspaceNotFound {
		t.Errorf("expected ErrWorkspaceNotFound, got %v", err)
	}
}

func TestResolveSessionPath_WithWorktree(t *testing.T) {
	svc := &Service{}
	session := &taskmodels.TaskSession{
		ID: "session-1",
		Worktrees: []*taskmodels.TaskSessionWorktree{
			{WorktreePath: "/path/to/worktree"},
		},
	}
	path, err := svc.resolveSessionPath(context.Background(), session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "/path/to/worktree" {
		t.Errorf("expected /path/to/worktree, got %s", path)
	}
}

func TestResolveSessionPath_EmptyWorktreePath(t *testing.T) {
	svc := &Service{}
	session := &taskmodels.TaskSession{
		ID: "session-1",
		Worktrees: []*taskmodels.TaskSessionWorktree{
			{WorktreePath: ""},
		},
	}
	_, err := svc.resolveSessionPath(context.Background(), session)
	if err != ErrWorkspaceNotFound {
		t.Errorf("expected ErrWorkspaceNotFound, got %v", err)
	}
}

func TestResolveFilePath(t *testing.T) {
	svc := &Service{}

	tests := []struct {
		name         string
		worktreePath string
		filePath     string
		expected     string
		expectErr    error
	}{
		{
			name:         "empty worktree path",
			worktreePath: "",
			filePath:     "file.txt",
			expected:     "",
			expectErr:    ErrWorkspaceNotFound,
		},
		{
			name:         "empty file path returns worktree",
			worktreePath: "/workspace",
			filePath:     "",
			expected:     "/workspace",
			expectErr:    nil,
		},
		{
			name:         "valid file path",
			worktreePath: "/workspace",
			filePath:     "src/main.go",
			expected:     "/workspace/src/main.go",
			expectErr:    nil,
		},
		{
			name:         "path traversal blocked",
			worktreePath: "/workspace",
			filePath:     "../etc/passwd",
			expected:     "",
			expectErr:    ErrEditorConfigInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := svc.resolveFilePath(tt.worktreePath, tt.filePath)
			if err != tt.expectErr {
				t.Errorf("expected error %v, got %v", tt.expectErr, err)
			}
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestBuildRemoteSSHURL(t *testing.T) {
	tests := []struct {
		name     string
		scheme   string
		user     string
		host     string
		path     string
		line     int
		column   int
		expected string
	}{
		{
			name:     "basic",
			scheme:   "vscode",
			user:     "",
			host:     "myhost",
			path:     "/home/user/file.txt",
			line:     0,
			column:   0,
			expected: "vscode://vscode-remote/ssh-remote+myhost:/home/user/file.txt",
		},
		{
			name:     "with user",
			scheme:   "vscode",
			user:     "admin",
			host:     "myhost",
			path:     "/home/user/file.txt",
			line:     0,
			column:   0,
			expected: "vscode://vscode-remote/ssh-remote+admin@myhost:/home/user/file.txt",
		},
		{
			name:     "with line",
			scheme:   "vscode",
			user:     "",
			host:     "myhost",
			path:     "/home/user/file.txt",
			line:     10,
			column:   0,
			expected: "vscode://vscode-remote/ssh-remote+myhost:/home/user/file.txt:10",
		},
		{
			name:     "with line and column",
			scheme:   "vscode",
			user:     "",
			host:     "myhost",
			path:     "/home/user/file.txt",
			line:     10,
			column:   5,
			expected: "vscode://vscode-remote/ssh-remote+myhost:/home/user/file.txt:10:5",
		},
		{
			name:     "default scheme",
			scheme:   "",
			user:     "",
			host:     "myhost",
			path:     "/file.txt",
			line:     0,
			column:   0,
			expected: "vscode://vscode-remote/ssh-remote+myhost:/file.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildRemoteSSHURL(tt.scheme, tt.user, tt.host, tt.path, tt.line, tt.column)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestBuildHostedURL(t *testing.T) {
	tests := []struct {
		name      string
		baseURL   string
		path      string
		line      int
		column    int
		expected  string
		expectErr bool
	}{
		{
			name:     "basic with path",
			baseURL:  "https://code.example.com",
			path:     "/home/user/file.txt",
			line:     0,
			column:   0,
			expected: "https://code.example.com?file=%2Fhome%2Fuser%2Ffile.txt",
		},
		{
			name:     "empty path",
			baseURL:  "https://code.example.com",
			path:     "",
			line:     0,
			column:   0,
			expected: "https://code.example.com?folder=%2F",
		},
		{
			name:     "with line",
			baseURL:  "https://code.example.com",
			path:     "/file.txt",
			line:     10,
			column:   0,
			expected: "https://code.example.com?file=%2Ffile.txt%3A10",
		},
		{
			name:     "with line and column",
			baseURL:  "https://code.example.com",
			path:     "/file.txt",
			line:     10,
			column:   5,
			expected: "https://code.example.com?file=%2Ffile.txt%3A10%3A5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := buildHostedURL(tt.baseURL, tt.path, tt.line, tt.column)
			if tt.expectErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestBuildLocalArgs(t *testing.T) {
	tests := []struct {
		name       string
		editorType string
		path       string
		line       int
		column     int
		expected   []string
	}{
		{
			name:       "empty path",
			editorType: "vscode",
			path:       "",
			line:       0,
			column:     0,
			expected:   nil,
		},
		{
			name:       "vscode without line",
			editorType: "vscode",
			path:       "/file.txt",
			line:       0,
			column:     0,
			expected:   []string{"/file.txt"},
		},
		{
			name:       "vscode with line",
			editorType: "vscode",
			path:       "/file.txt",
			line:       10,
			column:     0,
			expected:   []string{"--goto", "/file.txt:10"},
		},
		{
			name:       "vscode with line and column",
			editorType: "vscode",
			path:       "/file.txt",
			line:       10,
			column:     5,
			expected:   []string{"--goto", "/file.txt:10:5"},
		},
		{
			name:       "cursor with line",
			editorType: "cursor",
			path:       "/file.txt",
			line:       10,
			column:     0,
			expected:   []string{"--goto", "/file.txt:10"},
		},
		{
			name:       "other editor",
			editorType: "sublime",
			path:       "/file.txt",
			line:       10,
			column:     5,
			expected:   []string{"/file.txt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildLocalArgs(tt.editorType, tt.path, tt.line, tt.column)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("expected %v, got %v", tt.expected, result)
					return
				}
			}
		})
	}
}

func TestExpandCommandPlaceholders(t *testing.T) {
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
			name:         "all placeholders",
			template:     "code {cwd} {file} {rel} {line} {column}",
			worktreePath: "/workspace",
			absPath:      "/workspace/src/main.go",
			line:         10,
			column:       5,
			expected:     "code /workspace /workspace/src/main.go src/main.go 10 5",
		},
		{
			name:         "no placeholders",
			template:     "code --new-window",
			worktreePath: "/workspace",
			absPath:      "/workspace/file.txt",
			line:         0,
			column:       0,
			expected:     "code --new-window",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandCommandPlaceholders(tt.template, tt.worktreePath, tt.absPath, tt.line, tt.column)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestIsCustomKind(t *testing.T) {
	tests := []struct {
		kind     string
		expected bool
	}{
		{"custom_command", true},
		{"custom_remote_ssh", true},
		{"custom_hosted_url", true},
		{"vscode", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			result := isCustomKind(tt.kind)
			if result != tt.expected {
				t.Errorf("expected %v for kind %q, got %v", tt.expected, tt.kind, result)
			}
		})
	}
}

func TestParseCommandConfig(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		expectErr bool
	}{
		{
			name:      "valid",
			data:      `{"command": "code --new-window"}`,
			expectErr: false,
		},
		{
			name:      "empty command",
			data:      `{"command": ""}`,
			expectErr: true,
		},
		{
			name:      "whitespace command",
			data:      `{"command": "   "}`,
			expectErr: true,
		},
		{
			name:      "empty data",
			data:      "",
			expectErr: true,
		},
		{
			name:      "invalid json",
			data:      "not json",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseCommandConfig([]byte(tt.data))
			if tt.expectErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestParseRemoteSSHConfig(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		expectErr bool
	}{
		{
			name:      "valid with host",
			data:      `{"host": "myserver.com"}`,
			expectErr: false,
		},
		{
			name:      "valid with user and host",
			data:      `{"host": "myserver.com", "user": "admin"}`,
			expectErr: false,
		},
		{
			name:      "empty host",
			data:      `{"host": ""}`,
			expectErr: true,
		},
		{
			name:      "invalid json",
			data:      "not json",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseRemoteSSHConfig([]byte(tt.data))
			if tt.expectErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestParseHostedURLConfig(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		expectErr bool
	}{
		{
			name:      "valid",
			data:      `{"url": "https://code.example.com"}`,
			expectErr: false,
		},
		{
			name:      "empty url",
			data:      `{"url": ""}`,
			expectErr: true,
		},
		{
			name:      "invalid json",
			data:      "not json",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseHostedURLConfig([]byte(tt.data))
			if tt.expectErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
