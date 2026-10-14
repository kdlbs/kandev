package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	dbutil "github.com/kandev/kandev/internal/db"
	officeagents "github.com/kandev/kandev/internal/office/agents"
	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type adoptionHTTPFixture struct {
	router                   *gin.Engine
	db                       *sqlx.DB
	root, old, clone, source string
	manager                  *worktree.Manager
}

func adoptionGit(t *testing.T, path string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", path}, args...)...)
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "GIT_") {
			cmd.Env = append(cmd.Env, e)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return strings.TrimSpace(string(out))
}

func newAdoptionHTTPFixture(t *testing.T) adoptionHTTPFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	home, resolveErr := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, resolveErr)
	tasks := filepath.Join(home, "tasks")
	root := filepath.Join(tasks, "task-adoption")
	require.NoError(t, os.MkdirAll(root, 0700))
	require.NoError(t, storageworkspaces.WriteOwnershipMarker(root, storageworkspaces.OwnershipMarker{TaskID: "task-adoption", TaskDirName: "task-adoption", WorkspaceID: "ws-adoption", LayoutVersion: storageworkspaces.LayoutVersionSemantic}))
	source := filepath.Join(home, "source")
	require.NoError(t, os.Mkdir(source, 0700))
	adoptionGit(t, source, "init", "-b", "main")
	adoptionGit(t, source, "config", "user.email", "fixture@example.invalid")
	adoptionGit(t, source, "config", "user.name", "Fixture")
	adoptionGit(t, source, "config", "commit.gpgsign", "false")
	require.NoError(t, os.WriteFile(filepath.Join(source, "tracked"), []byte("original\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(source, ".gitignore"), []byte("ignored\n"), 0600))
	adoptionGit(t, source, "add", "tracked", ".gitignore")
	adoptionGit(t, source, "commit", "-m", "fixture")
	adoptionGit(t, source, "config", "remote.origin.url", "https://example.invalid/owned/repo.git")
	old := filepath.Join(root, "broken")
	clone := filepath.Join(root, "independent")
	adoptionGit(t, source, "worktree", "add", "-b", "feature/adoption", old, "main")
	marker, err := os.ReadFile(filepath.Join(old, ".git"))
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(strings.TrimSpace(strings.TrimPrefix(string(marker), "gitdir:"))))
	adoptionGit(t, source, "clone", "--no-local", source, clone)
	adoptionGit(t, clone, "checkout", "feature/adoption")
	adoptionGit(t, clone, "config", "remote.origin.url", "https://example.invalid/owned/repo.git")
	require.NoError(t, os.WriteFile(filepath.Join(clone, "tracked"), []byte("dirty\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(clone, "untracked"), []byte("preserve\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(clone, "staged"), []byte("stage\n"), 0600))
	adoptionGit(t, clone, "add", "staged")
	require.NoError(t, os.WriteFile(filepath.Join(clone, "ignored"), []byte("ignored fixture\n"), 0600))
	sibling := filepath.Join(root, "sibling")
	adoptionGit(t, source, "worktree", "add", "-b", "feature/sibling", sibling, "main")
	raw, err := dbutil.OpenSQLite(filepath.Join(home, "fixture.db"))
	require.NoError(t, err)
	db := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	repo, err := tasksqlite.NewWithDB(db, db, nil)
	require.NoError(t, err)
	statements := []struct {
		q    string
		args []any
	}{
		{`INSERT INTO workspaces(id,name,owner_id,created_at,updated_at) VALUES('ws-adoption','Fixture','operator',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, nil},
		{`INSERT INTO tasks(id,workspace_id,title,state,created_at,updated_at) VALUES('task-adoption','ws-adoption','Fixture','IN_PROGRESS',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, nil},
		{`INSERT INTO repositories(id,workspace_id,name,source_type,local_path,created_at,updated_at) VALUES('repo-adoption','ws-adoption','Fixture','local',?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, []any{source}},
		{`INSERT INTO task_repositories(id,task_id,repository_id,checkout_branch,position,created_at,updated_at) VALUES('attachment-adoption','task-adoption','repo-adoption','feature/adoption',2,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, nil},
		{`INSERT INTO task_environments(id,task_id,executor_type,status,ownership_generation,task_dir_name,workspace_path,created_at,updated_at) VALUES('env-adoption','task-adoption','worktree','ready',1,'task-adoption',?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, []any{root}},
		{`INSERT INTO task_environment_repos(id,task_environment_id,repository_id,worktree_id,worktree_path,worktree_branch,branch_slug,position,status,created_at,updated_at) VALUES('slot-adoption','env-adoption','repo-adoption','wt-adoption',?,'feature/adoption','feature-adoption',2,'active',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, []any{old}},
		{`INSERT INTO task_sessions(id,queue_incarnation_id,task_id,agent_profile_id,state,task_environment_id,workspace_path,started_at,updated_at) VALUES('session-adoption','incarnation-adoption','task-adoption','profile','WAITING_FOR_INPUT','env-adoption',?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, []any{root}},
	}
	statements = append(statements, struct {
		q    string
		args []any
	}{`INSERT INTO task_repositories(id,task_id,repository_id,checkout_branch,position,created_at,updated_at) VALUES('attachment-sibling','task-adoption','repo-adoption','feature/sibling',0,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, nil})
	statements = append(statements, struct {
		q    string
		args []any
	}{`INSERT INTO task_environment_repos(id,task_environment_id,repository_id,worktree_id,worktree_path,worktree_branch,branch_slug,position,status,created_at,updated_at) VALUES('slot-sibling','env-adoption','repo-adoption','wt-sibling',?,'feature/sibling','feature-sibling',0,'active',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, []any{sibling}})

	for _, st := range statements {
		_, err := db.Exec(st.q, st.args...)
		require.NoError(t, err)
	}
	log := newTestLogger(t)
	svc := service.NewService(service.Repos{Workspaces: repo, Tasks: repo, TaskRepos: repo, Sessions: repo, RepoEntities: repo, TaskEnvironments: repo}, nil, log, service.RepositoryDiscoveryConfig{})
	store, err := worktree.NewSQLiteStore(db, db)
	require.NoError(t, err)
	mgr, err := worktree.NewManager(worktree.Config{Enabled: true, TasksBasePath: tasks}, store, log)
	require.NoError(t, err)
	svc.SetWorktreeCleanup(mgr)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		role := authn.RoleAdmin
		if c.GetHeader("X-Fixture-Member") == "true" {
			role = authn.RoleMember
		}
		authn.SetOnGin(c, authn.Identity{UserID: "operator", Role: role})
		if c.GetHeader("X-Fixture-Agent") == "true" {
			c.Set("agent_claims", &officeagents.AgentClaims{})
		}
	})
	h := RegisterTaskRoutes(router, ws.NewDispatcher(), svc, nil, repo, nil, log)
	h.SetWorkspaceAdoptionCallerGuard(func(c *gin.Context) bool {
		return officeagents.CallerFromContext(c) == nil && officeagents.ClaimsFromContext(c) == nil
	})
	return adoptionHTTPFixture{router: router, db: db, root: root, old: old, clone: clone, source: source, manager: mgr}
}

func adoptionRequest(router *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	data, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(data)))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	return rec
}

func TestWorkspaceAdoptionHTTPPreservesIdentityAndContent(t *testing.T) {
	f := newAdoptionHTTPFixture(t)
	marker, err := os.ReadFile(filepath.Join(f.old, ".git"))
	require.NoError(t, err)
	index, err := os.ReadFile(filepath.Join(f.clone, ".git", "index"))
	require.NoError(t, err)
	rec := adoptionRequest(f.router, "/api/v1/tasks/task-adoption/workspace-adoption/preview", map[string]any{"session_id": "session-adoption", "slot_id": "slot-adoption", "checkout_path": f.clone})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var preview map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &preview))
	require.NotEmpty(t, preview["token"])
	var beforePath string
	require.NoError(t, f.db.Get(&beforePath, `SELECT worktree_path FROM task_environment_repos WHERE id='slot-adoption'`))
	require.Equal(t, f.old, beforePath)
	rec = adoptionRequest(f.router, "/api/v1/tasks/task-adoption/workspace-adoption/apply", map[string]any{"preview": preview, "operation_id": "11111111-1111-4111-8111-111111111111"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var row struct {
		ID             string
		WorktreeID     string `db:"worktree_id"`
		WorktreePath   string `db:"worktree_path"`
		WorktreeBranch string `db:"worktree_branch"`
		Position       int
	}
	require.NoError(t, f.db.Get(&row, `SELECT id,worktree_id,worktree_path,worktree_branch,position FROM task_environment_repos WHERE id='slot-adoption'`))
	require.Equal(t, "wt-adoption", row.WorktreeID)
	require.Equal(t, f.clone, row.WorktreePath)
	require.Equal(t, "feature/adoption", row.WorktreeBranch)
	require.Equal(t, 2, row.Position)
	var count int
	require.NoError(t, f.db.Get(&count, `SELECT COUNT(*) FROM task_sessions WHERE id='session-adoption' AND queue_incarnation_id='incarnation-adoption' AND task_environment_id='env-adoption' AND workspace_path=?`, f.root))
	require.Equal(t, 1, count)
	after, err := os.ReadFile(filepath.Join(f.old, ".git"))
	require.NoError(t, err)
	require.Equal(t, marker, after)
	after, err = os.ReadFile(filepath.Join(f.clone, ".git", "index"))
	require.NoError(t, err)
	require.Equal(t, index, after)
	data, err := os.ReadFile(filepath.Join(f.clone, "tracked"))
	require.NoError(t, err)
	require.Equal(t, "dirty\n", string(data))
	for name, want := range map[string]string{"untracked": "preserve\n", "staged": "stage\n", "ignored": "ignored fixture\n"} {
		data, err := os.ReadFile(filepath.Join(f.clone, name))
		require.NoError(t, err)
		require.Equal(t, want, string(data))
	}
	var siblingPath string
	require.NoError(t, f.db.Get(&siblingPath, `SELECT worktree_path FROM task_environment_repos WHERE id='slot-sibling' AND position=0 AND worktree_id='wt-sibling'`))
	require.Equal(t, filepath.Join(f.root, "sibling"), siblingPath)

}

func adoptionPreview(t *testing.T, f adoptionHTTPFixture) models.WorkspaceAdoptionPreview {
	t.Helper()
	rec := adoptionRequest(f.router, "/api/v1/tasks/task-adoption/workspace-adoption/preview", map[string]any{"session_id": "session-adoption", "slot_id": "slot-adoption", "checkout_path": f.clone})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var p models.WorkspaceAdoptionPreview
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &p))
	return p
}

func TestWorkspaceAdoptionHTTPNativeWorkingDirectory(t *testing.T) {
	f := newAdoptionHTTPFixture(t)
	for _, table := range []string{"task_environments", "task_sessions"} {
		_, err := f.db.Exec("UPDATE "+table+" SET workspace_path=?", f.old)
		require.NoError(t, err)
	}
	p := adoptionPreview(t, f)
	rec := adoptionRequest(f.router, "/api/v1/tasks/task-adoption/workspace-adoption/apply", map[string]any{"preview": p, "operation_id": "11111111-1111-4111-8111-111111111111"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	for _, table := range []string{"task_environments", "task_sessions"} {
		var cwd string
		require.NoError(t, f.db.Get(&cwd, "SELECT workspace_path FROM "+table))
		require.Equal(t, f.clone, cwd)
	}
	replay := adoptionRequest(f.router, "/api/v1/tasks/task-adoption/workspace-adoption/apply", map[string]any{"preview": p, "operation_id": "11111111-1111-4111-8111-111111111111"})
	require.Equal(t, http.StatusOK, replay.Code, replay.Body.String())
	require.JSONEq(t, rec.Body.String(), replay.Body.String())
}

func TestWorkspaceAdoptionHTTPRefusesStaleAndForeignProof(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*testing.T, adoptionHTTPFixture, models.WorkspaceAdoptionPreview)
	}{
		{"session incarnation", func(t *testing.T, f adoptionHTTPFixture, _ models.WorkspaceAdoptionPreview) {
			_, err := f.db.Exec(`UPDATE task_sessions SET queue_incarnation_id='replacement' WHERE id='session-adoption'`)
			require.NoError(t, err)
		}},
		{"ownership generation", func(t *testing.T, f adoptionHTTPFixture, _ models.WorkspaceAdoptionPreview) {
			_, err := f.db.Exec(`UPDATE task_environments SET ownership_generation=2 WHERE id='env-adoption'`)
			require.NoError(t, err)
		}},
		{"attachment ordering", func(t *testing.T, f adoptionHTTPFixture, _ models.WorkspaceAdoptionPreview) {
			_, err := f.db.Exec(`UPDATE task_repositories SET position=3 WHERE id='attachment-adoption'`)
			require.NoError(t, err)
		}},
		{"candidate origin", func(t *testing.T, f adoptionHTTPFixture, _ models.WorkspaceAdoptionPreview) {
			adoptionGit(t, f.clone, "config", "remote.origin.url", "https://example.invalid/foreign/repo.git")
		}},
		{"candidate branch", func(t *testing.T, f adoptionHTTPFixture, _ models.WorkspaceAdoptionPreview) {
			adoptionGit(t, f.clone, "checkout", "-b", "foreign")
		}},
		{"restored administration", func(t *testing.T, _ adoptionHTTPFixture, p models.WorkspaceAdoptionPreview) {
			require.NoError(t, os.MkdirAll(p.Proof.MissingGitDir, 0700))
		}},
		{"marker replacement", func(t *testing.T, f adoptionHTTPFixture, _ models.WorkspaceAdoptionPreview) {
			require.NoError(t, os.WriteFile(filepath.Join(f.old, ".git"), []byte("gitdir: /foreign/missing\n"), 0600))
		}},
		{"root ownership", func(t *testing.T, f adoptionHTTPFixture, _ models.WorkspaceAdoptionPreview) {
			require.NoError(t, os.WriteFile(filepath.Join(f.root, storageworkspaces.OwnershipMarkerFilename), []byte(`{"task_id":"foreign"}`), 0600))
		}},
		{"live consumer", func(t *testing.T, f adoptionHTTPFixture, _ models.WorkspaceAdoptionPreview) {
			_, err := f.db.Exec(`INSERT INTO task_sessions(id,queue_incarnation_id,task_id,agent_profile_id,state,task_environment_id,workspace_path,started_at,updated_at) VALUES('live','live-incarnation','task-adoption','profile','RUNNING','env-adoption',?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, f.root)
			require.NoError(t, err)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAdoptionHTTPFixture(t)
			p := adoptionPreview(t, f)
			tc.mutate(t, f, p)
			rec := adoptionRequest(f.router, "/api/v1/tasks/task-adoption/workspace-adoption/apply", map[string]any{"preview": p, "operation_id": "11111111-1111-4111-8111-111111111111"})
			require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
			var path string
			require.NoError(t, f.db.Get(&path, `SELECT worktree_path FROM task_environment_repos WHERE id='slot-adoption'`))
			require.Equal(t, f.old, path)
			var receipts int
			require.NoError(t, f.db.Get(&receipts, `SELECT COUNT(*) FROM task_environment_checkout_adoptions`))
			require.Zero(t, receipts)
		})
	}
}

func TestWorkspaceAdoptionHTTPReplayAndProvenance(t *testing.T) {
	f := newAdoptionHTTPFixture(t)
	p := adoptionPreview(t, f)
	const operationID = "11111111-1111-4111-8111-111111111111"
	apply := func(preview models.WorkspaceAdoptionPreview) *httptest.ResponseRecorder {
		return adoptionRequest(f.router, "/api/v1/tasks/task-adoption/workspace-adoption/apply", map[string]any{"preview": preview, "operation_id": operationID})
	}
	first := apply(p)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	replay := apply(p)
	require.Equal(t, http.StatusOK, replay.Code, replay.Body.String())
	require.JSONEq(t, first.Body.String(), replay.Body.String())
	changed := p
	changed.Proof.MarkerSHA256 = "changed"
	changed.Token = changed.Digest()
	require.Equal(t, http.StatusConflict, apply(changed).Code)
	require.NoError(t, os.MkdirAll(p.Proof.MissingGitDir, 0700))
	require.Equal(t, http.StatusConflict, apply(p).Code)
}

func TestWorkspaceAdoptionHTTPOperatorBoundary(t *testing.T) {
	f := newAdoptionHTTPFixture(t)
	for _, header := range []string{"X-Fixture-Member", "X-Fixture-Agent"} {
		for _, endpoint := range []struct{ method, suffix string }{
			{http.MethodPost, "preview"},
			{http.MethodPost, "apply"},
			{http.MethodGet, "11111111-1111-4111-8111-111111111111"},
		} {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(endpoint.method, "/api/v1/tasks/task-adoption/workspace-adoption/"+endpoint.suffix, strings.NewReader(`{}`))
			req.Header.Set(header, "true")
			f.router.ServeHTTP(rec, req)
			require.Equal(t, http.StatusForbidden, rec.Code, endpoint.suffix)
		}
	}
	for _, body := range []string{`{"unexpected":"field"}`, `{} {}`, strings.Repeat("x", 256*1024+1)} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-adoption/workspace-adoption/preview", strings.NewReader(body))
		f.router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	}
}

func TestWorkspaceAdoptionRefusesForgedOriginWithUnrelatedHistory(t *testing.T) {
	f := newAdoptionHTTPFixture(t)
	adoptionGit(t, f.clone, "config", "user.email", "fixture@example.invalid")
	adoptionGit(t, f.clone, "config", "user.name", "Fixture")
	adoptionGit(t, f.clone, "config", "commit.gpgsign", "false")
	adoptionGit(t, f.clone, "checkout", "--orphan", "unrelated-history")
	adoptionGit(t, f.clone, "add", "tracked", "untracked")
	adoptionGit(t, f.clone, "commit", "-m", "unrelated root")
	adoptionGit(t, f.clone, "branch", "-D", "feature/adoption")
	adoptionGit(t, f.clone, "branch", "-m", "feature/adoption")
	rec := adoptionRequest(f.router, "/api/v1/tasks/task-adoption/workspace-adoption/preview", map[string]any{"session_id": "session-adoption", "slot_id": "slot-adoption", "checkout_path": f.clone})
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
}
