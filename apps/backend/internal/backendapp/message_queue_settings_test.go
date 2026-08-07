package backendapp

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/system/queuesettings"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
)

func TestResolveQueueMaxPerSessionPrecedence(t *testing.T) {
	tests := []struct {
		name      string
		persisted int
		env       string
		want      int
	}{
		{name: "persisted setting", persisted: 6, want: 6},
		{name: "environment wins", persisted: 6, env: "20", want: 20},
		{name: "invalid environment falls back to setting", persisted: 6, env: "many", want: 6},
		{name: "negative environment means unlimited", persisted: 6, env: "-3", want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pool := newMessageQueueSettingsTestPool(t)
			raw, err := systemsettings.NewStore(pool)
			if err != nil {
				t.Fatalf("new system settings store: %v", err)
			}
			store := queuesettings.NewStore(raw)
			if err := store.Save(context.Background(), queuesettings.Settings{MaxPerSession: tc.persisted}); err != nil {
				t.Fatalf("save setting: %v", err)
			}
			t.Setenv(queuesettings.EnvironmentVariable, tc.env)

			if got := resolveQueueMaxPerSession(pool, testLogger(t)); got != tc.want {
				t.Fatalf("max = %d, want %d", got, tc.want)
			}
		})
	}
}

func newMessageQueueSettingsTestPool(t *testing.T) *db.Pool {
	t.Helper()
	connection, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = connection.Close() })
	return db.NewPool(connection, connection)
}
