package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	editormodels "github.com/kandev/kandev/internal/editors/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type embeddedTargetTasks struct {
	session    *taskmodels.TaskSession
	repository *taskmodels.Repository
	executor   *taskmodels.Executor
}

func (r *embeddedTargetTasks) GetTaskSession(_ context.Context, id string) (*taskmodels.TaskSession, error) {
	if r.session != nil && r.session.ID == id {
		return r.session, nil
	}
	return nil, nil
}

func (r *embeddedTargetTasks) GetRepository(_ context.Context, id string) (*taskmodels.Repository, error) {
	if r.repository != nil && r.repository.ID == id {
		return r.repository, nil
	}
	return nil, nil
}

func (r *embeddedTargetTasks) GetExecutor(_ context.Context, id string) (*taskmodels.Executor, error) {
	if r.executor != nil && r.executor.ID == id {
		return r.executor, nil
	}
	return nil, nil
}

func newEmbeddedTargetService(t *testing.T) (*Service, *embeddedTargetTasks, *editormodels.Editor) {
	t.Helper()
	editor := &editormodels.Editor{ID: "embedded-vscode", Kind: editorKindInternalVscode, Enabled: true}
	tasks := &embeddedTargetTasks{
		session: &taskmodels.TaskSession{
			ID: "session-1", ExecutorID: "executor-1",
			Worktrees: []*taskmodels.TaskEnvironmentRepo{
				{ID: "assoc-1", WorktreeID: "wt-1", WorktreePath: filepath.Join(t.TempDir(), "repo-a")},
			},
		},
		executor: &taskmodels.Executor{ID: "executor-1", Type: taskmodels.ExecutorTypeLocalDocker},
	}
	return NewService(&openEditorRepository{editor: editor}, tasks,
		&openEditorUserSettings{defaultEditorID: editor.ID}), tasks, editor
}

type embeddedTargetFixture struct {
	Name  string `json:"name"`
	Input struct {
		Path   string `json:"path"`
		Line   int    `json:"line"`
		Column int    `json:"column"`
	} `json:"input"`
	URL string `json:"url"`
}

// @covers AC-UI-EMBEDDED-EDITOR-TARGET-001.1
// @covers AC-UI-EMBEDDED-EDITOR-TARGET-001.2
// @covers AC-UI-EMBEDDED-EDITOR-TARGET-001.3
func TestOpenEditor_EmbeddedFileTargets(t *testing.T) {
	data, err := os.ReadFile("testdata/embedded-editor-file-targets.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []embeddedTargetFixture
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	if len(fixtures) == 0 {
		t.Fatal("empty target oracle")
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			svc, _, _ := newEmbeddedTargetService(t)
			actual, err := svc.OpenEditor(context.Background(), OpenEditorInput{
				SessionID: "session-1", FilePath: fixture.Input.Path,
				Line: fixture.Input.Line, Column: fixture.Input.Column,
			})
			if err != nil {
				t.Fatal(err)
			}
			if actual != fixture.URL {
				t.Fatalf("OpenEditor target = %q, want %q", actual, fixture.URL)
			}
		})
	}
}

// @covers AC-UI-EMBEDDED-EDITOR-TARGET-001.4
// @covers AC-UI-EMBEDDED-EDITOR-TARGET-001.5
func TestOpenEditor_EmbeddedTargetControls(t *testing.T) {
	t.Run("workspace and root actions", testEmbeddedWorkspaceActions)
	t.Run("selected worktree and repository fallback", testEmbeddedWorktreeSelection)
	t.Run("rejected targets", testEmbeddedTargetRejections)
	t.Run("external editor", func(t *testing.T) {
		svc, _, editor := newEmbeddedTargetService(t)
		editor.Kind = editorKindCustomHostedURL
		editor.Config = json.RawMessage(`{"url":"https://editor.example/open"}`)
		actual, err := svc.OpenEditor(context.Background(), OpenEditorInput{SessionID: "session-1", FilePath: "main.go"})
		if err != nil || !strings.HasPrefix(actual, "https://") {
			t.Fatalf("external editor = %q, %v", actual, err)
		}
	})
}

func testEmbeddedWorkspaceActions(t *testing.T) {
	svc, tasks, _ := newEmbeddedTargetService(t)
	for _, path := range []string{"", "."} {
		actual, err := svc.OpenEditor(context.Background(), OpenEditorInput{
			SessionID: "session-1", FilePath: path, Line: 17, Column: 5,
		})
		if err != nil || actual != "internal://vscode" {
			t.Fatalf("workspace %q = %q, %v", path, actual, err)
		}
	}
	tasks.session.Worktrees = nil
	actual, err := svc.OpenEditor(context.Background(), OpenEditorInput{SessionID: "session-1"})
	if err != nil || actual != "internal://vscode" {
		t.Fatalf("repository-less workspace = %q, %v", actual, err)
	}
}

func testEmbeddedWorktreeSelection(t *testing.T) {
	svc, tasks, _ := newEmbeddedTargetService(t)
	tasks.session.Worktrees[0].WorktreePath = ""
	tasks.session.Worktrees = append(tasks.session.Worktrees, &taskmodels.TaskEnvironmentRepo{
		ID: "assoc-2", WorktreeID: "wt-2", WorktreePath: filepath.Join(t.TempDir(), "repo-b"),
	})
	for _, selection := range []string{"wt-2", "assoc-2"} {
		actual, err := svc.OpenEditor(context.Background(), OpenEditorInput{
			SessionID: "session-1", WorktreeID: selection, FilePath: "src/./plain.ts", Line: 17,
		})
		if err != nil || actual != "internal://vscode?goto=src%2Fplain.ts&line=17" {
			t.Fatalf("selection %q = %q, %v", selection, actual, err)
		}
	}
	tasks.session.Worktrees = nil
	tasks.session.RepositoryID = "repo-1"
	tasks.repository = &taskmodels.Repository{ID: "repo-1", LocalPath: t.TempDir()}
	actual, err := svc.OpenEditor(context.Background(), OpenEditorInput{SessionID: "session-1", FilePath: "main.go"})
	if err != nil || actual != "internal://vscode?goto=main.go" {
		t.Fatalf("repository fallback = %q, %v", actual, err)
	}
}

func testEmbeddedTargetRejections(t *testing.T) {
	cases := []struct {
		name   string
		input  OpenEditorInput
		change func(*embeddedTargetTasks, *editormodels.Editor)
		want   error
	}{
		{name: "missing session", input: OpenEditorInput{SessionID: "missing"}, want: ErrWorkspaceNotFound},
		{name: "empty session", input: OpenEditorInput{}, want: ErrEditorConfigInvalid},
		{name: "invalid worktree", input: OpenEditorInput{SessionID: "session-1", FilePath: "main.go", WorktreeID: "missing"}, want: ErrWorkspaceNotFound},
		{name: "escaping path", input: OpenEditorInput{SessionID: "session-1", FilePath: "../main.go"}, want: ErrEditorConfigInvalid},
		{name: "missing workspace", input: OpenEditorInput{SessionID: "session-1", FilePath: "main.go"},
			change: func(tasks *embeddedTargetTasks, _ *editormodels.Editor) { tasks.session.Worktrees = nil }, want: ErrWorkspaceNotFound},
		{name: "unsupported executor", input: OpenEditorInput{SessionID: "session-1", FilePath: "main.go"},
			change: func(tasks *embeddedTargetTasks, _ *editormodels.Editor) {
				tasks.executor.Type = taskmodels.ExecutorTypeMockRemote
			}, want: ErrEditorUnavailable},
		{name: "disabled editor", input: OpenEditorInput{SessionID: "session-1", FilePath: "main.go"},
			change: func(_ *embeddedTargetTasks, editor *editormodels.Editor) { editor.Enabled = false }, want: ErrEditorUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, tasks, editor := newEmbeddedTargetService(t)
			if tc.change != nil {
				tc.change(tasks, editor)
			}
			actual, err := svc.OpenEditor(context.Background(), tc.input)
			if actual != "" || err != tc.want {
				t.Fatalf("rejected target = %q, %v; want empty, %v", actual, err, tc.want)
			}
		})
	}
}
