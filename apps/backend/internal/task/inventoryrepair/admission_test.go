package inventoryrepair

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/worktree"
	"go.uber.org/zap"
)

func fixtureManager(t *testing.T, f fixture) *worktree.Manager {
	t.Helper()
	store, err := worktree.NewSQLiteStore(f.db, f.db)
	if err != nil {
		t.Fatal(err)
	}
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	mgr, err := worktree.NewManager(worktree.Config{Enabled: true, TasksBasePath: f.plan.TasksRoot}, store, log)
	if err != nil {
		t.Fatal(err)
	}
	return mgr
}

func TestRepairedSharedInventoryPassesExistingAdmission(t *testing.T) {
	f := relocationFixture(t)
	mgr := fixtureManager(t, f)
	ctx := context.Background()
	req := worktree.CreateRequest{TaskID: "owner", SessionID: "session", TaskEnvironmentID: "env", WorktreeID: "wt", RepositoryID: "repo", RepositoryPath: f.repo, BaseBranch: "main", ReuseRequired: true}
	if _, err := mgr.Create(ctx, req); err == nil {
		t.Fatal("foreign-root inventory unexpectedly admitted")
	}
	p := observedPlan(t, f)
	if err := repairWithIsolatedHost(p, "apply"); err != nil {
		t.Fatal(err)
	}
	wt, err := mgr.Create(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if wt.Path != p.Repairs[0].Path || wt.ID != "wt" {
		t.Fatalf("wrong admitted checkout: %+v", wt)
	}
	if err := Verify(ctx, p); err != nil {
		t.Fatal("admission changed checkout: ", err)
	}
}

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.3
func TestBranchRepairKeepsArchiveGuards(t *testing.T) {
	for _, dirty := range []bool{false, true} {
		t.Run(map[bool]string{false: "clean", true: "dirty"}[dirty], func(t *testing.T) {
			f := newFixture(t)
			ctx := context.Background()
			r := f.plan.Repairs[0]
			execSQL(t, f.db, `UPDATE tasks SET archived_at=CURRENT_TIMESTAMP WHERE id='owner'`)
			git(t, f.repo, "commit", "--allow-empty", "-m", "advance other branch")
			oldRef := git(t, f.repo, "rev-parse", "HEAD")
			git(t, f.repo, "update-ref", "refs/heads/feature/stale", oldRef)
			mgr := fixtureManager(t, f)
			wt, err := mgr.GetByID(ctx, "wt")
			if err != nil {
				t.Fatal(err)
			}
			if err = mgr.CleanupArchivedWorktree(ctx, wt, "owner", r.Path, f.repo); err == nil {
				t.Fatal("divergent recorded branch was removed")
			}
			if dirty {
				if err = os.WriteFile(filepath.Join(r.Path, "tracked"), []byte("keep uncommitted change"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			p := observedPlan(t, f)
			if err = repairWithIsolatedHost(p, "apply"); err != nil {
				t.Fatal(err)
			}
			wt, err = mgr.GetByID(ctx, "wt")
			if err != nil {
				t.Fatal(err)
			}
			err = mgr.CleanupArchivedWorktree(ctx, wt, "owner", r.Path, f.repo)
			if dirty {
				if err == nil {
					t.Fatal("dirty checkout removed")
				}
				if _, err = os.Stat(r.Path); err != nil {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if got := git(t, f.repo, "rev-parse", "feature/stale"); got != oldRef {
				t.Fatal("old branch changed")
			}
			if got := git(t, f.repo, "rev-parse", r.Branch); got != r.HeadOID {
				t.Fatal("attached branch changed")
			}
		})
	}
}

func TestCleanupSuccessorPassesRealSourceManifestCapture(t *testing.T) {
	f := cleanupFixture(t)
	mgr := fixtureManager(t, f)
	p := observedPlan(t, f)
	if err := repairWithIsolatedHost(p, "apply"); err != nil {
		t.Fatal(err)
	}
	var data string
	if err := f.db.QueryRow(`SELECT resource_snapshot FROM task_resource_cleanup_jobs WHERE state='pending'`).Scan(&data); err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Worktrees []*worktree.Worktree `json:"worktrees"`
		Heads     map[string]string    `json:"worktree_head_oids"`
	}
	if err := json.Unmarshal([]byte(data), &snapshot); err != nil {
		t.Fatal(err)
	}
	manifests, err := mgr.CaptureArchiveSourceManifests(context.Background(), snapshot.Worktrees)
	if err != nil {
		t.Fatal(err)
	}
	if manifests["wt"].HeadOID != snapshot.Heads["wt"] {
		t.Fatal("successor omitted current HEAD evidence")
	}
}
