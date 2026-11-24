package service

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	editormodels "github.com/kandev/kandev/internal/editors/models"
	editorstore "github.com/kandev/kandev/internal/editors/store"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	usermodels "github.com/kandev/kandev/internal/user/models"
)

type openEditorRepository struct {
	editorstore.Repository
	editor *editormodels.Editor
}

func (r *openEditorRepository) GetEditorByID(_ context.Context, editorID string) (*editormodels.Editor, error) {
	if r.editor != nil && r.editor.ID == editorID {
		return r.editor, nil
	}
	return nil, nil
}

type openEditorTaskRepository struct {
	session  *taskmodels.TaskSession
	executor *taskmodels.Executor
}

func (r *openEditorTaskRepository) GetTaskSession(_ context.Context, _ string) (*taskmodels.TaskSession, error) {
	return r.session, nil
}

func (r *openEditorTaskRepository) GetRepository(_ context.Context, _ string) (*taskmodels.Repository, error) {
	return nil, nil
}

func (r *openEditorTaskRepository) GetExecutor(_ context.Context, _ string) (*taskmodels.Executor, error) {
	return r.executor, nil
}

type openEditorUserSettings struct {
	defaultEditorID string
}

func (s *openEditorUserSettings) GetUserSettings(_ context.Context) (*usermodels.UserSettings, error) {
	return &usermodels.UserSettings{DefaultEditorID: s.defaultEditorID}, nil
}

func (s *openEditorUserSettings) ClearDefaultEditorID(_ context.Context, _ string) error {
	return nil
}

func TestOpenEditor_InternalVscodeWithoutWorkspace(t *testing.T) {
	const editorID = "embedded-vscode"
	svc := NewService(
		&openEditorRepository{editor: &editormodels.Editor{
			ID:      editorID,
			Kind:    editorKindInternalVscode,
			Enabled: true,
		}},
		&openEditorTaskRepository{
			session:  &taskmodels.TaskSession{ID: "session-1", ExecutorID: "executor-1"},
			executor: &taskmodels.Executor{ID: "executor-1", Type: taskmodels.ExecutorTypeLocalDocker},
		},
		&openEditorUserSettings{defaultEditorID: editorID},
	)

	url, err := svc.OpenEditor(context.Background(), OpenEditorInput{
		SessionID: "session-1",
		EditorID:  editorID,
	})
	if err != nil {
		t.Fatalf("OpenEditor() error = %v, want nil", err)
	}
	if url != "internal://vscode" {
		t.Fatalf("OpenEditor() URL = %q, want internal://vscode", url)
	}
}

func TestOpenEditor_InternalVscodeRejectsMissingSession(t *testing.T) {
	const editorID = "embedded-vscode"
	svc := NewService(
		&openEditorRepository{editor: &editormodels.Editor{
			ID:      editorID,
			Kind:    editorKindInternalVscode,
			Enabled: true,
		}},
		&openEditorTaskRepository{},
		&openEditorUserSettings{defaultEditorID: editorID},
	)

	_, err := svc.OpenEditor(context.Background(), OpenEditorInput{
		SessionID: "missing-session",
		EditorID:  editorID,
	})
	if err != ErrWorkspaceNotFound {
		t.Fatalf("OpenEditor() error = %v, want ErrWorkspaceNotFound", err)
	}
}

func TestOpenEditor_InternalVscodeRejectsUnsupportedExecutor(t *testing.T) {
	const editorID = "embedded-vscode"
	svc := NewService(
		&openEditorRepository{editor: &editormodels.Editor{
			ID:      editorID,
			Kind:    editorKindInternalVscode,
			Enabled: true,
		}},
		&openEditorTaskRepository{
			session:  &taskmodels.TaskSession{ID: "session-1", ExecutorID: "executor-1"},
			executor: &taskmodels.Executor{ID: "executor-1", Type: taskmodels.ExecutorTypeMockRemote},
		},
		&openEditorUserSettings{defaultEditorID: editorID},
	)

	for _, input := range []OpenEditorInput{
		{SessionID: "session-1"},
		{SessionID: "session-1", EditorID: editorID, FilePath: "main.go"},
	} {
		_, err := svc.OpenEditor(context.Background(), input)
		if err != ErrEditorUnavailable {
			t.Fatalf("OpenEditor(%+v) error = %v, want ErrEditorUnavailable", input, err)
		}
	}
}

func TestOpenFolder_EmptySessionID(t *testing.T) {
	svc := &Service{}
	err := svc.OpenFolder(context.Background(), "", "")
	if err != ErrEditorConfigInvalid {
		t.Errorf("expected ErrEditorConfigInvalid, got %v", err)
	}
}

func TestResolveSessionPath_NilSession(t *testing.T) {
	svc := &Service{}
	_, err := svc.resolveSessionPath(context.Background(), nil, "")
	if err != ErrWorkspaceNotFound {
		t.Errorf("expected ErrWorkspaceNotFound, got %v", err)
	}
}

func TestResolveSessionPath_WithWorktree(t *testing.T) {
	svc := &Service{}
	session := &taskmodels.TaskSession{
		ID: "session-1",
		Worktrees: []*taskmodels.TaskEnvironmentRepo{
			{WorktreePath: "/path/to/worktree"},
		},
	}
	path, err := svc.resolveSessionPath(context.Background(), session, "")
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
		Worktrees: []*taskmodels.TaskEnvironmentRepo{
			{WorktreePath: ""},
		},
	}
	_, err := svc.resolveSessionPath(context.Background(), session, "")
	if err != ErrWorkspaceNotFound {
		t.Errorf("expected ErrWorkspaceNotFound, got %v", err)
	}
}

func TestResolveSessionPath_SelectsWorktreeByID(t *testing.T) {
	svc := &Service{}
	session := &taskmodels.TaskSession{
		ID: "session-1",
		Worktrees: []*taskmodels.TaskEnvironmentRepo{
			{ID: "assoc-1", WorktreeID: "wt-1", WorktreePath: "/path/to/repo-a"},
			{ID: "assoc-2", WorktreeID: "wt-2", WorktreePath: "/path/to/repo-b"},
		},
	}

	tests := []struct {
		name       string
		worktreeID string
		expected   string
		expectErr  error
	}{
		{name: "by worktree id", worktreeID: "wt-2", expected: "/path/to/repo-b"},
		{name: "by association id", worktreeID: "assoc-2", expected: "/path/to/repo-b"},
		{name: "empty falls back to first", worktreeID: "", expected: "/path/to/repo-a"},
		{name: "unknown id", worktreeID: "wt-missing", expectErr: ErrWorkspaceNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := svc.resolveSessionPath(context.Background(), session, tt.worktreeID)
			if err != tt.expectErr {
				t.Fatalf("expected error %v, got %v", tt.expectErr, err)
			}
			if path != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, path)
			}
		})
	}
}

func TestResolveSessionPath_SelectedWorktreeEmptyPath(t *testing.T) {
	svc := &Service{}
	session := &taskmodels.TaskSession{
		ID: "session-1",
		Worktrees: []*taskmodels.TaskEnvironmentRepo{
			{ID: "assoc-1", WorktreeID: "wt-1", WorktreePath: "/path/to/repo-a"},
			{ID: "assoc-2", WorktreeID: "wt-2", WorktreePath: ""},
		},
	}
	_, err := svc.resolveSessionPath(context.Background(), session, "wt-2")
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
			result, err := buildHostedURL(tt.baseURL, "", tt.path, tt.line, tt.column)
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

func TestLaunchCommandKeepsPlaceholderInsideOneArgument(t *testing.T) {
	worktreePath := t.TempDir()
	absPath := filepath.Join(worktreePath, `a" --evil "b`)
	previous := startProcess
	defer func() { startProcess = previous }()
	var got []string
	startProcess = func(cmd *exec.Cmd) error {
		got = append([]string(nil), cmd.Args...)
		return nil
	}

	if _, err := launchCommand(`code "{file}"`, worktreePath, absPath, 0, 0); err != nil {
		t.Fatalf("launchCommand() error = %v, want nil", err)
	}
	want := []string{"code", absPath}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("launch arguments = %#v, want %#v", got, want)
	}
}

func TestBuildCommandArgs(t *testing.T) {
	worktreePath := filepath.Join(t.TempDir(), "workspace with 'quotes")
	line, column := 10, 5
	templates := []struct {
		name       string
		template   string
		tokenCount int
		want       func(string) []string
	}{
		{"unquoted file", `code {file}`, 2, func(file string) []string { return []string{"code", file} }},
		{"double quoted file", `code "{file}"`, 2, func(file string) []string { return []string{"code", file} }},
		{"single quoted file", `code '{file}'`, 2, func(file string) []string { return []string{"code", file} }},
		{"combined placeholders", `code --goto {file}:{line}:{column}`, 3, func(file string) []string { return []string{"code", "--goto", file + ":10:5"} }},
		{"quoted option value", `code --goto="{file}"`, 2, func(file string) []string { return []string{"code", "--goto=" + file} }},
	}
	filenames := []string{
		"space name.go", `double"quote.go`, "single'quote.go", `back\slash.go`,
		"$(id).go", "`id`.go", "semi;colon.go", "pipe|name.go", "and&name.go", `a" --evil "b`,
	}

	for _, template := range templates {
		for _, filename := range filenames {
			t.Run(template.name+"/"+filename, func(t *testing.T) {
				file := filepath.Join(worktreePath, filename)
				got, err := buildCommandArgs(template.template, worktreePath, file, line, column)
				if err != nil {
					t.Fatalf("buildCommandArgs() error = %v, want nil", err)
				}
				if len(got) != template.tokenCount {
					t.Fatalf("argument count = %d, want template token count %d: %#v", len(got), template.tokenCount, got)
				}
				if want := template.want(file); !reflect.DeepEqual(got, want) {
					t.Fatalf("arguments = %#v, want %#v", got, want)
				}
			})
		}
	}
}

func TestBuildCommandArgsWorktreeAndRelativePlaceholders(t *testing.T) {
	worktreePath := filepath.Join(t.TempDir(), "workspace with 'quotes")
	file := filepath.Join(worktreePath, "src", "main.go")
	for _, template := range []string{`code {cwd}`, `code "{cwd}"`, `code '{cwd}'`} {
		got, err := buildCommandArgs(template, worktreePath, file, 10, 5)
		if err != nil {
			t.Fatalf("buildCommandArgs(%q) error = %v, want nil", template, err)
		}
		if want := []string{"code", worktreePath}; !reflect.DeepEqual(got, want) {
			t.Errorf("arguments for %q = %#v, want %#v", template, got, want)
		}
	}

	got, err := buildCommandArgs(`code "{cwd}" '{file}' {rel} {line} {column}`, worktreePath, file, 10, 5)
	if err != nil {
		t.Fatalf("buildCommandArgs(all placeholders) error = %v, want nil", err)
	}
	want := []string{"code", worktreePath, file, filepath.Join("src", "main.go"), "10", "5"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("all placeholder arguments = %#v, want %#v", got, want)
	}

	got, err = buildCommandArgs("code --new-window", worktreePath, file, 0, 0)
	if err != nil {
		t.Fatalf("buildCommandArgs(no placeholders) error = %v, want nil", err)
	}
	if want := []string{"code", "--new-window"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("no-placeholder arguments = %#v, want %#v", got, want)
	}
}

func TestBuildCommandArgsRejectsInvalidTemplate(t *testing.T) {
	for _, template := range []string{`code "{file}`, `code "é`, `code "escaped\"`, "", "   "} {
		t.Run(template, func(t *testing.T) {
			got, err := buildCommandArgs(template, "/workspace", "/workspace/main.go", 0, 0)
			if !errors.Is(err, ErrEditorConfigInvalid) {
				t.Fatalf("buildCommandArgs() error = %v, want ErrEditorConfigInvalid", err)
			}
			if len(got) != 0 {
				t.Fatalf("arguments = %#v, want none", got)
			}
		})
	}
}

// @covers AC-WORKSPACES-EDITOR-COMMAND-001.4
func TestLaunchCommandRejectsInvalidTemplateWithoutStarting(t *testing.T) {
	previous := startProcess
	defer func() { startProcess = previous }()
	starts := 0
	startProcess = func(*exec.Cmd) error {
		starts++
		return nil
	}
	for _, template := range []string{`code "{file}`, `code "é`, `code "escaped\"`, ""} {
		t.Run(template, func(t *testing.T) {
			_, err := launchCommand(template, "/workspace", "/workspace/main.go", 0, 0)
			if !errors.Is(err, ErrEditorConfigInvalid) {
				t.Fatalf("launchCommand() error = %v, want ErrEditorConfigInvalid", err)
			}
		})
	}
	if starts != 0 {
		t.Fatalf("process starts = %d, want 0", starts)
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

func TestBuildInternalVscodeURL(t *testing.T) {
	tests := []struct {
		name         string
		worktreePath string
		absPath      string
		line         int
		column       int
		expected     string
	}{
		{
			name:         "empty path returns base URL",
			worktreePath: "/workspace",
			absPath:      "",
			line:         0,
			column:       0,
			expected:     "internal://vscode",
		},
		{
			name:         "path same as worktree returns base URL",
			worktreePath: "/workspace",
			absPath:      "/workspace",
			line:         0,
			column:       0,
			expected:     "internal://vscode",
		},
		{
			name:         "file path without line",
			worktreePath: "/workspace",
			absPath:      "/workspace/src/main.go",
			line:         0,
			column:       0,
			expected:     "internal://vscode?goto=src%2Fmain.go",
		},
		{
			name:         "file path with line",
			worktreePath: "/workspace",
			absPath:      "/workspace/src/main.go",
			line:         42,
			column:       0,
			expected:     "internal://vscode?goto=src%2Fmain.go&line=42",
		},
		{
			name:         "file path with line and column",
			worktreePath: "/workspace",
			absPath:      "/workspace/src/main.go",
			line:         42,
			column:       10,
			expected:     "internal://vscode?column=10&goto=src%2Fmain.go&line=42",
		},
		{
			name:         "empty worktree uses absolute path",
			worktreePath: "",
			absPath:      "/some/file.go",
			line:         5,
			column:       0,
			expected:     "internal://vscode?goto=%2Fsome%2Ffile.go&line=5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildInternalVscodeURL(tt.worktreePath, tt.absPath, tt.line, tt.column)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
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
