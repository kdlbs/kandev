package inventoryrepair

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
)

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.3, AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.7
func TestLegacyAliasLifecycleDirtyConsent(t *testing.T) {
	f := lifecycleAliasFixture(t, false)
	path := f.plan.Repairs[0].Path
	files := map[string]string{"tracked": "unstaged change\n", "staged.txt": "staged change\n", "untracked.txt": "untracked change\n", "ignored": "ignored content\n", ".gitignore": "ignored\n"}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(path, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git(t, path, "add", "staged.txt")
	index := git(t, path, "diff", "--cached", "--binary")
	diff := git(t, path, "diff", "--binary")
	f = repairAndReopenLifecycle(t, f)
	for name, content := range files {
		got, err := os.ReadFile(filepath.Join(path, name))
		if err != nil || string(got) != content {
			t.Fatalf("repair changed %s: %q %v", name, got, err)
		}
	}
	if git(t, path, "diff", "--cached", "--binary") != index || git(t, path, "diff", "--binary") != diff {
		t.Fatal("repair changed index or tracked changes")
	}
	svc, _, router := lifecycleServices(t, f)
	missing := lifecycleRequest(t, router, http.MethodDelete, "/api/v1/tasks/owner?cascade=false", "", nil)
	if missing.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing confirmation: %d %s", missing.Code, missing.Body.String())
	}
	preview := lifecycleRequest(t, router, http.MethodPost, "/api/v1/tasks/delete-preflight", "", map[string]any{"task_ids": []string{"owner"}, "cascade": false})
	if preview.Code != http.StatusOK {
		t.Fatalf("dirty preview: %d %s", preview.Code, preview.Body.String())
	}
	var consent service.TaskDeletePreflightResult
	if err := json.Unmarshal(preview.Body.Bytes(), &consent); err != nil {
		t.Fatal(err)
	}
	if !consent.RequiresDiscardConsent {
		t.Fatal("dirty files lost their consent requirement")
	}
	refused := lifecycleRequest(t, router, http.MethodDelete, "/api/v1/tasks/owner?cascade=false", consent.ConfirmationID, nil)
	if refused.Code == http.StatusOK {
		t.Fatal("dirty deletion succeeded without discard consent")
	}
	for name, content := range files {
		got, err := os.ReadFile(filepath.Join(path, name))
		if err != nil || string(got) != content {
			t.Fatalf("refused deletion changed %s", name)
		}
	}
	lifecycleDelete(t, router, "owner", false, true)
	if err := svc.ResumeTaskResourceCleanupJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("confirmed deletion left checkout: %v", err)
	}
	assertLegacyCleanupEvidence(t, f)
}

func TestLegacyAliasLifecycleResumeIdentity(t *testing.T) {
	f := lifecycleAliasFixture(t, true)
	execSQL(t, f.db, `INSERT INTO executors_running(id,session_id,task_id,executor_id,status,resume_token,created_at,updated_at) VALUES('retained-runtime','child-session','child','executor','stopped','provider-resume-token',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	f = repairAndReopenLifecycle(t, f)
	var session, token string
	if err := f.db.QueryRow(`SELECT session_id,resume_token FROM executors_running WHERE id='retained-runtime'`).Scan(&session, &token); err != nil {
		t.Fatal(err)
	}
	if session != "child-session" || token != "provider-resume-token" {
		t.Fatalf("provider resume identity changed: %s %s", session, token)
	}
}

func TestLegacyAliasLifecycleHealthyBranch(t *testing.T) {
	f := lifecycleAliasFixture(t, false)
	other := filepath.Join(filepath.Dir(f.plan.Repairs[0].Path), "second")
	git(t, f.repo, "worktree", "add", "-b", "feature/second", other)
	execSQL(t, f.db, `INSERT INTO task_environment_repos(id,task_environment_id,repository_id,branch_slug,worktree_id,worktree_path,worktree_branch,status,created_at,updated_at) VALUES('healthy','env','repo','second','second-worktree',?,'feature/second','active',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, other)
	head := git(t, other, "rev-parse", "HEAD")
	f = repairAndReopenLifecycle(t, f)
	store, err := worktree.NewSQLiteStore(f.db, f.db)
	if err != nil {
		t.Fatal(err)
	}
	slots, err := store.GetWorktreesBySessionID(context.Background(), "session")
	if err != nil || len(slots) != 2 {
		t.Fatalf("healthy branch inventory: %+v %v", slots, err)
	}
	if git(t, other, "rev-parse", "HEAD") != head {
		t.Fatal("repair changed healthy branch")
	}
}
