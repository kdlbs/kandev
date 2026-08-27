package backendapp

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	commonconfig "github.com/kandev/kandev/internal/common/config"
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
			if err := store.Save(context.Background(), queuesettings.Settings{
				MaxPerSession: tc.persisted, MergeEnabled: true, AutoMergeEnabled: true,
			}); err != nil {
				t.Fatalf("save setting: %v", err)
			}
			t.Setenv(queuesettings.EnvironmentVariable, tc.env)

			if got := resolveQueueMaxPerSession(pool, testLogger(t)); got != tc.want {
				t.Fatalf("max = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestResolveQueueMaxPerSessionUsesYAMLConfigurationBeforePersistedSetting(t *testing.T) {
	pool := newMessageQueueSettingsTestPool(t)
	raw, err := systemsettings.NewStore(pool)
	if err != nil {
		t.Fatalf("new system settings store: %v", err)
	}
	if err := queuesettings.NewStore(raw).Save(context.Background(), queuesettings.Settings{
		MaxPerSession: 6, MergeEnabled: true, AutoMergeEnabled: true,
	}); err != nil {
		t.Fatalf("save setting: %v", err)
	}

	cfg := &commonconfig.Config{
		MessageQueue: commonconfig.MessageQueueConfig{MaxPerSession: 14},
		Source: commonconfig.ConfigSource{Values: map[string]commonconfig.SettingSource{
			"messageQueue.maxPerSession": commonconfig.SourceConfiguration,
		}},
	}
	t.Setenv(queuesettings.EnvironmentVariable, "")
	startup := queueConfiguration(cfg)
	resolution := resolveQueueSettings(pool, testLogger(t), startup)
	if resolution.Effective.MaxPerSession != 14 ||
		resolution.Effective.Source != queuesettings.SourceConfiguration ||
		!resolution.Effective.Locked {
		t.Fatalf("resolution = %+v, want locked YAML configuration value", resolution.Effective)
	}
}

func TestResolveQueueMaxPerSessionYAMLZeroLocksUnlimitedCapacity(t *testing.T) {
	pool := newMessageQueueSettingsTestPool(t)
	raw, err := systemsettings.NewStore(pool)
	if err != nil {
		t.Fatalf("new system settings store: %v", err)
	}
	if err := queuesettings.NewStore(raw).Save(context.Background(), queuesettings.Settings{
		MaxPerSession: 6, MergeEnabled: true, AutoMergeEnabled: true,
	}); err != nil {
		t.Fatalf("save setting: %v", err)
	}

	cfg := &commonconfig.Config{
		MessageQueue: commonconfig.MessageQueueConfig{MaxPerSession: 0},
		Source: commonconfig.ConfigSource{Values: map[string]commonconfig.SettingSource{
			"messageQueue.maxPerSession": commonconfig.SourceConfiguration,
		}},
	}
	t.Setenv(queuesettings.EnvironmentVariable, "")
	resolution := resolveQueueSettings(pool, testLogger(t), queueConfiguration(cfg))
	if resolution.Effective.MaxPerSession != 0 ||
		resolution.Effective.Source != queuesettings.SourceConfiguration ||
		!resolution.Effective.Locked {
		t.Fatalf("resolution = %+v, want locked YAML unlimited value", resolution.Effective)
	}
}

func TestResolveQueueMergeEnabledPrecedence(t *testing.T) {
	tests := []struct {
		name      string
		persisted *bool
		want      bool
	}{
		{name: "no persisted setting defaults to enabled", want: true},
		{name: "persisted disabled", persisted: new(false), want: false},
		{name: "persisted enabled", persisted: new(true), want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pool := newMessageQueueSettingsTestPool(t)
			if tc.persisted != nil {
				raw, err := systemsettings.NewStore(pool)
				if err != nil {
					t.Fatalf("new system settings store: %v", err)
				}
				store := queuesettings.NewStore(raw)
				if err := store.Save(context.Background(), queuesettings.Settings{
					MaxPerSession: queuesettings.DefaultMaxPerSession,
					MergeEnabled:  *tc.persisted, AutoMergeEnabled: true,
				}); err != nil {
					t.Fatalf("save setting: %v", err)
				}
			}

			if got := resolveQueueMergeEnabled(pool, testLogger(t)); got != tc.want {
				t.Fatalf("merge enabled = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestResolveQueueMergeEnabledWithoutPool asserts the nil-pool fallback path
// (mirrors resolveQueueMaxPerSession's own nil-pool behavior) still defaults
// merging to enabled.
func TestResolveQueueMergeEnabledWithoutPool(t *testing.T) {
	if got := resolveQueueMergeEnabled(nil, testLogger(t)); !got {
		t.Fatalf("merge enabled = %v, want true", got)
	}
}

func TestResolveQueueAutoMergeEnabledPrecedence(t *testing.T) {
	tests := []struct {
		name      string
		persisted *bool
		env       string
		want      bool
	}{
		{name: "no persisted setting defaults to enabled", want: true},
		{name: "persisted disabled", persisted: new(false), want: false},
		{name: "persisted enabled", persisted: new(true), want: true},
		{name: "capacity environment does not override", persisted: new(false), env: "20", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pool := newMessageQueueSettingsTestPool(t)
			if test.persisted != nil {
				raw, err := systemsettings.NewStore(pool)
				if err != nil {
					t.Fatalf("new system settings store: %v", err)
				}
				if err := queuesettings.NewStore(raw).Save(context.Background(), queuesettings.Settings{
					MaxPerSession: queuesettings.DefaultMaxPerSession,
					MergeEnabled:  true, AutoMergeEnabled: *test.persisted,
				}); err != nil {
					t.Fatalf("save setting: %v", err)
				}
			}
			t.Setenv(queuesettings.EnvironmentVariable, test.env)
			if got := resolveQueueAutoMergeEnabled(pool, testLogger(t)); got != test.want {
				t.Fatalf("automatic merge enabled = %v, want %v", got, test.want)
			}
		})
	}
}

func TestResolveQueueAutoMergeEnabledWithoutPool(t *testing.T) {
	if got := resolveQueueAutoMergeEnabled(nil, testLogger(t)); !got {
		t.Fatalf("automatic merge enabled = %v, want true", got)
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
