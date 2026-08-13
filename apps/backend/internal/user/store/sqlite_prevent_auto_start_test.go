package store

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/user/models"
)

func TestScanUserSettingsPreventAutoStartAgentOnOpenDefault(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "empty settings default to false", raw: `{}`, want: false},
		{name: "missing setting defaults to false", raw: `{"chat_submit_key":"cmd_enter"}`, want: false},
		{name: "explicit false is preserved", raw: `{"prevent_auto_start_agent_on_open":false}`, want: false},
		{name: "true is preserved", raw: `{"prevent_auto_start_agent_on_open":true}`, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings, err := scanUserSettings(settingsScanner{raw: tt.raw}, DefaultUserID)
			if err != nil {
				t.Fatalf("scan settings: %v", err)
			}
			if settings.PreventAutoStartAgentOnOpen != tt.want {
				t.Fatalf("PreventAutoStartAgentOnOpen = %v, want %v", settings.PreventAutoStartAgentOnOpen, tt.want)
			}
		})
	}
}

func TestMarshalUserSettingsPreventAutoStartAgentOnOpen(t *testing.T) {
	raw, err := marshalUserSettingsPayload(&models.UserSettings{
		PreventAutoStartAgentOnOpen: true,
	})
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if got := payload["prevent_auto_start_agent_on_open"]; got != true {
		t.Fatalf("prevent_auto_start_agent_on_open = %#v, want true", got)
	}
}

func TestSQLiteRepositoryPreventAutoStartAgentOnOpenRoundTrip(t *testing.T) {
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
	if settings.PreventAutoStartAgentOnOpen {
		t.Fatal("default should be false")
	}
	settings.PreventAutoStartAgentOnOpen = true
	upsertUserSettingsForTest(t, repo, ctx, settings)

	got, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get saved settings: %v", err)
	}
	if !got.PreventAutoStartAgentOnOpen {
		t.Fatal("PreventAutoStartAgentOnOpen did not round-trip through the SQLite blob")
	}
}
