package worktree

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/task/models"
	"testing"
)

func seedAliasIdentity(t *testing.T) (*SQLiteStore, *Worktree) {
	t.Helper()
	s := newTestStore(t)
	s.seedSessionWithEnvironment(t, "alias-session", "alias-task")
	wt := &Worktree{ID: "physical", SessionID: "alias-session", RepositoryID: "repo", Path: "/tmp/task/repo", Branch: "feature", Status: StatusActive}
	if err := s.CreateWorktree(context.Background(), wt); err != nil {
		t.Fatal(err)
	}
	_, err := s.db.Exec(`INSERT INTO task_environment_repos
 (id,task_environment_id,repository_id,branch_slug,worktree_id,worktree_path,worktree_branch,status,created_at,updated_at)
 VALUES ('alias','env-alias-session','repo','alias','physical','/tmp/task','feature','active',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	if err != nil {
		t.Fatal(err)
	}
	return s, wt
}

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.8
func TestStoreAliasIdentityReads(t *testing.T) {
	s, wt := seedAliasIdentity(t)
	ctx := context.Background()
	reads := map[string]func() error{
		"archive eligibility":  func() error { _, err := s.IsArchivedBranchCandidate(ctx, wt.ID); return err },
		"physical ID":          func() error { _, err := s.GetWorktreeByID(ctx, wt.ID); return err },
		"session":              func() error { _, err := s.GetWorktreeBySessionID(ctx, wt.SessionID); return err },
		"task":                 func() error { _, err := s.GetWorktreeByTaskID(ctx, "alias-task"); return err },
		"session inventory":    func() error { _, err := s.GetWorktreesBySessionID(ctx, wt.SessionID); return err },
		"repository inventory": func() error { _, err := s.GetWorktreesByRepositoryID(ctx, wt.RepositoryID); return err },
	}
	for name, read := range reads {
		t.Run(name, func(t *testing.T) {
			if err := read(); err == nil {
				t.Fatal("competing physical owners were accepted")
			}
		})
	}
}

func TestStoreAliasIdentityWrites(t *testing.T) {
	writes := map[string]func(*SQLiteStore, *Worktree) error{
		"create replacement": func(s *SQLiteStore, w *Worktree) error {
			w.ID = "replacement"
			return s.CreateWorktree(context.Background(), w)
		},
		"update": func(s *SQLiteStore, w *Worktree) error {
			w.Path = "/changed"
			return s.UpdateWorktree(context.Background(), w)
		},
		"delete": func(s *SQLiteStore, w *Worktree) error { return s.DeleteWorktree(context.Background(), w.ID) },
		"create": func(s *SQLiteStore, w *Worktree) error { return s.CreateWorktree(context.Background(), w) },
		"replace": func(s *SQLiteStore, w *Worktree) error {
			n := *w
			n.ID = "replacement"
			_, err := s.CompareAndSwapWorktree(context.Background(), w, &n)
			return err
		},
		"recovery head": func(s *SQLiteStore, w *Worktree) error {
			_, err := s.PersistBranchRecoveryHead(context.Background(), w.ID, "", "head")
			return err
		},
		"compaction": func(s *SQLiteStore, w *Worktree) error {
			_, err := s.PersistBranchCompactionComplete(context.Background(), w.ID, "")
			return err
		},
		"restore": func(s *SQLiteStore, w *Worktree) error {
			_, err := s.PersistBranchRecoveryRestored(context.Background(), w.ID, "")
			return err
		},
		"archive head": func(s *SQLiteStore, w *Worktree) error {
			_, err := s.PersistArchivedBranchRecoveryHead(context.Background(), w.ID, "", "head")
			return err
		},
		"archive compaction": func(s *SQLiteStore, w *Worktree) error {
			_, err := s.PersistArchivedBranchCompactionComplete(context.Background(), w.ID, "")
			return err
		},
		"archive touch": func(s *SQLiteStore, w *Worktree) error {
			return s.TouchArchivedBranchCandidate(context.Background(), w.ID)
		},
	}
	for name, write := range writes {
		t.Run(name, func(t *testing.T) {
			s, w := seedAliasIdentity(t)
			var before string
			if err := s.db.Get(&before, `SELECT json_group_array(json_object('id',id,'path',worktree_path,'head',worktree_recovery_head_sha,'updated',updated_at)) FROM task_environment_repos ORDER BY id`); err != nil {
				t.Fatal(err)
			}
			err := write(s, w)
			var ambiguous *models.WorktreeIdentityAmbiguousError
			if !errors.As(err, &ambiguous) {
				t.Errorf("expected typed ambiguity, got %v", err)
			}
			var after string
			if err := s.db.Get(&after, `SELECT json_group_array(json_object('id',id,'path',worktree_path,'head',worktree_recovery_head_sha,'updated',updated_at)) FROM task_environment_repos ORDER BY id`); err != nil {
				t.Fatal(err)
			}
			if after != before {
				t.Fatal("ambiguous write changed inventory")
			}
		})
	}
}

func TestStoreAliasIdentityDeletedHistory(t *testing.T) {
	s, w := seedAliasIdentity(t)
	ctx := context.Background()
	if _, err := s.db.Exec(`UPDATE task_environment_repos SET status='deleted',deleted_at=CURRENT_TIMESTAMP WHERE id='alias'`); err != nil {
		t.Fatal(err)
	}
	// Multiple sessions resolve the same canonical slot.
	if _, err := s.db.Exec(`INSERT INTO task_sessions (id,task_id,state,task_environment_id,started_at,updated_at) VALUES ('borrower','alias-task','COMPLETED','env-alias-session',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetWorktreeByID(ctx, w.ID)
	if err != nil || got == nil || got.Path != w.Path {
		t.Fatalf("canonical read: %+v, %v", got, err)
	}
	list, err := s.GetWorktreesByTaskID(ctx, "alias-task")
	if err != nil || len(list) != 1 {
		t.Fatalf("session join: %+v, %v", list, err)
	}
	if ok, err := s.PersistBranchRecoveryHead(ctx, w.ID, "", "canonical-head"); err != nil || !ok {
		t.Fatalf("head update: %v %v", ok, err)
	}
	var historicalHead string
	if err := s.db.Get(&historicalHead, `SELECT COALESCE(worktree_recovery_head_sha,'') FROM task_environment_repos WHERE id='alias'`); err != nil {
		t.Fatal(err)
	}
	if historicalHead != "" {
		t.Fatal("canonical update changed history")
	}
	if err := s.DeleteWorktree(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := s.db.Get(&remaining, `SELECT COUNT(*) FROM task_environment_repos WHERE id='alias'`); err != nil || remaining != 1 {
		t.Fatalf("history removed: %d %v", remaining, err)
	}
}

func TestStoreAliasIdentityRecoverySnapshot(t *testing.T) {
	s, w := seedAliasIdentity(t)
	_, err := s.ReadRecoverySelectionSnapshot(context.Background(), models.WorkspaceRecoverySelectionSnapshot{SessionID: w.SessionID, TaskEnvironmentID: w.TaskEnvironmentID})
	var ambiguous *models.WorktreeIdentityAmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("recovery admitted ambiguous slots: %v", err)
	}
}
