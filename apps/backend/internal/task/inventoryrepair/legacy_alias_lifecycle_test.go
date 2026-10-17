package inventoryrepair

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	dbutil "github.com/kandev/kandev/internal/db"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/task/handlers"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

const legacyCleanupEvidence = `{"legacy":"preserve these exact bytes"}`

func lifecycleAliasFixture(t *testing.T, shared bool) fixture {
	t.Helper()
	f := aliasFixture(t)
	if _, err := officesqlite.NewWithDB(f.db, f.db, nil); err != nil {
		t.Fatal(err)
	}
	seedLifecycleHistory(t, f, "owner", "session")
	execSQL(t, f.db, `UPDATE task_sessions SET downstream_acp_session_id='conversation-owner' WHERE id='session'`)
	execSQL(t, f.db, `INSERT INTO task_resource_cleanup_jobs(id,operation_id,task_id,trigger,state,resource_snapshot,created_at,updated_at) VALUES('old-job','old-operation','owner','archive','cancelled',?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, legacyCleanupEvidence)
	if !shared {
		return f
	}
	root := filepath.Dir(f.plan.Repairs[0].Path)
	execSQL(t, f.db, `INSERT INTO tasks(id,workspace_id,title,state,parent_id,metadata,created_at,updated_at) VALUES('child','workspace','Child','FAILED','owner','{"workspace":{"mode":"inherit_parent","group_id":"group"}}',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	execSQL(t, f.db, `INSERT INTO task_workspace_groups(id,workspace_id,owner_task_id,materialized_environment_id,materialized_path,materialized_kind,owned_by_kandev,cleanup_policy,restore_config_json,created_at,updated_at) VALUES('group','workspace','owner','env',?,'single_repo',1,'delete_when_last_member_archived_or_deleted','{"kind":"single_repo","worktree_ids":{"repo":"wt"}}',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, f.plan.Repairs[0].Path)
	execSQL(t, f.db, `INSERT INTO task_workspace_group_members(workspace_group_id,task_id,role,created_at) VALUES('group','owner','owner',CURRENT_TIMESTAMP),('group','child','member',CURRENT_TIMESTAMP)`)
	execSQL(t, f.db, `INSERT INTO task_environments(id,task_id,executor_type,status,task_dir_name,workspace_path,created_at,updated_at) VALUES('child-env','child','worktree','ready','owner_root',?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, root)
	execSQL(t, f.db, `INSERT INTO task_environment_repos(id,task_environment_id,repository_id,worktree_id,worktree_path,worktree_branch,created_at,updated_at) VALUES('copy','child-env','repo','wt',?,'feature/actual',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, f.plan.Repairs[0].Path)
	execSQL(t, f.db, `INSERT INTO task_sessions(id,task_id,agent_profile_id,state,task_environment_id,workspace_path,downstream_acp_session_id,started_at,updated_at) VALUES('child-session','child','profile','WAITING_FOR_INPUT','child-env',?,'conversation-child',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, root)
	seedLifecycleHistory(t, f, "child", "child-session")
	f.plan.Aliases[0].AliasRowIDs = append(f.plan.Aliases[0].AliasRowIDs, "copy")
	f.plan.RedundantEnvironments = []EnvironmentRebind{{EnvironmentID: "child-env", CanonicalEnvironmentID: "env", GroupID: "group", SessionIDs: []string{"child-session"}}}
	addAliasSnapshot(t, &f)
	return f
}

func repairAndReopenLifecycle(t *testing.T, f fixture) fixture {
	t.Helper()
	discovery, err := Discover(context.Background(), DiscoveryOptions{Home: f.plan.Home, Database: f.plan.Database, TasksRoot: f.plan.TasksRoot})
	if err != nil || len(discovery.Groups) != 1 {
		t.Fatalf("discovery: %+v %v", discovery, err)
	}
	p := observedPlan(t, f)
	if err := repairWithIsolatedHost(p, "apply"); err != nil {
		t.Fatal(err)
	}
	if err := Verify(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	conn, err := dbutil.OpenSQLite(p.Database)
	if err != nil {
		t.Fatal(err)
	}
	f.db = sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = f.db.Close() })
	f.plan = p
	var conversation string
	if err := f.db.Get(&conversation, `SELECT downstream_acp_session_id FROM task_sessions WHERE id='session'`); err != nil || conversation != "conversation-owner" {
		t.Fatalf("owner conversation: %q %v", conversation, err)
	}
	if len(p.RedundantEnvironments) > 0 {
		var environment, state string
		if err := f.db.QueryRow(`SELECT task_environment_id,state,downstream_acp_session_id FROM task_sessions WHERE id='child-session'`).Scan(&environment, &state, &conversation); err != nil {
			t.Fatal(err)
		}
		if environment != "env" || state != "WAITING_FOR_INPUT" || conversation != "conversation-child" {
			t.Fatalf("child identity changed: %s %s %s", environment, state, conversation)
		}
		if err := f.db.Get(&environment, `SELECT task_environment_id FROM task_session_git_snapshots WHERE id='snapshot'`); err != nil || environment != "env" {
			t.Fatalf("snapshot: %s %v", environment, err)
		}
	}
	for _, id := range []string{"owner", "child"} {
		if id == "child" && len(p.RedundantEnvironments) == 0 {
			continue
		}
		var content, sessionID, turnID string
		if err := f.db.QueryRow(`SELECT content,task_session_id,turn_id FROM task_session_messages WHERE id=?`, id+"-message").Scan(&content, &sessionID, &turnID); err != nil {
			t.Fatal(err)
		}
		expectedSession := "session"
		if id == "child" {
			expectedSession = "child-session"
		}
		if content != "retained history for "+id || sessionID != expectedSession || turnID != id+"-turn" {
			t.Fatal("conversation history changed")
		}
	}
	assertLegacyCleanupEvidence(t, f)
	return f
}

func lifecycleServices(t *testing.T, f fixture) (*service.Service, *service.HandoffService, *gin.Engine) {
	t.Helper()
	repo, err := tasksqlite.NewWithDB(f.db, f.db, nil)
	if err != nil {
		t.Fatal(err)
	}
	groups, err := officesqlite.NewWithDB(f.db, f.db, nil)
	if err != nil {
		t.Fatal(err)
	}
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	svc := service.NewService(service.Repos{Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo, Messages: repo, Turns: repo, Sessions: repo, GitSnapshots: repo, RepoEntities: repo, Executors: repo, Environments: repo, TaskEnvironments: repo, Reviews: repo, ResourceCleanups: repo}, nil, log, service.RepositoryDiscoveryConfig{})
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		for _, query := range []string{
			"SELECT id,task_id,state,last_error FROM task_resource_cleanup_jobs",
			"SELECT id,cleanup_status,cleanup_error FROM task_workspace_groups",
			"SELECT id,state FROM task_sessions",
		} {
			rows, err := queryRows(context.Background(), f.db, query)
			t.Logf("%s: %+v %v", query, rows, err)
		}
	})
	manager := fixtureManager(t, f)
	svc.SetWorktreeCleanup(manager)
	handoff := service.NewHandoffService(repo, repo, nil, nil, groups, log)
	handoff.SetSessionReader(repo)
	svc.SetWorkspaceGroupMembershipReader(groups)
	svc.SetTaskLifecycleCoordinator(handoff)
	handoff.SetTaskResourceCleaner(svc)
	handoff.SetWorkspaceCleaner(worktree.NewHandoffCleaner(manager, log))
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := handlers.RegisterTaskRoutes(router, ws.NewDispatcher(), svc, nil, repo, nil, log)
	h.SetHandoffService(handoff)
	return svc, handoff, router
}

func lifecycleRequest(t *testing.T, router *gin.Engine, method, path, confirmation string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	req = req.WithContext(authn.WithIdentity(req.Context(), authn.Identity{UserID: "repair-test", Synthetic: true}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Kandev-Task-Delete-Confirmation", confirmation)
	result := httptest.NewRecorder()
	router.ServeHTTP(result, req)
	return result
}

func lifecycleDelete(t *testing.T, router *gin.Engine, task string, cascade, discard bool) {
	t.Helper()
	preview := lifecycleRequest(t, router, http.MethodPost, "/api/v1/tasks/delete-preflight", "", map[string]any{"task_ids": []string{task}, "cascade": cascade, "discard_worktree_changes": discard})
	if preview.Code != http.StatusOK {
		t.Fatalf("preflight %d: %s", preview.Code, preview.Body.String())
	}
	var consent service.TaskDeletePreflightResult
	if err := json.Unmarshal(preview.Body.Bytes(), &consent); err != nil {
		t.Fatal(err)
	}
	result := lifecycleRequest(t, router, http.MethodDelete, fmt.Sprintf("/api/v1/tasks/%s?cascade=%t&discard_worktree_changes=%t", task, cascade, discard), consent.ConfirmationID, nil)
	if cascade && result.Code == http.StatusServiceUnavailable {
		var pending struct {
			Pending bool   `json:"pending"`
			Success bool   `json:"success"`
			TaskID  string `json:"task_id"`
		}
		if err := json.Unmarshal(result.Body.Bytes(), &pending); err != nil || !pending.Pending || pending.Success || pending.TaskID != task {
			t.Fatalf("invalid post-commit result: %s %v", result.Body.String(), err)
		}
		return
	}
	if result.Code != http.StatusOK {
		t.Fatalf("delete %d: %s", result.Code, result.Body.String())
	}
}

func assertLegacyCleanupEvidence(t *testing.T, f fixture) {
	t.Helper()
	var evidence string
	if err := f.db.Get(&evidence, `SELECT resource_snapshot FROM task_resource_cleanup_jobs WHERE id='old-job'`); err != nil || evidence != legacyCleanupEvidence {
		t.Fatalf("old cleanup evidence changed: %s %v", evidence, err)
	}
}

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.7, AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.3
func TestLegacyAliasLifecycle(t *testing.T) {
	for _, operation := range []string{"archive", "delete"} {
		for _, selection := range []string{"same-root", "parent", "child", "cascade"} {
			t.Run(operation+"/"+selection, func(t *testing.T) {
				f := repairAndReopenLifecycle(t, lifecycleAliasFixture(t, selection != "same-root"))
				svc, handoff, router := lifecycleServices(t, f)
				task := "owner"
				if selection == "child" {
					task = "child"
				}
				cascade := selection == "cascade"
				if operation == "archive" {
					if _, err := handoff.ArchiveTaskTree(context.Background(), task, cascade); err != nil {
						t.Fatal(err)
					}
				} else {
					lifecycleDelete(t, router, task, cascade, false)
				}
				if err := svc.ResumeTaskResourceCleanupJobs(context.Background()); err != nil {
					t.Fatal(err)
				}
				retained := selection == "parent" || selection == "child" || (operation == "archive" && cascade)
				_, err := os.Stat(f.plan.Repairs[0].Path)
				if retained && err != nil {
					t.Fatalf("retained borrower's checkout removed: %v", err)
				}
				if !retained && !os.IsNotExist(err) {
					t.Fatalf("cleanup left checkout: %v", err)
				}
				var count int
				if operation == "archive" {
					if err := f.db.Get(&count, `SELECT COUNT(*) FROM tasks WHERE id=? AND archived_at IS NOT NULL`, task); err != nil || count != 1 {
						t.Fatalf("archive not persisted: %d %v", count, err)
					}
				} else if err := f.db.Get(&count, `SELECT COUNT(*) FROM tasks WHERE id=?`, task); err != nil || count != 0 {
					t.Fatalf("delete not persisted: %d %v", count, err)
				}
				if cascade {
					assertLifecycleCascade(t, f, operation)
				}
				assertLegacyCleanupEvidence(t, f)
			})
		}
	}
}

func seedLifecycleHistory(t *testing.T, f fixture, taskID, sessionID string) {
	t.Helper()
	execSQL(t, f.db, `INSERT INTO task_session_turns(id,task_session_id,task_id,started_at,completed_at,created_at,updated_at) VALUES(?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, taskID+"-turn", sessionID, taskID)
	execSQL(t, f.db, `INSERT INTO task_session_messages(id,task_session_id,task_id,turn_id,author_type,content,created_at,updated_at) VALUES(?,?,?,?,'agent',?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, taskID+"-message", sessionID, taskID, taskID+"-turn", "retained history for "+taskID)
}
