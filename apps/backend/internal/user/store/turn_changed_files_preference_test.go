package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/testutil"
)

func TestTurnChangedFilesPreferenceScanAndMarshal(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
		want bool
	}{
		{name: "missing defaults on", raw: `{"unrelated":true}`, want: true},
		{name: "explicit false survives", raw: `{"show_turn_changed_files":false}`, want: false},
		{name: "explicit true survives", raw: `{"show_turn_changed_files":true}`, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			settings, err := scanUserSettings(settingsScanner{raw: test.raw}, DefaultUserID)
			if err != nil {
				t.Fatalf("scan settings: %v", err)
			}
			encoded, err := marshalUserSettingsPayload(settings)
			if err != nil {
				t.Fatalf("marshal settings: %v", err)
			}
			var payload map[string]any
			if err := json.Unmarshal(encoded, &payload); err != nil {
				t.Fatalf("decode settings: %v", err)
			}
			if got, ok := payload["show_turn_changed_files"].(bool); !ok || got != test.want {
				t.Fatalf("show_turn_changed_files = %#v, want %t in %s", payload["show_turn_changed_files"], test.want, encoded)
			}
		})
	}
}

func TestTurnChangedFilesPreferenceMigration(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	assertTurnChangedFilesPreferenceMigration(t, conn)
}

func TestTurnChangedFilesPreferencePostgresMigration(t *testing.T) {
	conn := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	assertTurnChangedFilesPreferenceMigration(t, conn)
}

func assertTurnChangedFilesPreferenceMigration(t *testing.T, conn *sqlx.DB) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := conn.Exec(`CREATE TABLE users (
		id TEXT PRIMARY KEY,
		email TEXT NOT NULL,
		settings TEXT NOT NULL DEFAULT '{}',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	)`); err != nil {
		t.Fatalf("create legacy users table: %v", err)
	}
	for _, user := range []struct {
		id       string
		settings string
	}{
		{id: DefaultUserID, settings: `{"unknown_key":"kept"}`},
		{id: "member-with-disabled-history", settings: `{"show_turn_changed_files":false}`},
	} {
		_, err := conn.Exec(conn.Rebind(`INSERT INTO users (id, email, settings, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`),
			user.id, user.id+"@example.com", user.settings, now, now)
		if err != nil {
			t.Fatalf("insert legacy user %s: %v", user.id, err)
		}
	}

	repo, err := newSQLiteRepositoryWithDB(conn, conn)
	if err != nil {
		t.Fatalf("initialize migrated repository: %v", err)
	}
	assertPreferenceValue := func(userID string, want bool) int64 {
		t.Helper()
		var raw string
		var revision int64
		if err := conn.QueryRow(conn.Rebind(`SELECT settings, settings_revision FROM users WHERE id = ?`), userID).Scan(&raw, &revision); err != nil {
			t.Fatalf("read migrated settings for %s: %v", userID, err)
		}
		var settings map[string]any
		if err := json.Unmarshal([]byte(raw), &settings); err != nil {
			t.Fatalf("decode migrated settings for %s: %v", userID, err)
		}
		if got, ok := settings["show_turn_changed_files"].(bool); !ok || got != want {
			t.Fatalf("%s show_turn_changed_files = %#v, want %t (settings %s)", userID, settings["show_turn_changed_files"], want, raw)
		}
		return revision
	}
	legacyRevision := assertPreferenceValue(DefaultUserID, true)
	assertPreferenceValue("member-with-disabled-history", false)
	var legacyRaw string
	if err := conn.Get(&legacyRaw, conn.Rebind(`SELECT settings FROM users WHERE id = ?`), DefaultUserID); err != nil {
		t.Fatalf("read legacy settings: %v", err)
	}
	var legacyFields map[string]any
	if err := json.Unmarshal([]byte(legacyRaw), &legacyFields); err != nil {
		t.Fatal(err)
	}
	if legacyFields["unknown_key"] != "kept" {
		t.Fatalf("unrelated settings field was lost: %s", legacyRaw)
	}
	settings, err := repo.GetUserSettings(context.Background(), "member-with-disabled-history")
	if err != nil {
		t.Fatalf("read settings before unrelated update: %v", err)
	}
	settings.UnreadDivider = true
	updated, err := repo.UpsertUserSettingsPreservingTaskCreateLastUsed(
		context.Background(), settings, nil, settings.Revision,
	)
	if err != nil {
		t.Fatalf("save unrelated preference: %v", err)
	}
	if updated.ShowTurnChangedFiles || !updated.UnreadDivider {
		t.Fatalf("unrelated save changed turn preference: changed=%t unread=%t", updated.ShowTurnChangedFiles, updated.UnreadDivider)
	}
	disabledRevision := updated.Revision

	if _, err := newSQLiteRepositoryWithDB(conn, conn); err != nil {
		t.Fatalf("replay user settings migrations: %v", err)
	}
	if got := assertPreferenceValue(DefaultUserID, true); got != legacyRevision {
		t.Fatalf("legacy revision after replay = %d, want %d", got, legacyRevision)
	}
	if got := assertPreferenceValue("member-with-disabled-history", false); got != disabledRevision {
		t.Fatalf("disabled preference revision after replay = %d, want %d", got, disabledRevision)
	}
	reopened, err := newSQLiteRepositoryWithDB(conn, conn)
	if err != nil {
		t.Fatalf("reopen migrated repository: %v", err)
	}
	if got, err := reopened.GetUserSettings(context.Background(), "member-with-disabled-history"); err != nil {
		t.Fatalf("read settings after restart: %v", err)
	} else if got.ShowTurnChangedFiles || !got.UnreadDivider {
		t.Fatalf("settings after restart: changed=%t unread=%t", got.ShowTurnChangedFiles, got.UnreadDivider)
	}
}
