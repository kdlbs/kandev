package sqlite_test

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	internaldb "github.com/kandev/kandev/internal/db"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

func TestTaskHierarchyAdmissionOfficeSerialization(t *testing.T) {
	first := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	tasks, err := tasksqlite.NewWithDB(first, first, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := settingsstore.Provide(first, first, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := officesqlite.NewWithDB(first, first, nil); err != nil {
		t.Fatal(err)
	}
	var schema string
	if err := first.QueryRow(`SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	raw, err := internaldb.OpenPostgres(officeHierarchySchemaDSN(testutil.PostgresDSNFromEnv(t), schema), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	second := sqlx.NewDb(raw, "pgx")
	t.Cleanup(func() { _ = second.Close() })
	var initialPID int
	if err := second.QueryRow(`SELECT pg_backend_pid()`).Scan(&initialPID); err != nil {
		t.Fatal(err)
	}
	second.SetMaxIdleConns(0)
	var renewedSchema string
	var renewedPID int
	if err := second.QueryRow(`SELECT current_schema(), pg_backend_pid()`).Scan(&renewedSchema, &renewedPID); err != nil {
		t.Fatal(err)
	}
	second.SetMaxIdleConns(1)
	if renewedPID == initialPID || renewedSchema != schema {
		t.Fatalf("renewed connection PID %d (was %d), schema %q, want isolated %q", renewedPID, initialPID, renewedSchema, schema)
	}
	office, err := officesqlite.NewWithDB(second, second, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := tasks.CreateWorkspace(ctx, &models.Workspace{ID: "office-ws", Name: "Office"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"parent", "child"} {
		if err := tasks.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "office-ws", Title: id}); err != nil {
			t.Fatal(err)
		}
	}
	var pid int
	if err := second.QueryRow(`SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	holder, err := first.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Rollback() }()
	if _, err := holder.Exec(`SELECT id FROM workspaces WHERE id='office-ws' FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- office.UpdateTaskParentID(ctx, "child", "parent") }()
	joined := false
	defer func() {
		_ = holder.Rollback()
		if !joined {
			<-result
		}
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting bool
		if err := holder.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_locks WHERE pid=$1 AND NOT granted AND locktype='transactionid')`, pid).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Office scalar did not physically wait on workspace")
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Logf("Office scalar physical workspace wait PID %d", pid)
	if _, err := holder.Exec(`DELETE FROM tasks WHERE id='parent'`); err != nil {
		t.Fatal(err)
	}
	if err := holder.Commit(); err != nil {
		t.Fatal(err)
	}
	err = <-result
	joined = true
	if !errors.Is(err, officesqlite.ErrTaskNotFound) {
		t.Fatalf("deleted target not rechecked: %v", err)
	}
	child, err := tasks.GetTask(ctx, "child")
	if err != nil {
		t.Fatal(err)
	}
	if child.ParentID != "" {
		t.Fatalf("dangling Office parent %q", child.ParentID)
	}
	if err := tasks.CreateTask(ctx, &models.Task{ID: "replacement", WorkspaceID: "office-ws", Title: "Replacement"}); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Exec(`UPDATE tasks SET metadata='{"workspace":{"mode":"inherit_parent","group_id":"office-group"},"keep":"current"}' WHERE id='child'`); err != nil {
		t.Fatal(err)
	}
	if err := office.UpdateTaskParentID(ctx, "child", "replacement"); err != nil {
		t.Fatal(err)
	}
	child, err = tasks.GetTask(ctx, "child")
	if err != nil {
		t.Fatal(err)
	}
	workspace, _ := child.Metadata["workspace"].(map[string]interface{})
	if child.ParentID != "replacement" || workspace["mode"] != "shared_group" || workspace["group_id"] != "office-group" || child.Metadata["keep"] != "current" {
		t.Fatalf("committed Office normalization=%+v", child)
	}
	if _, err := first.Exec(`UPDATE tasks SET metadata='{"workspace":{"mode":"inherit_parent","group_id":"office-group"}}' WHERE id='child'`); err != nil {
		t.Fatal(err)
	}
	if err := office.UpdateTaskParentID(ctx, "child", "replacement"); err != nil {
		t.Fatal(err)
	}
	child, err = tasks.GetTask(ctx, "child")
	if err != nil {
		t.Fatal(err)
	}
	workspace, _ = child.Metadata["workspace"].(map[string]interface{})
	if workspace["mode"] != "inherit_parent" {
		t.Fatal("same parent normalized workspace")
	}
	if _, err := first.Exec(`UPDATE tasks SET metadata='{' WHERE id='child'`); err != nil {
		t.Fatal(err)
	}
	if err := office.UpdateTaskParentID(ctx, "child", ""); err != nil {
		t.Fatal(err)
	}
	var metadata, parent string
	if err := first.QueryRow(`SELECT metadata, parent_id FROM tasks WHERE id='child'`).Scan(&metadata, &parent); err != nil {
		t.Fatal(err)
	}
	if metadata != "{" || parent != "" {
		t.Fatalf("invalid metadata control: metadata=%q parent=%q", metadata, parent)
	}
}

func officeHierarchySchemaDSN(dsn, schema string) string {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		parsed, err := url.Parse(dsn)
		if err != nil {
			panic(err)
		}
		query := parsed.Query()
		query.Set("search_path", schema)
		parsed.RawQuery = query.Encode()
		return parsed.String()
	}
	return dsn + " search_path=" + schema
}
