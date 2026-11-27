package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	editormodels "github.com/kandev/kandev/internal/editors/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type countingEditorTaskRepository struct {
	taskSessionReader
	session     *taskmodels.TaskSession
	executor    *taskmodels.Executor
	sessionRead int
}

func (r *countingEditorTaskRepository) GetTaskSession(context.Context, string) (*taskmodels.TaskSession, error) {
	r.sessionRead++
	return r.session, nil
}

func (r *countingEditorTaskRepository) GetExecutor(context.Context, string) (*taskmodels.Executor, error) {
	return r.executor, nil
}

func newSessionAccessTestService(t *testing.T) (*Service, *countingEditorTaskRepository) {
	t.Helper()
	const editorID = "unavailable-command-editor"
	tasks := &countingEditorTaskRepository{session: &taskmodels.TaskSession{
		ID:        "session-1",
		Worktrees: []*taskmodels.TaskEnvironmentRepo{{WorktreePath: t.TempDir()}},
	}}
	svc := NewService(
		&openEditorRepository{editor: &editormodels.Editor{
			ID: editorID, Kind: editorKindCustomCommand, Enabled: true,
			Config: json.RawMessage(`{"command":"editor-command-that-is-not-installed {file}"}`),
		}},
		tasks,
		&openEditorUserSettings{defaultEditorID: editorID},
	)
	return svc, tasks
}

func TestOpenEditorRejectsRealIdentityBeforeSessionRead(t *testing.T) {
	svc, tasks := newSessionAccessTestService(t)
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "viewer", Role: authn.RoleMember})
	_, err := svc.OpenEditor(ctx, OpenEditorInput{SessionID: "session-1", FilePath: "main.go"})
	if !errors.Is(err, ErrSessionForbidden) {
		t.Fatalf("OpenEditor error = %v, want ErrSessionForbidden", err)
	}
	if tasks.sessionRead != 0 {
		t.Fatalf("session reads = %d, want 0", tasks.sessionRead)
	}
}

func TestOpenFolderRejectsRealIdentityBeforeAvailabilityCheck(t *testing.T) {
	svc, tasks := newSessionAccessTestService(t)
	t.Setenv("PATH", t.TempDir())
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "viewer", Role: authn.RoleMember})
	err := svc.OpenFolder(ctx, "session-1", "")
	if !errors.Is(err, ErrSessionForbidden) {
		t.Fatalf("OpenFolder error = %v, want ErrSessionForbidden", err)
	}
	if tasks.sessionRead != 0 {
		t.Fatalf("session reads = %d, want 0", tasks.sessionRead)
	}
}

func TestSessionAccessCheckerDeniesBeforeReadsOrProcessStarts(t *testing.T) {
	makeFolderOpenerAvailable(t)
	svc, tasks := newSessionAccessTestService(t)
	svc.SetSessionAccessChecker(func(context.Context, string) error { return ErrSessionForbidden })
	starts := recordProcessStarts(t)
	ctx := t.Context()

	if _, err := svc.OpenEditor(ctx, OpenEditorInput{SessionID: "session-1", FilePath: "main.go"}); !errors.Is(err, ErrSessionForbidden) {
		t.Fatalf("OpenEditor error = %v, want ErrSessionForbidden", err)
	}
	if err := svc.OpenFolder(ctx, "session-1", ""); !errors.Is(err, ErrSessionForbidden) {
		t.Fatalf("OpenFolder error = %v, want ErrSessionForbidden", err)
	}
	if tasks.sessionRead != 0 {
		t.Errorf("session reads = %d, want 0", tasks.sessionRead)
	}
	if *starts != 0 {
		t.Errorf("process starts = %d, want 0", *starts)
	}

	const embeddedEditorID = "embedded-vscode-editor"
	tasks.session.ExecutorID = "executor-1"
	tasks.executor = &taskmodels.Executor{ID: "executor-1", Type: taskmodels.ExecutorTypeLocalDocker}
	svc.repo = &openEditorRepository{editor: &editormodels.Editor{
		ID: embeddedEditorID, Kind: editorKindInternalVscode, Enabled: true,
	}}
	svc.userSettings = &openEditorUserSettings{defaultEditorID: embeddedEditorID}
	if _, err := svc.OpenEditor(ctx, OpenEditorInput{SessionID: "session-1", EditorID: embeddedEditorID}); !errors.Is(err, ErrSessionForbidden) {
		t.Errorf("OpenEditor embedded editor error = %v, want ErrSessionForbidden", err)
	}
	if tasks.sessionRead != 0 {
		t.Errorf("embedded editor session reads = %d, want 0", tasks.sessionRead)
	}
	if *starts != 0 {
		t.Errorf("embedded editor process starts = %d, want 0", *starts)
	}
}

func TestSessionAccessCheckerAllowsBothEditorLaunches(t *testing.T) {
	makeFolderOpenerAvailable(t)
	svc, tasks := newSessionAccessTestService(t)
	svc.SetSessionAccessChecker(func(context.Context, string) error { return nil })
	starts := recordProcessStarts(t)

	if _, err := svc.OpenEditor(t.Context(), OpenEditorInput{SessionID: "session-1", FilePath: "main.go"}); err != nil {
		t.Fatalf("OpenEditor error = %v, want nil", err)
	}
	if err := svc.OpenFolder(t.Context(), "session-1", ""); err != nil {
		t.Fatalf("OpenFolder error = %v, want nil", err)
	}
	if tasks.sessionRead != 2 {
		t.Errorf("session reads = %d, want 2", tasks.sessionRead)
	}
	if *starts != 2 {
		t.Errorf("process starts = %d, want 2", *starts)
	}
}

func TestUnwiredSessionAccessAllowsUnscopedCallers(t *testing.T) {
	for _, tc := range []struct {
		name string
		ctx  context.Context
	}{
		{name: "no identity", ctx: context.Background()},
		{name: "synthetic identity", ctx: authn.WithIdentity(context.Background(), authn.Identity{UserID: "local", Synthetic: true})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			makeFolderOpenerAvailable(t)
			svc, tasks := newSessionAccessTestService(t)
			starts := recordProcessStarts(t)
			if _, err := svc.OpenEditor(tc.ctx, OpenEditorInput{SessionID: "session-1", FilePath: "main.go"}); err != nil {
				t.Fatalf("OpenEditor error = %v, want nil", err)
			}
			if err := svc.OpenFolder(tc.ctx, "session-1", ""); err != nil {
				t.Fatalf("OpenFolder error = %v, want nil", err)
			}
			if tasks.sessionRead != 2 || *starts != 2 {
				t.Fatalf("session reads = %d, process starts = %d; want 2 each", tasks.sessionRead, *starts)
			}
		})
	}
}

func makeFolderOpenerAvailable(t *testing.T) {
	t.Helper()
	command := folderOpenCommand(runtime.GOOS)
	if command == "" {
		t.Skip("folder opener is unsupported on this platform")
	}
	dir := t.TempDir()
	name := command
	if runtime.GOOS == "windows" {
		name += ".exe"
		t.Setenv("PATHEXT", ".COM;.EXE;.BAT;.CMD")
	}
	if err := os.WriteFile(filepath.Join(dir, name), nil, 0o700); err != nil {
		t.Fatalf("create folder opener fixture: %v", err)
	}
	t.Setenv("PATH", dir)
}

func recordProcessStarts(t *testing.T) *int {
	t.Helper()
	previous := startProcess
	starts := new(int)
	startProcess = func(*exec.Cmd) error {
		*starts++
		return nil
	}
	t.Cleanup(func() { startProcess = previous })
	return starts
}
