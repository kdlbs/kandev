package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"path/filepath"
	"reflect"
	"testing"

	editormodels "github.com/kandev/kandev/internal/editors/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

const hostedWorkspaceBaseURL = "https://hosted.example.test/editor/path?theme=dark&tag=a%2Bb&tag=%23two#panel"

type hostedWorkspaceTaskRepo struct {
	openEditorTaskRepository
	repository *taskmodels.Repository
	readErr    error
}

func (r *hostedWorkspaceTaskRepo) GetTaskSession(ctx context.Context, id string) (*taskmodels.TaskSession, error) {
	if r.readErr != nil {
		return nil, r.readErr
	}
	return r.openEditorTaskRepository.GetTaskSession(ctx, id)
}

func (r *hostedWorkspaceTaskRepo) GetRepository(_ context.Context, _ string) (*taskmodels.Repository, error) {
	return r.repository, nil
}

type hostedWorkspaceFixture struct {
	service *Service
	tasks   *hostedWorkspaceTaskRepo
	editor  *editormodels.Editor
	input   OpenEditorInput
	first   string
	second  string
}

func newHostedWorkspaceFixture(t *testing.T) hostedWorkspaceFixture {
	t.Helper()
	parent := t.TempDir()
	first := filepath.Join(parent, "first")
	second := filepath.Join(parent, "chosen space #plus+日本語")
	config, err := json.Marshal(hostedURLConfig{URL: hostedWorkspaceBaseURL})
	if err != nil {
		t.Fatal(err)
	}
	editor := &editormodels.Editor{ID: "hosted-workspace-editor", Kind: editorKindCustomHostedURL,
		Enabled: true, Config: config}
	tasks := &hostedWorkspaceTaskRepo{openEditorTaskRepository: openEditorTaskRepository{
		session: &taskmodels.TaskSession{ID: "hosted-workspace-session", ExecutorID: "hosted-executor",
			Worktrees: []*taskmodels.TaskEnvironmentRepo{
				{ID: "first-association", WorktreeID: "first-worktree", WorktreePath: first},
				{ID: "chosen-association", WorktreeID: "chosen-worktree", WorktreePath: second},
			}},
		executor: &taskmodels.Executor{ID: "hosted-executor", Type: taskmodels.ExecutorTypeLocalDocker},
	}}
	return hostedWorkspaceFixture{
		service: NewService(&openEditorRepository{editor: editor}, tasks,
			&openEditorUserSettings{defaultEditorID: editor.ID}),
		tasks: tasks, editor: editor, first: first, second: second,
		input: OpenEditorInput{SessionID: tasks.session.ID, EditorID: editor.ID,
			WorktreeID: "chosen-worktree", Line: 13, Column: 6},
	}
}

func assertHostedWorkspaceURL(t *testing.T, target, key, expected string) {
	t.Helper()
	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "https" || parsed.Host != "hosted.example.test" ||
		parsed.Path != "/editor/path" || parsed.Fragment != "panel" {
		t.Fatalf("unexpected configured URL components: %q", target)
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		t.Fatal(err)
	}
	want := url.Values{key: {expected}, "theme": {"dark"}, "tag": {"a+b", "#two"}}
	if !reflect.DeepEqual(query, want) {
		t.Fatalf("decoded query = %v, want %v", query, want)
	}
}

// @covers AC-WORKSPACES-HOSTED-EDITOR-001.1, AC-WORKSPACES-HOSTED-EDITOR-001.2,
// AC-WORKSPACES-HOSTED-EDITOR-001.5
func TestOpenEditor_HostedWorkspaceRoot(t *testing.T) {
	for _, selection := range []string{"chosen-worktree", "chosen-association"} {
		for _, file := range []string{"", ".", "src/.."} {
			t.Run(selection+"/"+file, func(t *testing.T) {
				fixture := newHostedWorkspaceFixture(t)
				fixture.input.WorktreeID = selection
				fixture.input.FilePath = filepath.FromSlash(file)
				target, err := fixture.service.OpenEditor(context.Background(), fixture.input)
				if err != nil {
					t.Fatal(err)
				}
				assertHostedWorkspaceURL(t, target, "folder", fixture.second)
			})
		}
	}
}

// @covers AC-WORKSPACES-HOSTED-EDITOR-001.2
func TestOpenEditor_HostedWorkspaceFallbacks(t *testing.T) {
	for _, repositoryFallback := range []bool{false, true} {
		t.Run(map[bool]string{false: "first worktree", true: "repository"}[repositoryFallback], func(t *testing.T) {
			fixture := newHostedWorkspaceFixture(t)
			fixture.input.WorktreeID = ""
			if repositoryFallback {
				fixture.tasks.session.Worktrees = nil
				fixture.tasks.session.RepositoryID = "fallback-repository"
				fixture.tasks.repository = &taskmodels.Repository{ID: "fallback-repository", LocalPath: fixture.first}
			}
			target, err := fixture.service.OpenEditor(context.Background(), fixture.input)
			if err != nil {
				t.Fatal(err)
			}
			assertHostedWorkspaceURL(t, target, "folder", fixture.first)
		})
	}
}

// @covers AC-WORKSPACES-HOSTED-EDITOR-001.3
func TestOpenEditor_HostedCompatibility(t *testing.T) {
	cases := []struct {
		name   string
		line   int
		column int
		suffix string
	}{
		{name: "no coordinates"},
		{name: "line", line: 13, suffix: ":13"},
		{name: "line and column", line: 13, column: 6, suffix: ":13:6"},
		{name: "column without line", column: 6},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := newHostedWorkspaceFixture(t)
			fixture.input.FilePath = filepath.FromSlash("src/file space #plus+日本語.go")
			fixture.input.Line, fixture.input.Column = test.line, test.column
			target, err := fixture.service.OpenEditor(context.Background(), fixture.input)
			if err != nil {
				t.Fatal(err)
			}
			want := filepath.Join(fixture.second, "src", "file space #plus+日本語.go") + test.suffix
			assertHostedWorkspaceURL(t, target, "file", want)
		})
	}
}

type hostedWorkspaceErrorCase struct {
	name   string
	change func(*hostedWorkspaceFixture)
	want   error
}

func hostedWorkspaceErrorCases() []hostedWorkspaceErrorCase {
	return []hostedWorkspaceErrorCase{
		{"empty session ID", func(f *hostedWorkspaceFixture) { f.input.SessionID = "" }, ErrEditorConfigInvalid},
		{"missing session", func(f *hostedWorkspaceFixture) { f.tasks.session = nil }, ErrWorkspaceNotFound},
		{"missing editor", func(f *hostedWorkspaceFixture) { f.input.EditorID = "unknown-editor" }, ErrEditorNotFound},
		{"disabled editor", func(f *hostedWorkspaceFixture) { f.editor.Enabled = false }, ErrEditorUnavailable},
		{"missing selection", func(f *hostedWorkspaceFixture) { f.input.WorktreeID = "unknown-worktree" }, ErrWorkspaceNotFound},
		{"empty selected root", func(f *hostedWorkspaceFixture) { f.tasks.session.Worktrees[1].WorktreePath = "" }, ErrWorkspaceNotFound},
		{"no workspace", func(f *hostedWorkspaceFixture) {
			f.input.WorktreeID = ""
			f.tasks.session.Worktrees = nil
		}, ErrWorkspaceNotFound},
		{"missing config", func(f *hostedWorkspaceFixture) { f.editor.Config = nil }, ErrEditorConfigInvalid},
		{"empty URL", func(f *hostedWorkspaceFixture) { f.editor.Config = json.RawMessage(`{"url":""}`) }, ErrEditorConfigInvalid},
		{"malformed URL", func(f *hostedWorkspaceFixture) { f.editor.Config = json.RawMessage(`{"url":"%invalid"}`) }, ErrEditorConfigInvalid},
		{"malformed config", func(f *hostedWorkspaceFixture) { f.editor.Config = json.RawMessage(`{`) }, ErrEditorConfigInvalid},
		{"parent escape", func(f *hostedWorkspaceFixture) { f.input.FilePath = ".." }, ErrEditorConfigInvalid},
		{"normalized escape", func(f *hostedWorkspaceFixture) {
			f.input.FilePath = filepath.FromSlash("src/../../outside.go")
		}, ErrEditorConfigInvalid},
	}
}

// @covers AC-WORKSPACES-HOSTED-EDITOR-001.2, AC-WORKSPACES-HOSTED-EDITOR-001.4
func TestOpenEditor_HostedWorkspaceErrors(t *testing.T) {
	readErr := errors.New("synthetic session read failure")
	cases := append(hostedWorkspaceErrorCases(), hostedWorkspaceErrorCase{
		name: "dependency error", change: func(f *hostedWorkspaceFixture) { f.tasks.readErr = readErr }, want: readErr,
	})
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := newHostedWorkspaceFixture(t)
			test.change(&fixture)
			target, err := fixture.service.OpenEditor(context.Background(), fixture.input)
			if !errors.Is(err, test.want) || target != "" {
				t.Fatalf("OpenEditor = %q, %v; want empty URL, %v", target, err, test.want)
			}
		})
	}
}

// @covers AC-WORKSPACES-HOSTED-EDITOR-001.4
func TestOpenEditor_HostedWorkspaceEmbeddedControl(t *testing.T) {
	fixture := newHostedWorkspaceFixture(t)
	fixture.editor.Kind = editorKindInternalVscode
	for _, file := range []string{"", ".", "src/.."} {
		fixture.input.FilePath = filepath.FromSlash(file)
		target, err := fixture.service.OpenEditor(context.Background(), fixture.input)
		if err != nil || target != "internal://vscode" {
			t.Fatalf("embedded root %q = %q, %v; want internal://vscode, nil", file, target, err)
		}
	}
}
