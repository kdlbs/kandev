package store

import (
	"context"
	"encoding/json"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/kandev/kandev/internal/user/models"
	"testing"
	"time"
)

// @covers AC-UI-NAV-HIERARCHY-004.2, AC-UI-NAV-HIERARCHY-004.3
func TestSidebarPresentationUpgradeAndReopen(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	assertSidebarPresentationUpgradeAndReopen(t, conn)
}

func TestSidebarPresentationPostgresUpgradeAndReopen(t *testing.T) {
	conn := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	assertSidebarPresentationUpgradeAndReopen(t, conn)
}

func assertSidebarPresentationUpgradeAndReopen(t *testing.T, conn *sqlx.DB) {
	t.Helper()
	_, err := newSQLiteRepositoryWithDB(conn, conn)
	if err != nil {
		t.Fatal(err)
	}
	var initial string
	if err = conn.Get(&initial, "SELECT settings FROM users WHERE id = 'default-user'"); err != nil {
		t.Fatal(err)
	}
	var values map[string]any
	if err = json.Unmarshal([]byte(initial), &values); err != nil {
		t.Fatal(err)
	}
	if values["sidebar_fast_actions_enabled"] != false || values["sidebar_new_task_style"] != "simple" || values["show_turn_changed_files"] != true {
		t.Fatalf("new user insert: %s", initial)
	}
	if _, err = conn.Exec(`UPDATE users SET settings = '{"unknown_key":17,"sidebar_fast_actions_enabled":false}', settings_revision=8`); err != nil {
		t.Fatal(err)
	}
	repo, err := newSQLiteRepositoryWithDB(conn, conn)
	if err != nil {
		t.Fatal(err)
	}
	var raw string
	var revision int64
	if err = conn.QueryRow("SELECT settings, settings_revision FROM users WHERE id = 'default-user'").Scan(&raw, &revision); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal([]byte(raw), &values); err != nil {
		t.Fatal(err)
	}
	if values["sidebar_fast_actions_enabled"] != false || values["sidebar_new_task_style"] != "compact" || values["show_turn_changed_files"] != true || values["unknown_key"] != float64(17) || revision != 10 {
		t.Fatalf("backfill: %s revision %d", raw, revision)
	}
	if _, err = newSQLiteRepositoryWithDB(conn, conn); err != nil {
		t.Fatal(err)
	}
	var next int64
	_ = conn.Get(&next, "SELECT settings_revision FROM users WHERE id = 'default-user'")
	if next != revision {
		t.Fatal("replay changed preferences")
	}
	user := &models.User{ID: "new-account", Email: "new@example.com", Role: models.RoleAdmin, Status: models.StatusActive, CreatedAt: time.Now()}
	if err = repo.CreateUser(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	if _, err = newSQLiteRepositoryWithDB(conn, conn); err != nil {
		t.Fatal(err)
	}
	if err = conn.Get(&raw, "SELECT settings FROM users WHERE id = 'new-account'"); err != nil {
		t.Fatal(err)
	}
	values = nil
	_ = json.Unmarshal([]byte(raw), &values)
	if values["sidebar_fast_actions_enabled"] != false || values["sidebar_new_task_style"] != "simple" || values["show_turn_changed_files"] != true {
		t.Fatalf("new account after reopen: %s", raw)
	}
}

func TestSidebarPresentationBackfillPreservesExplicitChoices(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		fast    bool
		style   string
		changed bool
	}{
		{`{}`, true, "compact", true},
		{`{"sidebar_fast_actions_enabled":false}`, false, "compact", true},
		{`{"sidebar_new_task_style":"simple"}`, true, "simple", true},
		{`{"sidebar_fast_actions_enabled":false,"sidebar_new_task_style":"simple","future_key":17}`, false, "simple", false},
	} {
		raw, changed, err := backfillSidebarPresentation(tc.raw)
		if err != nil || changed != tc.changed {
			t.Fatalf("%s: changed=%v error=%v", tc.raw, changed, err)
		}
		if !changed {
			raw = []byte(tc.raw)
		}
		var fields map[string]any
		if err = json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		if fields["sidebar_fast_actions_enabled"] != tc.fast || fields["sidebar_new_task_style"] != tc.style {
			t.Fatalf("%s", raw)
		}
	}
}

func TestSidebarPresentationMigrationRollsBackMalformedSettings(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	repo, err := newSQLiteRepositoryWithDB(conn, conn)
	if err != nil {
		t.Fatal(err)
	}
	user := &models.User{ID: "broken", Email: "broken@example.com", Role: models.RoleAdmin, Status: models.StatusActive, CreatedAt: time.Now()}
	if err = repo.CreateUser(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(`UPDATE users SET settings='{}', settings_revision=8 WHERE id='default-user'`); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(`UPDATE users SET settings='[]' WHERE id='broken'`); err != nil {
		t.Fatal(err)
	}
	if err = repo.migrateSidebarPresentation(); err == nil {
		t.Fatal("malformed account must abort migration")
	}
	var raw string
	var revision int64
	if err = conn.QueryRow("SELECT settings, settings_revision FROM users WHERE id='default-user'").Scan(&raw, &revision); err != nil {
		t.Fatal(err)
	}
	if raw != "{}" || revision != 8 {
		t.Fatalf("partial migration: %s revision=%d", raw, revision)
	}
}
