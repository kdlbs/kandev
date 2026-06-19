package github

import (
	"context"
	"testing"
	"time"
)

func TestService_GetTaskPRByOwnerRepoNumber(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	t.Run("returns exact owner repo number match", func(t *testing.T) {
		store := newTestStore(t)
		svc := NewService(nil, "pat", nil, store, nil, testLogger(t))
		want := &TaskPR{
			TaskID:       "task-1",
			RepositoryID: "repo-backend",
			Owner:        "kdlbs",
			Repo:         "kandev",
			PRNumber:     1446,
			PRURL:        "https://github.com/kdlbs/kandev/pull/1446",
			PRTitle:      "CI automation",
			HeadBranch:   "feature/ci",
			BaseBranch:   "main",
			State:        "open",
			CreatedAt:    now,
		}
		if err := store.CreateTaskPR(ctx, want); err != nil {
			t.Fatalf("create matching task PR: %v", err)
		}
		if err := store.CreateTaskPR(ctx, &TaskPR{
			TaskID:       "task-1",
			RepositoryID: "repo-other",
			Owner:        "kdlbs",
			Repo:         "kandev",
			PRNumber:     1447,
			CreatedAt:    now,
		}); err != nil {
			t.Fatalf("create non-matching task PR: %v", err)
		}

		got, err := svc.GetTaskPRByOwnerRepoNumber(ctx, "task-1", "kdlbs", "kandev", 1446)
		if err != nil {
			t.Fatalf("get task PR by owner/repo/number: %v", err)
		}
		if got == nil || got.RepositoryID != want.RepositoryID || got.PRNumber != want.PRNumber {
			t.Fatalf("expected exact matching PR, got %+v", got)
		}
	})

	t.Run("returns nil when no PR matches", func(t *testing.T) {
		store := newTestStore(t)
		svc := NewService(nil, "pat", nil, store, nil, testLogger(t))
		if err := store.CreateTaskPR(ctx, &TaskPR{
			TaskID:       "task-1",
			RepositoryID: "repo-backend",
			Owner:        "kdlbs",
			Repo:         "kandev",
			PRNumber:     1446,
			CreatedAt:    now,
		}); err != nil {
			t.Fatalf("create task PR: %v", err)
		}

		got, err := svc.GetTaskPRByOwnerRepoNumber(ctx, "task-1", "kdlbs", "kandev", 9999)
		if err != nil {
			t.Fatalf("get task PR by owner/repo/number: %v", err)
		}
		if got != nil {
			t.Fatalf("expected no match, got %+v", got)
		}
	})

	t.Run("propagates store list errors", func(t *testing.T) {
		store := newTestStore(t)
		svc := NewService(nil, "pat", nil, store, nil, testLogger(t))
		if err := store.db.Close(); err != nil {
			t.Fatalf("close store db: %v", err)
		}

		got, err := svc.GetTaskPRByOwnerRepoNumber(ctx, "task-1", "kdlbs", "kandev", 1446)
		if err == nil {
			t.Fatalf("expected store error, got nil")
		}
		if got != nil {
			t.Fatalf("expected nil PR on error, got %+v", got)
		}
	})
}
