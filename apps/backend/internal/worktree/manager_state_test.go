package worktree

import (
	"context"
	"errors"
	"os"
	"testing"

	commonlogger "github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type createWorktreeErrorStore struct {
	*mockStore
	err error
}

func (s *createWorktreeErrorStore) CreateWorktree(context.Context, *Worktree) error {
	return s.err
}

func newObservedWorktreeLogger(t *testing.T) (*commonlogger.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := commonlogger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observer logger: %v", err)
	}
	return log, logs
}

func TestPersistWorktree_EnvironmentNotResolved(t *testing.T) {
	log, logs := newObservedWorktreeLogger(t)
	store := &createWorktreeErrorStore{
		mockStore: newMockStore(),
		err:       ErrEnvironmentNotResolved,
	}
	mgr := &Manager{store: store, logger: log}
	worktreePath := t.TempDir()

	err := mgr.persistWorktree(context.Background(), &Worktree{ID: "wt-1"}, CreateRequest{
		TaskID:    "task-1",
		SessionID: "session-1",
	}, worktreePath)
	if err != nil {
		t.Fatalf("persistWorktree returned error: %v", err)
	}
	if _, err := os.Stat(worktreePath); err != nil {
		t.Fatalf("expected physical worktree path to remain: %v", err)
	}

	entries := logs.FilterMessage("skipping worktree persistence: session has no task environment yet").All()
	if len(entries) != 1 {
		t.Fatalf("observed %d deferred-persistence entries, want 1", len(entries))
	}
	if entries[0].Level != zap.DebugLevel {
		t.Fatalf("log level = %s, want debug", entries[0].Level)
	}
	warningCount := 0
	for _, entry := range logs.All() {
		if entry.Level == zap.WarnLevel {
			warningCount++
		}
	}
	if warningCount != 0 {
		t.Fatalf("observed %d warning entries, want 0", warningCount)
	}
}

func TestPersistWorktree_OtherStoreError(t *testing.T) {
	storeErr := errors.New("store unavailable")
	log, _ := newObservedWorktreeLogger(t)
	store := &createWorktreeErrorStore{
		mockStore: newMockStore(),
		err:       storeErr,
	}
	mgr := &Manager{store: store, logger: log}

	err := mgr.persistWorktree(context.Background(), &Worktree{ID: "wt-2"}, CreateRequest{
		TaskID:         "task-2",
		SessionID:      "session-2",
		RepositoryPath: t.TempDir(),
	}, t.TempDir())
	if !errors.Is(err, storeErr) {
		t.Fatalf("error = %v, want wrapped store error", err)
	}
}
