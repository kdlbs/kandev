package github

import (
	"context"
	"testing"
	"time"
)

func TestTaskPRSourceFreshSchemaRoundTrips(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	for _, source := range []string{TaskPRSourceWatch, TaskPRSourceURLLink} {
		t.Run(source, func(t *testing.T) {
			id := "source-" + source
			if err := store.CreateTaskPR(ctx, &TaskPR{
				ID: id, WorkspaceID: "ws-source", TaskID: id, RepositoryID: id,
				Owner: "acme", Repo: "demo", PRNumber: 42,
				PRURL: "https://github.com/acme/demo/pull/42", PRTitle: "Source",
				HeadBranch: "feature/source", BaseBranch: "main", AuthorLogin: "alice",
				State: "open", CreatedAt: now, Source: source,
			}); err != nil {
				t.Fatalf("create task PR: %v", err)
			}
			got, err := store.GetTaskPRByID(ctx, id)
			if err != nil {
				t.Fatalf("get task PR: %v", err)
			}
			if got == nil || got.Source != source {
				t.Fatalf("stored source = %q, want %q; row = %+v", sourceOfTaskPR(got), source, got)
			}
		})
	}
}

func TestTaskPRSourceLegacyConstraintRebuildPreservesValuesAndReplay(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO workspaces (id) VALUES ('ws-source');
		INSERT INTO tasks (id, workspace_id) VALUES
			('task-legacy-watch', 'ws-source'),
			('task-legacy-empty', 'ws-source')`); err != nil {
		t.Fatalf("seed legacy tasks: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
		DROP TABLE github_task_prs;
		CREATE TABLE github_task_prs (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			task_id TEXT NOT NULL,
			repository_id TEXT NOT NULL DEFAULT '',
			owner TEXT NOT NULL,
			repo TEXT NOT NULL,
			pr_number INTEGER NOT NULL,
			pr_url TEXT NOT NULL,
			pr_title TEXT NOT NULL,
			head_branch TEXT NOT NULL,
			base_branch TEXT NOT NULL,
			head_sha TEXT NOT NULL DEFAULT '',
			author_login TEXT NOT NULL,
			state TEXT NOT NULL DEFAULT 'open',
			review_state TEXT NOT NULL DEFAULT '',
			checks_state TEXT NOT NULL DEFAULT '',
			mergeable_state TEXT NOT NULL DEFAULT '',
			merge_queue_state TEXT NOT NULL DEFAULT '',
			merge_queue_position INTEGER,
			merge_queue_entry_id TEXT NOT NULL DEFAULT '',
			merge_queue_entry_head_sha TEXT NOT NULL DEFAULT '',
			merge_queue_estimated_time_to_merge_seconds INTEGER,
			merge_queue_last_removal_id TEXT NOT NULL DEFAULT '',
			merge_queue_last_removed_at DATETIME,
			merge_queue_last_removal_reason TEXT NOT NULL DEFAULT '',
			merge_queue_last_removal_before_sha TEXT NOT NULL DEFAULT '',
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
			source TEXT NOT NULL DEFAULT '',
			UNIQUE(task_id, pr_number)
		);
		INSERT INTO github_task_prs (
			id, workspace_id, task_id, repository_id, owner, repo, pr_number, pr_url,
			pr_title, head_branch, base_branch, author_login, state, created_at, updated_at, source
		) VALUES
			('legacy-watch', 'ws-source', 'task-legacy-watch', 'repo-watch', 'acme', 'demo', 1,
			 'https://github.com/acme/demo/pull/1', 'Watch', 'feature/watch', 'main', 'alice', 'open', ?, ?, 'watch'),
			('legacy-empty', 'ws-source', 'task-legacy-empty', 'repo-empty', 'acme', 'demo', 2,
			 'https://github.com/acme/demo/pull/2', 'Legacy', 'feature/legacy', 'main', 'alice', 'open', ?, ?, '')
	`, now, now, now, now); err != nil {
		t.Fatalf("seed legacy task PR table: %v", err)
	}

	reopened, err := NewStore(store.db, store.ro)
	if err != nil {
		t.Fatalf("reopen store for source migration: %v", err)
	}
	assertTaskPRSource := func(t *testing.T, id, want string) {
		t.Helper()
		got, err := reopened.GetTaskPRByID(ctx, id)
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		if got == nil || got.Source != want {
			t.Fatalf("%s source = %q, want %q; row = %+v", id, sourceOfTaskPR(got), want, got)
		}
	}
	assertTaskPRSource(t, "legacy-watch", TaskPRSourceWatch)
	assertTaskPRSource(t, "legacy-empty", "")

	replayed, err := NewStore(reopened.db, reopened.ro)
	if err != nil {
		t.Fatalf("replay source migration: %v", err)
	}
	for id, want := range map[string]string{"legacy-watch": TaskPRSourceWatch, "legacy-empty": ""} {
		got, err := replayed.GetTaskPRByID(ctx, id)
		if err != nil {
			t.Fatalf("get %s after replay: %v", id, err)
		}
		if got == nil || got.Source != want {
			t.Fatalf("%s source after replay = %q, want %q", id, sourceOfTaskPR(got), want)
		}
	}
}

func sourceOfTaskPR(tp *TaskPR) string {
	if tp == nil {
		return "<nil>"
	}
	return tp.Source
}
