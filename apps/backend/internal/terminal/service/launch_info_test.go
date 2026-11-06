package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/terminal/repository"
)

func newSharedEnvironmentService(t *testing.T) (*Service, *fakeBackend) {
	t.Helper()
	// Unique in-memory DSN per test: `cache=shared` databases are keyed by
	// name process-wide, so a fixed name would leak rows between tests.
	rawDB, err := sql.Open("sqlite3", "file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	rawDB.SetMaxOpenConns(1)
	sqlxDB := sqlx.NewDb(rawDB, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	repo, err := repository.NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("repo: %v", err)
	}
	be := newFakeBackend()
	return New(repo, be, nil), be
}

// Two tasks can share one environment. The owner lookup must return the
// persisted owner regardless of which task's execution serves the transport,
// and must reject a different environment or an unknown row.
func TestOwnerForTerminalUsesPersistedOwnerInSharedEnvironment(t *testing.T) {
	svc, _ := newSharedEnvironmentService(t)
	ctx := context.Background()
	sharedEnv := "env-shared"

	taskA, err := svc.CreateWithOneShotInitialCommand(ctx, "task-a", sharedEnv, "echo a")
	if err != nil {
		t.Fatal(err)
	}
	taskB, err := svc.Create(ctx, "task-b", sharedEnv, "echo b")
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name       string
		terminalID string
		wantOwner  string
	}{
		{name: "task a", terminalID: taskA.ID, wantOwner: "task-a"},
		{name: "task b", terminalID: taskB.ID, wantOwner: "task-b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owner, err := svc.OwnerForTerminal(ctx, sharedEnv, tc.terminalID)
			if err != nil {
				t.Fatalf("OwnerForTerminal: %v", err)
			}
			if owner != tc.wantOwner {
				t.Fatalf("owner = %q, want %q", owner, tc.wantOwner)
			}
		})
	}

	if _, err := svc.OwnerForTerminal(ctx, "env-other", taskA.ID); !errors.Is(err, ErrTaskMismatch) {
		t.Fatalf("wrong environment error = %v, want ErrTaskMismatch", err)
	}
	if _, err := svc.OwnerForTerminal(ctx, sharedEnv, "shell-missing"); err == nil {
		t.Fatal("missing terminal produced an owner")
	}
	if _, err := svc.OwnerForTerminal(ctx, sharedEnv, BottomPanelID); !errors.Is(err, ErrNotManaged) {
		t.Fatalf("fixed terminal error = %v, want ErrNotManaged", err)
	}
	if _, err := svc.OwnerForTerminal(ctx, "", taskA.ID); !errors.Is(err, ErrTaskMismatch) {
		t.Fatalf("empty environment error = %v, want ErrTaskMismatch", err)
	}
}

// The owner lookup is a read: it must not consume a one-shot command, or a
// denied attachment would burn the credential before authorization runs.
func TestOwnerForTerminalLeavesOneShotCommandUnclaimed(t *testing.T) {
	svc, _ := newSharedEnvironmentService(t)
	ctx := context.Background()
	command := "cursor-agent mcp login server"
	term, err := svc.CreateWithOneShotInitialCommand(ctx, "task-a", "env-shared", command)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.OwnerForTerminal(ctx, "env-shared", term.ID); err != nil {
		t.Fatalf("OwnerForTerminal: %v", err)
	}

	info, err := svc.ClaimShellLaunchInfo(ctx, "task-a", "env-shared", term.ID)
	if err != nil {
		t.Fatalf("ClaimShellLaunchInfo: %v", err)
	}
	if info.InitialCommand != command {
		t.Fatalf("initial command = %q, want %q; owner lookup consumed it", info.InitialCommand, command)
	}
}
