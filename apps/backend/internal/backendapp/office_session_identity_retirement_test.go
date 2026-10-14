package backendapp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/runtimeflags"
	_ "github.com/mattn/go-sqlite3"
)

func TestStartupIgnoresRetiredOfficeSessionIdentityValues(t *testing.T) {
	const key = "features.officeSessionIdentity"
	const envVar = "KANDEV_FEATURES_OFFICE_SESSION_IDENTITY"
	t.Setenv(envVar, "false")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load with retired environment value: %v", err)
	}
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	store, err := runtimeflags.NewSQLiteStore(db, db)
	if err != nil {
		t.Fatalf("create runtime flag store: %v", err)
	}
	if err := store.SetOverride(context.Background(), key, false); err != nil {
		t.Fatalf("seed retired false override: %v", err)
	}
	if !applyStartupRuntimeFlags(context.Background(), cfg, &Repositories{RuntimeFlags: store}, nil) {
		t.Fatal("startup runtime flag application rejected a retired override")
	}

	if _, active := runtimeflags.OptionsFromConfig(cfg).EnvValues[envVar]; active {
		t.Fatalf("retired environment variable %q remains active after config load", envVar)
	}
	encoded, err := json.Marshal(cfg.Features)
	if err != nil {
		t.Fatalf("marshal feature config: %v", err)
	}
	if strings.Contains(string(encoded), "officeSessionIdentity") {
		t.Fatalf("retired feature remains in loaded config: %s", encoded)
	}
}
