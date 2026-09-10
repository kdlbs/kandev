package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/kandev/kandev/internal/user/models"
)

// @covers AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.3
func TestSQLiteRepositoryStartupPagePersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.db")
	conn := openStartupPageSQLite(t, path)
	writeStartupPageSettings(t, conn)
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	assertStartupPageSettings(t, openStartupPageSQLite(t, path))
}

func TestPostgresRepositoryStartupPagePersistence(t *testing.T) {
	conn := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	writeStartupPageSettings(t, conn)
	assertStartupPageSettings(t, conn)
}

func openStartupPageSQLite(t *testing.T, path string) *sqlx.DB {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	conn.SetMaxOpenConns(1)
	return conn
}

func writeStartupPageSettings(t *testing.T, conn *sqlx.DB) {
	t.Helper()
	repo, err := newSQLiteRepositoryWithDB(conn, conn)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := repo.CreateUser(ctx, &models.User{
		ID: "other-user", Email: "other@example.test", Role: models.RoleMember, Status: models.StatusActive,
	}); err != nil {
		t.Fatal(err)
	}
	settings, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	settings.StartupPage = "threads"
	upsertUserSettingsForTest(t, repo, ctx, settings)
	saved, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.StartupPage != "threads" {
		t.Fatalf("saved startup page = %q, want threads", saved.StartupPage)
	}
	saved.TasksListShowDetails = true
	upsertUserSettingsForTest(t, repo, ctx, saved)
}

func assertStartupPageSettings(t *testing.T, conn *sqlx.DB) {
	t.Helper()
	repo, err := newSQLiteRepositoryWithDB(conn, conn)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := repo.GetUserSettings(context.Background(), DefaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	if settings.StartupPage != "threads" || !settings.TasksListShowDetails || settings.Revision != 2 {
		t.Fatalf("reopened settings: startup=%q details=%v revision=%d", settings.StartupPage,
			settings.TasksListShowDetails, settings.Revision)
	}
	other, err := repo.GetUserSettings(context.Background(), "other-user")
	if err != nil {
		t.Fatal(err)
	}
	if other.StartupPage != models.StartupPageTaskOverview || other.Revision != 0 {
		t.Fatalf("other user's settings changed: startup=%q revision=%d", other.StartupPage, other.Revision)
	}
}
