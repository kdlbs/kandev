package github

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestTaskPRDetachFiltersActiveRowsAndPersistsTombstone(t *testing.T) {
	store := newTestStore(t)
	detacher, ok := any(store).(interface {
		DetachTaskPR(context.Context, string) (*TaskPR, bool, error)
	})
	if !ok {
		t.Fatal("Store does not implement DetachTaskPR")
	}
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := store.db.Exec(`INSERT INTO tasks (id, workspace_id) VALUES ('task-1', 'ws-1')`); err != nil {
		t.Fatalf("seed task-1: %v", err)
	}
	first := &TaskPR{
		WorkspaceID: "ws-1", TaskID: "task-1", RepositoryID: "repo-1",
		Owner: "acme", Repo: "demo", PRNumber: 1, PRURL: "https://github.com/acme/demo/pull/1",
		PRTitle: "old", HeadBranch: "old", BaseBranch: "main", State: "merged", CreatedAt: now,
	}
	second := &TaskPR{
		WorkspaceID: "ws-1", TaskID: "task-1", RepositoryID: "repo-1",
		Owner: "acme", Repo: "demo", PRNumber: 2, PRURL: "https://github.com/acme/demo/pull/2",
		PRTitle: "new", HeadBranch: "new", BaseBranch: "main", State: "open", CreatedAt: now.Add(time.Second),
	}
	if err := store.CreateTaskPR(ctx, first); err != nil {
		t.Fatalf("create first PR: %v", err)
	}
	if err := store.CreateTaskPR(ctx, second); err != nil {
		t.Fatalf("create second PR: %v", err)
	}

	detached, transitioned, err := detacher.DetachTaskPR(ctx, first.ID)
	if err != nil {
		t.Fatalf("detach first PR: %v", err)
	}
	if !transitioned {
		t.Fatal("first detach did not report a transition")
	}
	detachedAt := reflect.Value{}
	if detached != nil {
		detachedAt = reflect.ValueOf(detached).Elem().FieldByName("DetachedAt")
	}
	if detached == nil || detached.ID != first.ID || !detachedAt.IsValid() || detachedAt.IsNil() {
		t.Fatalf("detached row = %+v, want stamped row", detached)
	}

	active, err := store.ListTaskPRsByTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("list active PRs: %v", err)
	}
	if len(active) != 1 || active[0].ID != second.ID {
		t.Fatalf("active PRs = %+v, want only second PR", active)
	}
	_, transitioned, err = detacher.DetachTaskPR(ctx, first.ID)
	if err != nil {
		t.Fatalf("repeat detach first PR: %v", err)
	}
	if transitioned {
		t.Fatal("repeat detach reported a transition")
	}

	reopened, err := NewStore(store.db, store.ro)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	active, err = reopened.ListTaskPRsByTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("list active PRs after reopen: %v", err)
	}
	if len(active) != 1 || active[0].ID != second.ID {
		t.Fatalf("active PRs after reopen = %+v, want only second PR", active)
	}
}

func TestLegacyTaskPRRebuildPreservesDetachedAt(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO workspaces (id) VALUES ('ws-legacy');
		INSERT INTO tasks (id, workspace_id) VALUES ('task-legacy', 'ws-legacy');
		DROP TABLE github_task_prs;
		CREATE TABLE github_task_prs (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			task_id TEXT NOT NULL,
			owner TEXT NOT NULL,
			repo TEXT NOT NULL,
			pr_number INTEGER NOT NULL,
			pr_url TEXT NOT NULL,
			pr_title TEXT NOT NULL,
			head_branch TEXT NOT NULL,
			base_branch TEXT NOT NULL,
			author_login TEXT NOT NULL,
			state TEXT NOT NULL DEFAULT 'open',
			review_state TEXT NOT NULL DEFAULT '',
			checks_state TEXT NOT NULL DEFAULT '',
			mergeable_state TEXT NOT NULL DEFAULT '',
			review_count INTEGER DEFAULT 0,
			pending_review_count INTEGER DEFAULT 0,
			required_reviews INTEGER,
			comment_count INTEGER DEFAULT 0,
			unresolved_review_threads INTEGER DEFAULT 0,
			checks_total INTEGER DEFAULT 0,
			checks_passing INTEGER DEFAULT 0,
			additions INTEGER DEFAULT 0,
			deletions INTEGER DEFAULT 0,
			created_at DATETIME NOT NULL,
			merged_at DATETIME,
			closed_at DATETIME,
			last_synced_at DATETIME,
			detached_at DATETIME,
			updated_at DATETIME NOT NULL,
			UNIQUE(task_id, pr_number)
		);
		INSERT INTO github_task_prs (
			id, workspace_id, task_id, owner, repo, pr_number, pr_url, pr_title,
			head_branch, base_branch, author_login, state, created_at, detached_at, updated_at
		) VALUES (
			'legacy-detached', 'ws-legacy', 'task-legacy', 'acme', 'demo', 42,
			'https://github.com/acme/demo/pull/42', 'Legacy detached PR', 'feat/legacy',
			'main', 'octocat', 'merged', ?, ?, ?
		)
	`, now, now, now); err != nil {
		t.Fatalf("seed legacy task PR schema: %v", err)
	}

	reopened, err := NewStore(store.db, store.ro)
	if err != nil {
		t.Fatalf("reopen store with legacy task PR schema: %v", err)
	}
	got, err := reopened.GetTaskPRByID(ctx, "legacy-detached")
	if err != nil {
		t.Fatalf("get migrated detached PR: %v", err)
	}
	if got == nil || got.DetachedAt == nil || !got.DetachedAt.Equal(now) {
		t.Fatalf("migrated detached PR = %+v, want detached_at %s", got, now)
	}
	active, err := reopened.ListTaskPRsByTask(ctx, "task-legacy")
	if err != nil {
		t.Fatalf("list migrated active PRs: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("migrated active PRs = %+v, want detached row filtered", active)
	}
}

// TestLegacyTaskPRRebuildPreservesOutcomeColumns is the regression for
// AC-07a: the legacy-constraint rebuild's CREATE TABLE and
// INSERT...SELECT copy list must both carry all five outcome-attribution
// columns, or a real user database silently loses merged_by_login,
// closed_by_login, is_draft, changed_files, and auto_merge_observed_at the
// moment the rebuild fires. TestLegacyTaskPRRebuildPreservesDetachedAt above
// never populates the outcome columns on its seeded row, so it cannot catch
// a rebuild that drops them from the copy list: without a seeded legacy
// constraint AND non-NULL outcome values, the rebuild either no-ops or
// degenerately "succeeds" while silently losing data. This test seeds all
// five with non-NULL values and reopens the store twice
// (task_external_id_migration_test.go's replay precedent) to also prove
// idempotence once the legacy constraint is gone.
func TestLegacyTaskPRRebuildPreservesOutcomeColumns(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	mergedAt := now.Add(-time.Hour)
	autoMergeAt := now.Add(-2 * time.Hour)
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO workspaces (id) VALUES ('ws-legacy-outcome');
		INSERT INTO tasks (id, workspace_id) VALUES ('task-legacy-outcome', 'ws-legacy-outcome');
		DROP TABLE github_task_prs;
		CREATE TABLE github_task_prs (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			task_id TEXT NOT NULL,
			owner TEXT NOT NULL,
			repo TEXT NOT NULL,
			pr_number INTEGER NOT NULL,
			pr_url TEXT NOT NULL,
			pr_title TEXT NOT NULL,
			head_branch TEXT NOT NULL,
			base_branch TEXT NOT NULL,
			author_login TEXT NOT NULL,
			state TEXT NOT NULL DEFAULT 'open',
			review_state TEXT NOT NULL DEFAULT '',
			checks_state TEXT NOT NULL DEFAULT '',
			mergeable_state TEXT NOT NULL DEFAULT '',
			review_count INTEGER DEFAULT 0,
			pending_review_count INTEGER DEFAULT 0,
			required_reviews INTEGER,
			comment_count INTEGER DEFAULT 0,
			unresolved_review_threads INTEGER DEFAULT 0,
			checks_total INTEGER DEFAULT 0,
			checks_passing INTEGER DEFAULT 0,
			additions INTEGER DEFAULT 0,
			deletions INTEGER DEFAULT 0,
			created_at DATETIME NOT NULL,
			merged_at DATETIME,
			closed_at DATETIME,
			last_synced_at DATETIME,
			detached_at DATETIME,
			updated_at DATETIME NOT NULL,
			is_draft BOOLEAN,
			changed_files INTEGER,
			merged_by_login TEXT,
			closed_by_login TEXT,
			auto_merge_observed_at DATETIME,
			UNIQUE(task_id, pr_number)
		);
		INSERT INTO github_task_prs (
			id, workspace_id, task_id, owner, repo, pr_number, pr_url, pr_title,
			head_branch, base_branch, author_login, state, created_at, merged_at, updated_at,
			is_draft, changed_files, merged_by_login, closed_by_login, auto_merge_observed_at
		) VALUES (
			'legacy-outcome', 'ws-legacy-outcome', 'task-legacy-outcome', 'acme', 'demo', 77,
			'https://github.com/acme/demo/pull/77', 'Legacy outcome PR', 'feat/legacy-outcome',
			'main', 'octocat', 'merged', ?, ?, ?,
			0, 12, 'carlosflorencio', 'octocat', ?
		)
	`, now, mergedAt, now, autoMergeAt); err != nil {
		t.Fatalf("seed legacy task PR schema with outcome columns: %v", err)
	}

	assertOutcomeColumnsSurvived := func(t *testing.T, s *Store) {
		t.Helper()
		got, err := s.GetTaskPRByID(ctx, "legacy-outcome")
		if err != nil {
			t.Fatalf("get migrated outcome PR: %v", err)
		}
		if got == nil {
			t.Fatal("migrated outcome PR not found")
		}
		if got.IsDraft == nil || *got.IsDraft != false {
			t.Errorf("is_draft = %v, want false (observed, not NULL)", got.IsDraft)
		}
		if got.ChangedFiles == nil || *got.ChangedFiles != 12 {
			t.Errorf("changed_files = %v, want 12", got.ChangedFiles)
		}
		if got.MergedByLogin == nil || *got.MergedByLogin != "carlosflorencio" {
			t.Errorf("merged_by_login = %v, want carlosflorencio", got.MergedByLogin)
		}
		if got.ClosedByLogin == nil || *got.ClosedByLogin != "octocat" {
			t.Errorf("closed_by_login = %v, want octocat", got.ClosedByLogin)
		}
		if got.AutoMergeObservedAt == nil || !got.AutoMergeObservedAt.Equal(autoMergeAt) {
			t.Errorf("auto_merge_observed_at = %v, want %s", got.AutoMergeObservedAt, autoMergeAt)
		}
	}

	reopened, err := NewStore(store.db, store.ro)
	if err != nil {
		t.Fatalf("reopen store with legacy outcome schema: %v", err)
	}
	assertOutcomeColumnsSurvived(t, reopened)

	// Reopen a second time: the legacy UNIQUE(task_id, pr_number) constraint
	// is gone after the first rebuild, so this run must be a no-op that
	// leaves the already-migrated values untouched (idempotence).
	reopenedAgain, err := NewStore(reopened.db, reopened.ro)
	if err != nil {
		t.Fatalf("reopen store a second time: %v", err)
	}
	assertOutcomeColumnsSurvived(t, reopenedAgain)
}
