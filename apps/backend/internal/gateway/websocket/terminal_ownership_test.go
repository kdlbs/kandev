package websocket

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	terminalrepo "github.com/kandev/kandev/internal/terminal/repository"
	terminalservice "github.com/kandev/kandev/internal/terminal/service"
)

// newSharedEnvironmentTerminalService builds a real terminal service over an
// isolated SQLite file so ownership regressions exercise the persisted row,
// not a stub.
func newSharedEnvironmentTerminalService(t *testing.T) (*terminalservice.Service, *lifecycle.Manager) {
	t.Helper()
	log := testTerminalLogger(t)
	manager := lifecycle.NewManager(nil, bus.NewMemoryEventBus(log), nil, nil, nil, nil, lifecycle.ExecutorFallbackDeny, t.TempDir(), log)
	rawDB, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "terminals.db"))
	if err != nil {
		t.Fatal(err)
	}
	rawDB.SetMaxOpenConns(1)
	db := sqlx.NewDb(rawDB, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := terminalrepo.NewWithDB(db, db, log)
	if err != nil {
		t.Fatal(err)
	}
	return terminalservice.New(repo, nil, log), manager
}

func sharedEnvironmentContext(t *testing.T, environmentID, terminalID string) *gin.Context {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/terminal/environment/"+environmentID+"?terminalId="+terminalID,
		nil,
	)
	return c
}

// The execution that serves a shared environment may belong to a sibling
// task. Ownership must come from the persisted terminal row, so both task A
// and task B connect through either task's execution.
func TestOrdinaryTerminalLaunchUsesPersistedOwnerInSharedEnvironment(t *testing.T) {
	ctx := context.Background()
	terminals, manager := newSharedEnvironmentTerminalService(t)
	const sharedEnv = "env-shared"
	termA, err := terminals.Create(ctx, "task-a", sharedEnv, "echo a")
	if err != nil {
		t.Fatal(err)
	}
	labelA := "Task A login"
	if err := terminals.Rename(ctx, "task-a", termA.ID, &labelA); err != nil {
		t.Fatal(err)
	}
	termB, err := terminals.Create(ctx, "task-b", sharedEnv, "echo b")
	if err != nil {
		t.Fatal(err)
	}

	handler := NewTerminalHandler(manager, nil, nil, testTerminalLogger(t))
	handler.SetTerminalService(terminals)

	for _, tc := range []struct {
		name       string
		execTaskID string
		terminalID string
		wantLabel  string
		wantCmd    string
	}{
		{name: "task A execution and task A terminal", execTaskID: "task-a", terminalID: termA.ID, wantLabel: labelA, wantCmd: "echo a"},
		{name: "task B execution and task B terminal", execTaskID: "task-b", terminalID: termB.ID, wantLabel: "Terminal 1", wantCmd: "echo b"},
		{name: "task B execution and task A terminal", execTaskID: "task-b", terminalID: termA.ID, wantLabel: labelA, wantCmd: "echo a"},
		{name: "task A execution and task B terminal", execTaskID: "task-a", terminalID: termB.ID, wantLabel: "Terminal 1", wantCmd: "echo b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			execution := &lifecycle.AgentExecution{
				ID: "exec-" + tc.execTaskID, TaskID: tc.execTaskID, SessionID: "session-" + tc.execTaskID,
				TaskEnvironmentID: sharedEnv, WorkspacePath: t.TempDir(),
			}
			c := sharedEnvironmentContext(t, sharedEnv, tc.terminalID)
			label, command, once, httpErr := handler.resolveOrdinaryTerminalLaunch(
				c, execution, sharedEnv, tc.terminalID, "untrusted", "",
			)
			if httpErr != "" {
				t.Fatalf("resolveOrdinaryTerminalLaunch error = %q", httpErr)
			}
			if label != tc.wantLabel || command != tc.wantCmd || once {
				t.Fatalf("launch = (%q, %q, %v), want (%q, %q, false)", label, command, once, tc.wantLabel, tc.wantCmd)
			}
		})
	}
}

// Authorization must run on the resolved owner, not the transport execution's
// task. A denied pair exposes no launch metadata and leaves the one-shot
// command unclaimed for a later authorized attempt.
func TestOrdinaryTerminalLaunchDeniesBeforeClaimingSharedEnvironmentCommand(t *testing.T) {
	ctx := context.Background()
	terminals, manager := newSharedEnvironmentTerminalService(t)
	const sharedEnv = "env-shared"
	command := "cursor-agent mcp login server"
	term, err := terminals.CreateWithOneShotInitialCommand(ctx, "task-a", sharedEnv, command)
	if err != nil {
		t.Fatal(err)
	}
	deny := true
	manager.SetTaskEnvironmentAccessChecker(func(_ context.Context, taskID, environmentID string) error {
		if deny && taskID == "task-a" && environmentID == sharedEnv {
			return errors.New("task not found")
		}
		return nil
	})

	handler := NewTerminalHandler(manager, nil, nil, testTerminalLogger(t))
	handler.SetTerminalService(terminals)
	execution := &lifecycle.AgentExecution{
		ID: "exec-b", TaskID: "task-b", SessionID: "session-b",
		TaskEnvironmentID: sharedEnv, WorkspacePath: t.TempDir(),
	}

	c := sharedEnvironmentContext(t, sharedEnv, term.ID)
	label, initialCommand, once, httpErr := handler.resolveOrdinaryTerminalLaunch(
		c, execution, sharedEnv, term.ID, "untrusted", "",
	)
	if httpErr != "terminal not found" {
		t.Fatalf("denied launch error = %q, want terminal not found", httpErr)
	}
	if label != "" || initialCommand != "" || once {
		t.Fatalf("denied launch leaked metadata: (%q, %q, %v)", label, initialCommand, once)
	}

	// The persisted command is still available: denial did not consume it.
	info, err := terminals.ShellLaunchInfo(ctx, "task-a", sharedEnv, term.ID)
	if err != nil {
		t.Fatal(err)
	}
	if info.InitialCommand != command {
		t.Fatalf("persisted command = %q, want %q after denial", info.InitialCommand, command)
	}

	deny = false
	label, initialCommand, once, httpErr = handler.resolveOrdinaryTerminalLaunch(
		sharedEnvironmentContext(t, sharedEnv, term.ID), execution, sharedEnv, term.ID, "untrusted", "",
	)
	if httpErr != "" {
		t.Fatalf("authorized launch error = %q", httpErr)
	}
	if initialCommand != command || !once {
		t.Fatalf("authorized launch = (%q, %v), want (%q, true)", initialCommand, once, command)
	}
	if label == "untrusted" {
		t.Fatalf("launch used the client label %q", label)
	}
}

// Missing metadata, a foreign environment, and an unknown ID must all fail
// closed with the same non-disclosing error.
func TestOrdinaryTerminalLaunchRejectsForeignSharedEnvironmentMetadata(t *testing.T) {
	ctx := context.Background()
	terminals, manager := newSharedEnvironmentTerminalService(t)
	term, err := terminals.Create(ctx, "task-a", "env-shared", "echo a")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewTerminalHandler(manager, nil, nil, testTerminalLogger(t))
	handler.SetTerminalService(terminals)
	execution := &lifecycle.AgentExecution{
		ID: "exec-a", TaskID: "task-a", SessionID: "session-a",
		TaskEnvironmentID: "env-shared", WorkspacePath: t.TempDir(),
	}

	for _, tc := range []struct {
		name          string
		environmentID string
		terminalID    string
	}{
		{name: "foreign environment", environmentID: "env-other", terminalID: term.ID},
		{name: "unknown terminal", environmentID: "env-shared", terminalID: "shell-missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := sharedEnvironmentContext(t, tc.environmentID, tc.terminalID)
			label, command, once, httpErr := handler.resolveOrdinaryTerminalLaunch(
				c, execution, tc.environmentID, tc.terminalID, "untrusted", "",
			)
			if httpErr != "terminal not found" {
				t.Fatalf("error = %q, want terminal not found", httpErr)
			}
			if label != "" || command != "" || once {
				t.Fatalf("rejected launch leaked metadata: (%q, %q, %v)", label, command, once)
			}
		})
	}
}

// An environment removed after the owner lookup keeps the missing-target
// result so the browser stops reconnecting; other owner denials stay a
// non-disclosing terminal mismatch.
func TestEnvironmentTerminalOwnerKeepsMissingEnvironment(t *testing.T) {
	ctx := context.Background()
	terminals, manager := newSharedEnvironmentTerminalService(t)
	const sharedEnv = "env-shared"
	term, err := terminals.Create(ctx, "task-a", sharedEnv, "")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewTerminalHandler(manager, nil, nil, testTerminalLogger(t))
	handler.SetTerminalService(terminals)

	accessErr := fmt.Errorf("authorize: %w", repoerrors.ErrTaskEnvironmentNotFound)
	manager.SetTaskEnvironmentAccessChecker(func(context.Context, string, string) error { return accessErr })
	if _, err := handler.resolveEnvironmentTerminalExecution(ctx, sharedEnv, term.ID); !errors.Is(err, repoerrors.ErrTaskEnvironmentNotFound) {
		t.Fatalf("removed environment error = %v, want ErrTaskEnvironmentNotFound", err)
	}

	accessErr = repoerrors.ErrTaskNotFound
	if _, err := handler.resolveEnvironmentTerminalExecution(ctx, sharedEnv, term.ID); !errors.Is(err, terminalservice.ErrTaskMismatch) {
		t.Fatalf("denied owner error = %v, want ErrTaskMismatch", err)
	}
}
