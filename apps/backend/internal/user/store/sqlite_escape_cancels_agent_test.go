package store

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/user/models"
)

func TestScanUserSettingsEscapeCancelsAgentDefault(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "empty settings default to false", raw: `{}`, want: false},
		{name: "missing setting defaults to false", raw: `{"chat_submit_key":"cmd_enter"}`, want: false},
		{name: "explicit false is preserved", raw: `{"escape_cancels_agent":false}`, want: false},
		{name: "true is preserved", raw: `{"escape_cancels_agent":true}`, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings, err := scanUserSettings(settingsScanner{raw: tt.raw}, DefaultUserID)
			if err != nil {
				t.Fatalf("scan settings: %v", err)
			}
			if settings.EscapeCancelsAgent != tt.want {
				t.Fatalf("EscapeCancelsAgent = %v, want %v", settings.EscapeCancelsAgent, tt.want)
			}
		})
	}
}

func TestMarshalUserSettingsEscapeCancelsAgent(t *testing.T) {
	raw, err := marshalUserSettingsPayload(&models.UserSettings{
		EscapeCancelsAgent: true,
	})
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if got := payload["escape_cancels_agent"]; got != true {
		t.Fatalf("escape_cancels_agent = %#v, want true", got)
	}
}

func TestSQLiteRepositoryEscapeCancelsAgentRoundTrip(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	repo, err := newSQLiteRepositoryWithDB(conn, conn)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	ctx := context.Background()
	settings, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get defaults: %v", err)
	}
	if settings.EscapeCancelsAgent {
		t.Fatal("default should be false")
	}
	settings.EscapeCancelsAgent = true
	upsertUserSettingsForTest(t, repo, ctx, settings)

	got, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get saved settings: %v", err)
	}
	if !got.EscapeCancelsAgent {
		t.Fatal("EscapeCancelsAgent did not round-trip through the SQLite blob")
	}
}
