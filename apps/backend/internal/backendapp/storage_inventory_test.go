package backendapp

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	agentdocker "github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/system/storage/docknet"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestWorkspaceInventoryOnlyProtectsActiveTaskEnvironments(t *testing.T) {
	database := newContainerInventoryDatabase(t)
	insertContainerInventoryTask(t, database, "active", v1.TaskStateInProgress, false)
	insertContainerInventoryTask(t, database, "archived", v1.TaskStateInProgress, true)
	insertContainerInventoryTask(t, database, "borrowed-owner", v1.TaskStateInProgress, true)
	insertContainerInventoryTask(t, database, "borrower", v1.TaskStateInProgress, false)
	for _, item := range []struct {
		taskID string
		path   string
	}{
		{taskID: "active", path: "/tasks/active_abc"},
		{taskID: "archived", path: "/tasks/archived_def"},
		{taskID: "borrowed-owner", path: "/tasks/borrowed_jkl"},
		{taskID: "deleted", path: "/tasks/deleted_ghi"},
	} {
		if _, err := database.Exec(
			"INSERT INTO task_environments (id, task_id, status, workspace_path) VALUES (?, ?, ?, ?)",
			"env-"+item.taskID, item.taskID, models.TaskEnvironmentStatusReady, item.path,
		); err != nil {
			t.Fatalf("insert task environment for %q: %v", item.taskID, err)
		}
	}
	if _, err := database.Exec(
		"INSERT INTO task_sessions (id, task_id, state, task_environment_id) VALUES (?, ?, ?, ?)",
		"session-borrower", "borrower", models.TaskSessionStateRunning, "env-borrowed-owner",
	); err != nil {
		t.Fatalf("insert borrowed environment session: %v", err)
	}

	rows, err := (&storageInventory{reader: database}).activeWorkspaceRows(context.Background())
	if err != nil {
		t.Fatalf("activeWorkspaceRows: %v", err)
	}
	want := []activeWorkspaceRow{
		{TaskID: "active", WorkspaceID: "workspace-active", WorkspacePath: "/tasks/active_abc"},
		{
			TaskID: "borrowed-owner", WorkspaceID: "workspace-borrowed-owner",
			WorkspacePath: "/tasks/borrowed_jkl",
		},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("active workspace rows = %#v, want %#v", rows, want)
	}
}

func TestWorkspaceInventoryOnlyProtectsActiveTaskWorktrees(t *testing.T) {
	database := newContainerInventoryDatabase(t)
	insertContainerInventoryTask(t, database, "active", v1.TaskStateInProgress, false)
	insertContainerInventoryTask(t, database, "archived", v1.TaskStateInProgress, true)
	for _, item := range []struct {
		taskID string
		path   string
	}{
		{taskID: "active", path: "/tasks/active_abc/repo"},
		{taskID: "archived", path: "/tasks/archived_def/repo"},
		{taskID: "deleted", path: "/tasks/deleted_ghi/repo"},
	} {
		if _, err := database.Exec(
			"INSERT INTO task_environments (id, task_id, status) VALUES (?, ?, 'ready')",
			"env-"+item.taskID, item.taskID,
		); err != nil {
			t.Fatalf("insert task environment for %q: %v", item.taskID, err)
		}
		if _, err := database.Exec(
			"INSERT INTO task_environment_repos (id, task_environment_id, status, worktree_path) VALUES (?, ?, 'active', ?)",
			"worktree-"+item.taskID, "env-"+item.taskID, item.path,
		); err != nil {
			t.Fatalf("insert task worktree for %q: %v", item.taskID, err)
		}
	}

	paths, err := (&storageInventory{reader: database}).activeWorktreePaths(context.Background())
	if err != nil {
		t.Fatalf("activeWorktreePaths: %v", err)
	}
	want := []string{"/tasks/active_abc/repo"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("active worktree paths = %#v, want %#v", paths, want)
	}
}

func TestWorkspaceInventoryLoadsTaskEligibilityStates(t *testing.T) {
	t.Run("active and archived tasks", func(t *testing.T) {
		database := newContainerInventoryDatabase(t)
		insertContainerInventoryTask(t, database, "active", v1.TaskStateInProgress, false)
		insertContainerInventoryTask(t, database, "archived", v1.TaskStateCompleted, true)

		states, err := (&storageInventory{reader: database}).taskWorkspaceStates(context.Background())
		if err != nil {
			t.Fatalf("taskWorkspaceStates: %v", err)
		}
		known, archived := taskEligibilitySets(states)
		if !reflect.DeepEqual(known, map[string]struct{}{"active": {}, "archived": {}}) {
			t.Fatalf("known task IDs = %#v", known)
		}
		if !reflect.DeepEqual(archived, map[string]struct{}{"archived": {}}) {
			t.Fatalf("archived task IDs = %#v", archived)
		}
	})

	t.Run("empty database", func(t *testing.T) {
		database := newContainerInventoryDatabase(t)
		states, err := (&storageInventory{reader: database}).taskWorkspaceStates(context.Background())
		if err != nil {
			t.Fatalf("taskWorkspaceStates: %v", err)
		}
		known, archived := taskEligibilitySets(states)
		if len(states) != 0 || len(known) != 0 || len(archived) != 0 {
			t.Fatalf("empty task eligibility = states %#v known %#v archived %#v", states, known, archived)
		}
	})

	t.Run("query error", func(t *testing.T) {
		database := newContainerInventoryDatabase(t)
		if err := database.Close(); err != nil {
			t.Fatalf("close inventory database: %v", err)
		}
		if _, err := (&storageInventory{reader: database}).taskWorkspaceStates(context.Background()); err == nil {
			t.Fatal("taskWorkspaceStates error = nil, want closed-database error")
		}
	})
}

func TestContainerInventoryRemovability(t *testing.T) {
	tests := []struct {
		name   string
		taskID string
		setup  func(*testing.T, *sqlx.DB)
		want   bool
	}{
		{
			name:   "missing task is removable",
			taskID: "missing",
			want:   true,
		},
		{
			name:   "nonterminal task with stopped environment is retained",
			taskID: "active-stopped-env",
			setup: func(t *testing.T, database *sqlx.DB) {
				insertContainerInventoryTask(t, database, "active-stopped-env", v1.TaskStateInProgress, false)
				insertContainerInventoryEnvironment(t, database, "active-stopped-env", models.TaskEnvironmentStatusStopped)
			},
		},
		{
			name:   "archived task without live handles is removable",
			taskID: "archived",
			setup: func(t *testing.T, database *sqlx.DB) {
				insertContainerInventoryTask(t, database, "archived", v1.TaskStateInProgress, true)
			},
			want: true,
		},
		{
			name:   "terminal task without live handles is removable",
			taskID: "terminal",
			setup: func(t *testing.T, database *sqlx.DB) {
				insertContainerInventoryTask(t, database, "terminal", v1.TaskStateCompleted, false)
			},
			want: true,
		},
		{
			name:   "terminal task with live environment is retained",
			taskID: "terminal-live-env",
			setup: func(t *testing.T, database *sqlx.DB) {
				insertContainerInventoryTask(t, database, "terminal-live-env", v1.TaskStateCompleted, false)
				insertContainerInventoryEnvironment(t, database, "terminal-live-env", models.TaskEnvironmentStatusReady)
			},
		},
		{
			name:   "terminal task with unknown environment state is retained",
			taskID: "terminal-unknown-env",
			setup: func(t *testing.T, database *sqlx.DB) {
				insertContainerInventoryTask(t, database, "terminal-unknown-env", v1.TaskStateCompleted, false)
				insertContainerInventoryEnvironment(t, database, "terminal-unknown-env", "new-state")
			},
		},
		{
			name:   "archived task with live executor handle is retained",
			taskID: "archived-live-executor",
			setup: func(t *testing.T, database *sqlx.DB) {
				insertContainerInventoryTask(t, database, "archived-live-executor", v1.TaskStateCompleted, true)
				insertContainerInventoryExecutor(t, database, "archived-live-executor", models.ExecutorRunningStatusRunning)
			},
		},
		{
			name:   "terminal task with stopped executor is removable",
			taskID: "terminal-stopped-executor",
			setup: func(t *testing.T, database *sqlx.DB) {
				insertContainerInventoryTask(t, database, "terminal-stopped-executor", v1.TaskStateFailed, false)
				insertContainerInventoryExecutor(t, database, "terminal-stopped-executor", models.ExecutorRunningStatusStopped)
			},
			want: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			database := newContainerInventoryDatabase(t)
			if test.setup != nil {
				test.setup(t, database)
			}

			got, err := (&containerInventory{reader: database}).ContainerTaskRemovable(
				context.Background(), test.taskID,
			)
			if err != nil {
				t.Fatalf("ContainerTaskRemovable: %v", err)
			}
			if got != test.want {
				t.Fatalf("ContainerTaskRemovable = %v, want %v", got, test.want)
			}
		})
	}
}

func TestContainerInventoryFailsClosedWhenInventoryUnavailable(t *testing.T) {
	database := newContainerInventoryDatabase(t)
	insertContainerInventoryTask(t, database, "inventory-error", v1.TaskStateCompleted, false)
	if err := database.Close(); err != nil {
		t.Fatalf("close inventory database: %v", err)
	}

	removable, err := (&containerInventory{reader: database}).ContainerTaskRemovable(
		context.Background(), "inventory-error",
	)
	if err == nil {
		t.Fatal("ContainerTaskRemovable error = nil, want inventory error")
	}
	if removable {
		t.Fatal("ContainerTaskRemovable = true on inventory error, want fail-closed false")
	}
}

func newContainerInventoryDatabase(t *testing.T) *sqlx.DB {
	t.Helper()
	database, err := sqlx.Open("sqlite3", filepath.Join(t.TempDir(), "inventory.db"))
	if err != nil {
		t.Fatalf("open inventory database: %v", err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = database.Close() })
	const schema = `
		CREATE TABLE tasks (
			id TEXT PRIMARY KEY,
			state TEXT NOT NULL,
			workspace_id TEXT NOT NULL DEFAULT '',
			archived_at TIMESTAMP
		);
		CREATE TABLE task_environments (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			status TEXT NOT NULL,
			workspace_path TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE task_sessions (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			state TEXT NOT NULL DEFAULT 'CREATED',
			task_environment_id TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE task_environment_repos (
			id TEXT PRIMARY KEY,
			task_environment_id TEXT NOT NULL,
			status TEXT NOT NULL,
			worktree_path TEXT NOT NULL DEFAULT '',
			deleted_at TIMESTAMP
		);
		CREATE TABLE executors_running (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			status TEXT NOT NULL,
			runtime TEXT NOT NULL
		);`
	if _, err := database.Exec(schema); err != nil {
		t.Fatalf("create inventory schema: %v", err)
	}
	return database
}

func insertContainerInventoryTask(
	t *testing.T,
	database *sqlx.DB,
	taskID string,
	state v1.TaskState,
	archived bool,
) {
	t.Helper()
	var archivedAt any
	if archived {
		archivedAt = time.Now().UTC()
	}
	if _, err := database.Exec(
		"INSERT INTO tasks (id, state, workspace_id, archived_at) VALUES (?, ?, ?, ?)",
		taskID, state, "workspace-"+taskID, archivedAt,
	); err != nil {
		t.Fatalf("insert task %q: %v", taskID, err)
	}
}

func insertContainerInventoryEnvironment(
	t *testing.T,
	database *sqlx.DB,
	taskID string,
	status models.TaskEnvironmentStatus,
) {
	t.Helper()
	if _, err := database.Exec(
		"INSERT INTO task_environments (id, task_id, status) VALUES (?, ?, ?)",
		"env-"+taskID, taskID, status,
	); err != nil {
		t.Fatalf("insert task environment for %q: %v", taskID, err)
	}
}

func insertContainerInventoryExecutor(
	t *testing.T,
	database *sqlx.DB,
	taskID string,
	status string,
) {
	t.Helper()
	if _, err := database.Exec(
		"INSERT INTO executors_running (id, task_id, status, runtime) VALUES (?, ?, ?, ?)",
		"executor-"+taskID, taskID, status, "docker",
	); err != nil {
		t.Fatalf("insert executor handle for %q: %v", taskID, err)
	}
}

func TestNetworkTaskOracleResolvesKandevTaskLabels(t *testing.T) {
	database := newContainerInventoryDatabase(t)
	insertContainerInventoryTask(t, database, "11111111-1111-4111-8111-111111111111", v1.TaskStateInProgress, false)
	insertContainerInventoryTask(t, database, "22222222-2222-4222-8222-222222222222", v1.TaskStateCompleted, true)
	oracle := &networkTaskOracle{reader: database}

	active := oracleLookup(t, oracle, "11111111-1111-4111-8111-111111111111")
	if active != docknet.TaskLookupActive {
		t.Fatalf("active task lookup = %v, want active", active)
	}
	inactive := oracleLookup(t, oracle, "22222222-2222-4222-8222-222222222222")
	if inactive != docknet.TaskLookupInactive {
		t.Fatalf("archived task lookup = %v, want inactive", inactive)
	}
	// A kandev-labeled network whose task row is gone is an orphan (owned
	// once, finished now): the classifier reclaims it after the grace window.
	deleted := oracleLookup(t, oracle, "99999999-9999-4999-8999-999999999999")
	if deleted != docknet.TaskLookupInactive {
		t.Fatalf("deleted task lookup = %v, want inactive (orphan)", deleted)
	}
}

func TestNetworkTaskOracleResolvesComposeProjectNames(t *testing.T) {
	database := newContainerInventoryDatabase(t)
	taskID := "33333333-3333-4333-8333-333333333333"
	insertContainerInventoryTask(t, database, taskID, v1.TaskStateInProgress, false)
	oracle := &networkTaskOracle{reader: database}

	// Exact project form: kd_<task id>.
	lookup := oracleLookup(t, oracle, "kd_"+taskID)
	if lookup != docknet.TaskLookupActive {
		t.Fatalf("compose project lookup = %v, want active", lookup)
	}
	// Unknown project names stay unknown (fail-closed observation).
	unknown := oracleLookup(t, oracle, "kd_nope")
	if unknown != docknet.TaskLookupUnknown {
		t.Fatalf("unknown project lookup = %v, want unknown", unknown)
	}
	// Non-kd_ project names are third-party and never resolved.
	foreign := oracleLookup(t, oracle, "myapp_default")
	if foreign != docknet.TaskLookupUnknown {
		t.Fatalf("foreign project lookup = %v, want unknown", foreign)
	}
}

func TestNetworkTaskOracleOwnershipKeyResolution(t *testing.T) {
	database := newContainerInventoryDatabase(t)
	taskID := "44444444-4444-4444-8444-444444444444"
	insertContainerInventoryTask(t, database, taskID, v1.TaskStateCompleted, true)
	oracle := &networkTaskOracle{reader: database}

	kandevLabeled := agentdocker.NetworkInfo{
		ID: "n1", Name: "kd_x_default", Driver: "bridge",
		Labels: map[string]string{"kandev.task_id": taskID},
	}
	key, lookup, err := oracle.Ownership(kandevLabeled)
	if err != nil {
		t.Fatalf("Ownership: %v", err)
	}
	if key != taskID || lookup != docknet.TaskLookupInactive {
		t.Fatalf("kandev label lookup = key %q %v, want inactive task", key, lookup)
	}

	composeLabeled := agentdocker.NetworkInfo{
		ID: "n2", Name: "kd_y_default", Driver: "bridge",
		Labels: map[string]string{"com.docker.compose.project": "kd_" + taskID},
	}
	key, lookup, err = oracle.Ownership(composeLabeled)
	if err != nil {
		t.Fatalf("Ownership compose: %v", err)
	}
	if key != "kd_"+taskID || lookup != docknet.TaskLookupInactive {
		t.Fatalf("compose label lookup = key %q %v, want inactive task", key, lookup)
	}

	// Label-less networks never consult the task store.
	_, lookup, err = oracle.Ownership(agentdocker.NetworkInfo{ID: "n3", Name: "anon", Labels: map[string]string{}})
	if err != nil || lookup != docknet.TaskLookupUnknown {
		t.Fatalf("label-free lookup = %v (%v), want unknown without error", lookup, err)
	}
}

func oracleLookup(t *testing.T, oracle *networkTaskOracle, key string) docknet.TaskLookup {
	t.Helper()
	network := agentdocker.NetworkInfo{Labels: map[string]string{"kandev.task_id": key}}
	if !isUUID(key) {
		network = agentdocker.NetworkInfo{Labels: map[string]string{"com.docker.compose.project": key}}
	}
	_, lookup, err := oracle.Ownership(network)
	if err != nil {
		t.Fatalf("Ownership for %q: %v", key, err)
	}
	return lookup
}
