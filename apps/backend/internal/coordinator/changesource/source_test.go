package changesource

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
)

type env struct {
	src *Source
	gh  *github.Store
	gl  *gitlab.Store
	db  *sqlx.DB
}

func newEnv(t *testing.T) *env {
	t.Helper()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	d := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = d.Close() })
	for _, q := range []string{
		`CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '', archived_at DATETIME)`,
		`CREATE TABLE workspaces (id TEXT PRIMARY KEY, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`INSERT INTO tasks (id, workspace_id) VALUES ('t1','ws'),('t2','ws'),('t3','other')`,
	} {
		if _, err := d.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	gh, err := github.NewStore(d, d)
	if err != nil {
		t.Fatal(err)
	}
	gl, err := gitlab.NewStore(d, d)
	if err != nil {
		t.Fatal(err)
	}
	return &env{src: New(d), gh: gh, gl: gl, db: d}
}

func (e *env) addPR(t *testing.T, task string, mut func(*github.TaskPR)) {
	t.Helper()
	now := time.Now()
	pr := &github.TaskPR{
		GitHubHost: "github.com", WorkspaceID: "ws", TaskID: task, RepositoryID: "r1", Owner: "o", Repo: "r",
		PRNumber: 7, PRURL: "u", PRTitle: "t", HeadBranch: "b", BaseBranch: "main", AuthorLogin: "a",
		State: "open", HeadSHA: "h1", ChecksState: "success", ChecksTotal: 2, ChecksPassing: 2,
		MergeableState: "clean", ReviewState: "approved", CreatedAt: now, LastSyncedAt: &now,
	}
	if mut != nil {
		mut(pr)
	}
	if err := e.gh.CreateTaskPR(context.Background(), pr); err != nil {
		t.Fatal(err)
	}
}

var ghKey = mergestate.ChangeKey{Provider: "github", Host: "github.com", RepoID: "r1", Number: 7}

func TestReadGitHubGreen(t *testing.T) {
	e := newEnv(t)
	yes := false
	e.addPR(t, "t1", func(p *github.TaskPR) { p.HasMergeConflicts = &yes })
	s, err := e.src.ReadChange(context.Background(), "ws", ghKey)
	if err != nil {
		t.Fatal(err)
	}
	if s.State != mergestate.StateOpen || s.HeadSHA != "h1" || s.Pipeline != mergestate.PipelineGreen ||
		s.Conflict != mergestate.ConflictClear || s.Mergeable.Mergeable != mergestate.Met || s.Approvals != mergestate.Met || s.Draft != mergestate.Met {
		t.Fatalf("snapshot = %+v", s)
	}
}

func TestReadGitHubNoChecksAndRed(t *testing.T) {
	e := newEnv(t)
	e.addPR(t, "t1", func(p *github.TaskPR) { p.ChecksState, p.ChecksTotal, p.ChecksPassing = "", 0, 0 })
	s, _ := e.src.ReadChange(context.Background(), "ws", ghKey)
	if s.Pipeline != mergestate.PipelineNone {
		t.Fatalf("pipeline = %s", s.Pipeline)
	}
	e.db.MustExec(`UPDATE github_task_prs SET checks_state='failure'`)
	s, _ = e.src.ReadChange(context.Background(), "ws", ghKey)
	if s.Pipeline != mergestate.PipelineRed {
		t.Fatalf("pipeline = %s", s.Pipeline)
	}
}

func TestReadNeverSyncedIsUnavailableNotPassing(t *testing.T) {
	e := newEnv(t)
	e.addPR(t, "t1", func(p *github.TaskPR) { p.LastSyncedAt = nil })
	s, err := e.src.ReadChange(context.Background(), "ws", ghKey)
	if err != nil {
		t.Fatal(err)
	}
	if s.Pipeline != mergestate.PipelineUnavailable || s.Mergeable.Mergeable != mergestate.Unavailable || s.Approvals == mergestate.Met {
		t.Fatalf("unsynced read looks healthy: %+v", s)
	}
}

func TestReadUnknownChangeIsAnError(t *testing.T) {
	e := newEnv(t)
	if _, err := e.src.ReadChange(context.Background(), "ws", ghKey); err == nil {
		t.Fatal("want error for a change with no link")
	}
}

func TestReadScopedToWorkspace(t *testing.T) {
	e := newEnv(t)
	e.addPR(t, "t3", nil) // task belongs to workspace "other"
	if _, err := e.src.ReadChange(context.Background(), "ws", ghKey); err == nil {
		t.Fatal("read crossed the workspace boundary")
	}
	ids, _ := e.src.TasksLinkedToChange(context.Background(), "ws", ghKey.String())
	if len(ids) != 0 {
		t.Fatalf("linked = %v", ids)
	}
}

func TestTasksLinkedToChangeSkipsArchived(t *testing.T) {
	e := newEnv(t)
	e.addPR(t, "t1", nil)
	e.addPR(t, "t2", nil)
	e.db.MustExec(`UPDATE tasks SET archived_at = CURRENT_TIMESTAMP WHERE id='t2'`)
	ids, err := e.src.TasksLinkedToChange(context.Background(), "ws", ghKey.String())
	if err != nil || len(ids) != 1 || ids[0] != "t1" {
		t.Fatalf("ids = %v err = %v", ids, err)
	}
}

func TestReadGitLab(t *testing.T) {
	e := newEnv(t)
	err := e.gl.UpsertTaskMR(context.Background(), &gitlab.TaskMR{
		TaskID: "t1", RepositoryID: "r2", Host: "https://GitLab.example.com/", ProjectPath: "g/p", MRIID: 3,
		MRURL: "u", MRTitle: "t", HeadBranch: "b", BaseBranch: "main", HeadSHA: "h9", State: "open",
		PipelineState: "failure", DetailedMergeStatus: "ci_must_pass", UnresolvedDiscussions: 2,
		ApprovalState: "approved", LastSyncedAt: ptr(time.Now()),
	})
	if err != nil {
		t.Fatal(err)
	}
	key := mergestate.ChangeKey{Provider: "gitlab", Host: "gitlab.example.com", RepoID: "r2", Number: 3}
	s, err := e.src.ReadChange(context.Background(), "ws", key)
	if err != nil {
		t.Fatal(err)
	}
	if s.Pipeline != mergestate.PipelineRed || s.UnresolvedThreads != 2 || s.Mergeable.Explains != mergestate.CondPipeline || s.HeadSHA != "h9" {
		t.Fatalf("snapshot = %+v", s)
	}
	ids, _ := e.src.TasksLinkedToChange(context.Background(), "ws", key.String())
	if len(ids) != 1 {
		t.Fatalf("linked = %v", ids)
	}
}

func ptr[T any](v T) *T { return &v }

func TestOpenChangesListsOnlyOpenLiveLinkedChanges(t *testing.T) {
	e := newEnv(t)
	e.addPR(t, "t1", nil)
	e.addPR(t, "t3", func(p *github.TaskPR) { p.PRNumber = 8 }) // other workspace
	e.addPR(t, "t2", func(p *github.TaskPR) { p.PRNumber = 9; p.State = "merged" })
	keys, err := e.src.OpenChanges(context.Background(), "ws")
	if err != nil || len(keys) != 1 || keys[0] != ghKey.String() {
		t.Fatalf("keys = %v err = %v", keys, err)
	}
}

func TestChangesLinkedToTask(t *testing.T) {
	e := newEnv(t)
	e.addPR(t, "t1", nil)
	e.addPR(t, "t1", func(p *github.TaskPR) { p.PRNumber = 8 })
	e.addPR(t, "t2", func(p *github.TaskPR) { p.PRNumber = 9 })
	keys, err := e.src.ChangesLinkedToTask(context.Background(), "ws", "t1")
	if err != nil || len(keys) != 2 {
		t.Fatalf("keys = %v err = %v", keys, err)
	}
	none, _ := e.src.ChangesLinkedToTask(context.Background(), "ws", "t3")
	if len(none) != 0 {
		t.Fatalf("cross-workspace task listed: %v", none)
	}
}
