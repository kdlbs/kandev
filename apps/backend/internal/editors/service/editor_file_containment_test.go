package service

import (
	"bytes"
	"context"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	editormodels "github.com/kandev/kandev/internal/editors/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type editorContainmentCase struct {
	name      string
	input     string
	expected  string
	err       error
	directory bool
}

func editorContainmentCases() []editorContainmentCase {
	separator := string(filepath.Separator)
	cases := []editorContainmentCase{
		{name: "ordinary file", input: "src/plain.go", expected: "src/plain.go"},
		{name: "two-dot file", input: "..notes.go", expected: "..notes.go"},
		{name: "two-dot directory", input: "..notes/inside.go", expected: "..notes/inside.go"},
		{name: "two-dot folder target", input: "..notes", expected: "..notes", directory: true},
		{name: "one-dot file", input: ".hidden.go", expected: ".hidden.go"},
		{name: "three-dot file", input: "...notes.go", expected: "...notes.go"},
		{name: "nested two-dot file", input: "src/..notes.go", expected: "src/..notes.go"},
		{name: "current directory prefix", input: "./..notes.go", expected: "..notes.go"},
		{name: "contained parent normalization", input: "src/../..notes.go", expected: "..notes.go"},
		{name: "folder empty", input: ""},
		{name: "folder dot", input: "."},
		{name: "normalized folder", input: "src/.."},
		{name: "parent itself", input: "..", err: ErrEditorConfigInvalid},
		{name: "parent file", input: "../outside.go", err: ErrEditorConfigInvalid},
		{name: "nested parent escape", input: "src/../../outside.go", err: ErrEditorConfigInvalid},
		{name: "dot-directory parent escape", input: "..notes/../../outside.go", err: ErrEditorConfigInvalid},
	}
	for index := range cases {
		cases[index].input = filepath.FromSlash(cases[index].input)
		cases[index].expected = filepath.FromSlash(cases[index].expected)
	}
	cases = append(cases, editorContainmentCase{
		name: "repeated native separators", input: "src" + separator + separator + "plain.go",
		expected: filepath.FromSlash("src/plain.go"),
	})
	if runtime.GOOS == "windows" {
		cases = append(cases,
			editorContainmentCase{name: "Windows forward slash contained", input: "src/../..notes.go", expected: "..notes.go"},
			editorContainmentCase{name: "Windows forward slash escape", input: "../outside.go", err: ErrEditorConfigInvalid},
		)
	} else {
		cases = append(cases, editorContainmentCase{
			name: "POSIX backslash filename", input: `..\notes.go`, expected: `..\notes.go`,
		})
	}
	return cases
}

// @covers AC-WORKSPACES-EDITOR-CONTAINMENT-001.1, AC-WORKSPACES-EDITOR-CONTAINMENT-001.2,
// AC-WORKSPACES-EDITOR-CONTAINMENT-001.3, AC-WORKSPACES-EDITOR-CONTAINMENT-001.4
func TestOpenEditor_ContainedDotPaths(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "worktree")
	cases := editorContainmentCases()
	files := make(map[string][]byte)
	for _, test := range cases {
		if test.expected != "" && !test.directory {
			files[filepath.Join(root, test.expected)] = []byte("contained: " + test.expected + "\n\x00\xff")
		}
	}
	files[filepath.Join(parent, "outside.go")] = []byte("outside sentinel\n\x00\xfe")
	for filename, content := range files {
		if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	const editorID = "containment-embedded-editor"
	svc := NewService(
		&openEditorRepository{editor: &editormodels.Editor{ID: editorID, Kind: editorKindInternalVscode, Enabled: true}},
		&openEditorTaskRepository{
			session: &taskmodels.TaskSession{ID: "containment-session", ExecutorID: "containment-executor",
				Worktrees: []*taskmodels.TaskEnvironmentRepo{{WorktreeID: "containment-worktree", WorktreePath: root}}},
			executor: &taskmodels.Executor{ID: "containment-executor", Type: taskmodels.ExecutorTypeLocalDocker},
		},
		&openEditorUserSettings{defaultEditorID: editorID},
	)
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Cleanup(func() { assertEditorFixtureBytes(t, files) })
			target, err := svc.OpenEditor(context.Background(), OpenEditorInput{
				SessionID: "containment-session", EditorID: editorID, WorktreeID: "containment-worktree",
				FilePath: test.input, Line: 7, Column: 3,
			})
			if err != test.err {
				t.Fatalf("OpenEditor(%q) error = %v, want %v", test.input, err, test.err)
			}
			if test.err != nil {
				if target != "" {
					t.Fatalf("rejected target = %q, want empty", target)
				}
				return
			}
			assertContainedEditorTarget(t, target, test.expected)
			resolved, err := svc.resolveFilePath(root, test.input)
			if expected := filepath.Join(root, test.expected); err != nil || resolved != expected {
				t.Fatalf("resolved target = %q, %v; want %q, nil", resolved, err, expected)
			}
			if test.directory {
				info, err := os.Stat(resolved)
				if err != nil || !info.IsDir() {
					t.Fatalf("resolved directory %q: %v, want existing directory", resolved, err)
				}
			} else if test.expected != "" {
				assertEditorFixtureBytes(t, map[string][]byte{resolved: files[resolved]})
			}
		})
	}
}

func assertEditorFixtureBytes(t *testing.T, files map[string][]byte) {
	t.Helper()
	for filename, expected := range files {
		actual, err := os.ReadFile(filename)
		if err != nil {
			t.Errorf("read fixture %q: %v", filename, err)
			continue
		}
		if !bytes.Equal(actual, expected) {
			t.Errorf("fixture %q bytes = %q, want %q", filename, actual, expected)
		}
	}
}

func assertContainedEditorTarget(t *testing.T, target, relative string) {
	t.Helper()
	if relative == "" {
		if target != "internal://vscode" {
			t.Fatalf("folder target = %q, want internal://vscode", target)
		}
		return
	}
	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatalf("parse target %q: %v", target, err)
	}
	if parsed.Scheme != "internal" || parsed.Host != "vscode" || parsed.Path != "" || parsed.Fragment != "" {
		t.Fatalf("unexpected embedded target: %q", target)
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		t.Fatalf("parse target query %q: %v", parsed.RawQuery, err)
	}
	if len(query) == 1 {
		if values := query["goto"]; len(values) != 1 || values[0] != relative+":7:3" {
			t.Fatalf("legacy target query = %v, want exact goto %q", query, relative+":7:3")
		}
		return
	}
	if len(query) != 3 || len(query["goto"]) != 1 || len(query["line"]) != 1 || len(query["column"]) != 1 {
		t.Fatalf("unexpected structured target query: %v", query)
	}
	if query.Get("goto") != relative || query.Get("line") != "7" || query.Get("column") != "3" {
		t.Fatalf("structured target query = %v, want file %q, line 7, column 3", query, relative)
	}
}

// @covers AC-WORKSPACES-EDITOR-CONTAINMENT-001.1, AC-WORKSPACES-EDITOR-CONTAINMENT-001.2,
// AC-WORKSPACES-EDITOR-CONTAINMENT-001.3, AC-WORKSPACES-EDITOR-CONTAINMENT-001.4
func TestResolveFilePath_ContainmentControls(t *testing.T) {
	root := filepath.Join(t.TempDir(), "worktree")
	svc := &Service{}
	for _, test := range editorContainmentCases() {
		t.Run(test.name, func(t *testing.T) {
			actual, err := svc.resolveFilePath(root, test.input)
			if err != test.err {
				t.Fatalf("resolveFilePath(%q) error = %v, want %v", test.input, err, test.err)
			}
			expected := ""
			if test.err == nil {
				expected = filepath.Join(root, test.expected)
			}
			if actual != expected {
				t.Fatalf("resolveFilePath(%q) = %q, want %q", test.input, actual, expected)
			}
		})
	}
	t.Run("empty root", func(t *testing.T) {
		for _, input := range []string{"", "..notes.go"} {
			actual, err := svc.resolveFilePath("", input)
			if err != ErrWorkspaceNotFound || actual != "" {
				t.Fatalf("empty-root target for %q = %q, %v; want empty, ErrWorkspaceNotFound", input, actual, err)
			}
		}
	})
}
