package managedruntime

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
)

func TestOpenCodeSelectionPersistsAcrossStoreReopen(t *testing.T) {
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "settings.db")
	settings, closeSettings := openOpenCodeSettingsStore(t, databasePath)
	store := NewStore(settings)
	v1 := OpenCodeSelection{
		SchemaVersion:         1,
		Family:                OpenCodeFamilyV1,
		Source:                OpenCodeSourceManaged,
		Package:               "opencode-ai",
		SelectedVersion:       "1.18.32",
		AppliedDefaultVersion: "1.18.32",
		Revision:              1,
	}
	if err := store.SaveOpenCodeSelection(ctx, 0, v1); err != nil {
		t.Fatalf("save initial v1 selection: %v", err)
	}

	v2 := OpenCodeSelection{
		SchemaVersion:         1,
		Family:                OpenCodeFamilyV2,
		Source:                OpenCodeSourceManaged,
		Package:               "@opencode/cli",
		SelectedVersion:       "2.0.18",
		AppliedDefaultVersion: "2.0.18",
		Revision:              2,
	}
	if err := store.SaveOpenCodeSelection(ctx, v1.Revision, v2); err != nil {
		t.Fatalf("save migrated v2 selection: %v", err)
	}
	closeSettings()

	reopenedSettings, closeReopenedSettings := openOpenCodeSettingsStore(t, databasePath)
	t.Cleanup(closeReopenedSettings)
	reopenedSelection, found, err := NewStore(reopenedSettings).GetOpenCodeSelection(ctx)
	if err != nil {
		t.Fatalf("read selection after reopening database: %v", err)
	}
	if !found {
		t.Fatal("selection was missing after reopening database")
	}
	if reopenedSelection != v2 {
		t.Fatalf("selection after reopening = %+v, want %+v", reopenedSelection, v2)
	}
}

func openOpenCodeSettingsStore(t *testing.T, databasePath string) (*systemsettings.Store, func()) {
	t.Helper()

	connection, err := db.OpenSQLite(databasePath)
	if err != nil {
		t.Fatalf("open settings database: %v", err)
	}
	database := sqlx.NewDb(connection, "sqlite3")
	settings, err := systemsettings.NewStore(db.NewPool(database, database))
	if err != nil {
		_ = database.Close()
		t.Fatalf("create system settings store: %v", err)
	}
	return settings, func() {
		_ = database.Close()
	}
}
