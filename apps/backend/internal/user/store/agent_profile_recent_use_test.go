package store

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	commonlogger "github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/user/models"
)

func TestSQLiteRepositoryCreatesAgentProfileRecentUseTable(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	repo, cleanup, err := Provide(conn, conn)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}
	_ = repo
	t.Cleanup(func() { _ = cleanup() })

	var tableName string
	err = conn.GetContext(context.Background(), &tableName, `
		SELECT name FROM sqlite_master
		WHERE type = 'table' AND name = 'user_agent_profile_recent_use'
	`)
	if err != nil {
		t.Fatalf("find recent-use table: %v", err)
	}
	if tableName != "user_agent_profile_recent_use" {
		t.Fatalf("table name = %q, want recent-use table", tableName)
	}
}

func TestSQLiteRepositoryPersistsAgentProfileRecentUseWithCAS(t *testing.T) {
	conn := openRecentUseSQLite(t)
	repo, cleanup, err := Provide(conn, conn)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })

	ctx := context.Background()
	record := &models.AgentProfileRecentUse{
		UserID:     DefaultUserID,
		Context:    models.AgentProfileRecentUseTaskSession,
		ProfileIDs: []string{"profile-a", "profile-b"},
		Revision:   1,
		UpdatedAt:  time.Now().UTC(),
	}
	if _, err := repo.UpsertAgentProfileRecentUse(ctx, record, 0); err != nil {
		t.Fatalf("insert recent-use record: %v", err)
	}
	if _, err := repo.UpsertAgentProfileRecentUse(ctx, &models.AgentProfileRecentUse{
		UserID:     DefaultUserID,
		Context:    models.AgentProfileRecentUseTaskSession,
		ProfileIDs: []string{"profile-c"},
		Revision:   1,
		UpdatedAt:  time.Now().UTC(),
	}, 0); err != ErrAgentProfileRecentUseRevisionConflict {
		t.Fatalf("stale upsert error = %v, want revision conflict", err)
	}
	updated, err := repo.UpsertAgentProfileRecentUse(ctx, &models.AgentProfileRecentUse{
		UserID:     DefaultUserID,
		Context:    models.AgentProfileRecentUseTaskSession,
		ProfileIDs: []string{"profile-c"},
		Revision:   2,
		UpdatedAt:  time.Now().UTC(),
	}, 1)
	if err != nil {
		t.Fatalf("conditional update: %v", err)
	}
	if updated.Revision != 2 || len(updated.ProfileIDs) != 1 || updated.ProfileIDs[0] != "profile-c" {
		t.Fatalf("updated record = %+v", updated)
	}
}

func TestSQLiteRepositoryRecentUseMigrationReplayAndCascade(t *testing.T) {
	conn := openRecentUseSQLite(t)
	repo, cleanup, err := Provide(conn, conn)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	ctx := context.Background()
	if _, err := repo.UpsertAgentProfileRecentUse(ctx, &models.AgentProfileRecentUse{
		UserID:     DefaultUserID,
		Context:    models.AgentProfileRecentUseQuickChat,
		ProfileIDs: []string{"profile-a"},
		Revision:   1,
		UpdatedAt:  time.Now().UTC(),
	}, 0); err != nil {
		t.Fatalf("insert record: %v", err)
	}
	if _, err := newSQLiteRepositoryWithDB(conn, conn); err != nil {
		t.Fatalf("replay schema: %v", err)
	}
	if err := conn.GetContext(ctx, new(int), `SELECT COUNT(1) FROM user_agent_profile_recent_use`); err != nil {
		t.Fatalf("query replayed table: %v", err)
	}
	if _, err := conn.Exec(`DELETE FROM users WHERE id = ?`, DefaultUserID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	var count int
	if err := conn.GetContext(ctx, &count, `SELECT COUNT(1) FROM user_agent_profile_recent_use`); err != nil {
		t.Fatalf("count cascaded rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("recent-use rows after user deletion = %d, want 0", count)
	}
}

func TestSQLiteRepositoryRejectsInvalidRecentUseJSON(t *testing.T) {
	conn := openRecentUseSQLite(t)
	repo, cleanup, err := Provide(conn, conn)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	if _, err := conn.Exec(`INSERT INTO user_agent_profile_recent_use
		(user_id, context, profile_ids, revision, updated_at) VALUES (?, ?, ?, ?, ?)`,
		DefaultUserID, models.AgentProfileRecentUseConfigChat, "{}", 1, time.Now().UTC()); err != nil {
		t.Fatalf("insert invalid JSON: %v", err)
	}
	if _, err := repo.GetAgentProfileRecentUse(context.Background(), DefaultUserID, models.AgentProfileRecentUseConfigChat); err == nil {
		t.Fatal("invalid profile_ids JSON was accepted")
	}
}

func TestSQLiteRepositoryListSkipsMalformedContextRows(t *testing.T) {
	conn := openRecentUseSQLite(t)
	repo, cleanup, err := Provide(conn, conn)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	core, logs := observer.New(zap.WarnLevel)
	log, err := commonlogger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observer logger: %v", err)
	}
	repo.SetAgentProfileRecentUseLogger(log)
	ctx := context.Background()
	if _, err := repo.UpsertAgentProfileRecentUse(ctx, &models.AgentProfileRecentUse{
		UserID:     DefaultUserID,
		Context:    models.AgentProfileRecentUseQuickChat,
		ProfileIDs: []string{"profile-valid"},
		Revision:   1,
		UpdatedAt:  time.Now().UTC(),
	}, 0); err != nil {
		t.Fatalf("insert valid record: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO user_agent_profile_recent_use
		(user_id, context, profile_ids, revision, updated_at) VALUES (?, ?, ?, ?, ?)`,
		DefaultUserID, models.AgentProfileRecentUseConfigChat, "{}", 1, time.Now().UTC()); err != nil {
		t.Fatalf("insert malformed record: %v", err)
	}

	records, err := repo.ListAgentProfileRecentUse(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("list records: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("listed records = %d, want one valid context", len(records))
	}
	if records[0].Context != models.AgentProfileRecentUseQuickChat || records[0].ProfileIDs[0] != "profile-valid" {
		t.Fatalf("listed record = %+v, want valid quick-chat context", records[0])
	}
	entries := logs.FilterMessage("skipping malformed agent profile recent-use row").All()
	if len(entries) != 1 {
		t.Fatalf("malformed-row diagnostics = %d, want one", len(entries))
	}
	if got := entries[0].ContextMap()["context"]; got != string(models.AgentProfileRecentUseConfigChat) {
		t.Fatalf("malformed-row context = %v, want %q", got, models.AgentProfileRecentUseConfigChat)
	}
}

func openRecentUseSQLite(t *testing.T) *sqlx.DB {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	if _, err := conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}
