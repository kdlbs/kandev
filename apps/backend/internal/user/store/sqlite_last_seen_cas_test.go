package store

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/testutil"
	"github.com/kandev/kandev/internal/user/models"
)

// TestDefaultUserSettingsLastSeenDisplay verifies the default LastSeenDisplay is absolute.
func TestDefaultUserSettingsLastSeenDisplay(t *testing.T) {
	settings := defaultUserSettings(DefaultUserID)
	if settings.LastSeenDisplay != models.LastSeenDisplayAbsolute {
		t.Fatalf("default LastSeenDisplay = %q, want %q", settings.LastSeenDisplay, models.LastSeenDisplayAbsolute)
	}
}

// TestScanUserSettingsLastSeenDisplay verifies last_seen_display defaults to absolute, loads stored values, and coerces unknown values.
func TestScanUserSettingsLastSeenDisplay(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "default empty", raw: "{}", want: models.LastSeenDisplayAbsolute},
		{name: "relative stored", raw: `{"last_seen_display":"relative"}`, want: models.LastSeenDisplayRelative},
		{name: "absolute stored", raw: `{"last_seen_display":"absolute"}`, want: models.LastSeenDisplayAbsolute},
		{name: "unknown coerced", raw: `{"last_seen_display":"garbage"}`, want: models.LastSeenDisplayAbsolute},
		{name: "empty coerced", raw: `{"last_seen_display":""}`, want: models.LastSeenDisplayAbsolute},
		{name: "number coerced", raw: `{"last_seen_display":123}`, want: models.LastSeenDisplayAbsolute},
		{name: "object coerced", raw: `{"last_seen_display":{"x":1}}`, want: models.LastSeenDisplayAbsolute},
		{name: "boolean coerced", raw: `{"last_seen_display":true}`, want: models.LastSeenDisplayAbsolute},
		{name: "null coerced", raw: `{"last_seen_display":null}`, want: models.LastSeenDisplayAbsolute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings, err := scanUserSettings(settingsScanner{raw: tt.raw}, DefaultUserID)
			if err != nil {
				t.Fatalf("scan settings: %v", err)
			}
			if settings.LastSeenDisplay != tt.want {
				t.Fatalf("LastSeenDisplay = %q, want %q", settings.LastSeenDisplay, tt.want)
			}
		})
	}
}

// TestMarshalUserSettingsPersistsLastSeenDisplay verifies a relative LastSeenDisplay survives a marshal and scan round trip.
func TestMarshalUserSettingsPersistsLastSeenDisplay(t *testing.T) {
	raw, err := marshalUserSettingsPayload(&models.UserSettings{LastSeenDisplay: models.LastSeenDisplayRelative})
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	settings, err := scanUserSettings(settingsScanner{raw: string(raw)}, DefaultUserID)
	if err != nil {
		t.Fatalf("scan settings: %v", err)
	}
	if settings.LastSeenDisplay != models.LastSeenDisplayRelative {
		t.Fatalf("round-trip LastSeenDisplay = %q, want %q", settings.LastSeenDisplay, models.LastSeenDisplayRelative)
	}
}

// TestMarshalUserSettingsNormalizesUnknownLastSeenDisplay verifies an unknown LastSeenDisplay is marshaled as absolute.
func TestMarshalUserSettingsNormalizesUnknownLastSeenDisplay(t *testing.T) {
	raw, err := marshalUserSettingsPayload(&models.UserSettings{LastSeenDisplay: "garbage"})
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	if !strings.Contains(string(raw), `"last_seen_display":"absolute"`) {
		t.Fatalf("marshaled payload = %s, want normalized absolute", raw)
	}
}

// TestSQLiteRepositoryLastSeenDisplayRoundTrip verifies LastSeenDisplay round-trips through the SQLite repository.
func TestSQLiteRepositoryLastSeenDisplayRoundTrip(t *testing.T) {
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
	if settings.LastSeenDisplay != models.LastSeenDisplayAbsolute {
		t.Fatalf("default LastSeenDisplay = %q, want %q", settings.LastSeenDisplay, models.LastSeenDisplayAbsolute)
	}

	settings.LastSeenDisplay = models.LastSeenDisplayRelative
	updated, err := repo.UpsertUserSettingsPreservingTaskCreateLastUsed(ctx, settings, nil, settings.Revision)
	if err != nil {
		t.Fatalf("write relative: %v", err)
	}
	if updated.LastSeenDisplay != models.LastSeenDisplayRelative {
		t.Fatalf("persisted LastSeenDisplay = %q, want %q", updated.LastSeenDisplay, models.LastSeenDisplayRelative)
	}

	updated.LastSeenDisplay = models.LastSeenDisplayAbsolute
	if _, err := repo.UpsertUserSettingsPreservingTaskCreateLastUsed(ctx, updated, nil, updated.Revision); err != nil {
		t.Fatalf("write absolute: %v", err)
	}

	reloaded, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("reload settings: %v", err)
	}
	if reloaded.LastSeenDisplay != models.LastSeenDisplayAbsolute {
		t.Fatalf("reloaded LastSeenDisplay = %q, want %q", reloaded.LastSeenDisplay, models.LastSeenDisplayAbsolute)
	}
}

// TestSQLiteRepositoryUpsertSettingsRevisionConflict verifies a stale revision write fails with ErrUserSettingsRevisionConflict.
func TestSQLiteRepositoryUpsertSettingsRevisionConflict(t *testing.T) {
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
	staleRevision := settings.Revision
	settings.AppStatusBarEnabled = true
	if _, err := repo.UpsertUserSettingsPreservingTaskCreateLastUsed(ctx, settings, nil, settings.Revision); err != nil {
		t.Fatalf("initial write: %v", err)
	}

	stale := *settings
	stale.AppStatusBarEnabled = false
	if _, err := repo.UpsertUserSettingsPreservingTaskCreateLastUsed(ctx, &stale, nil, staleRevision); !errors.Is(err, ErrUserSettingsRevisionConflict) {
		t.Fatalf("stale write error = %v, want ErrUserSettingsRevisionConflict", err)
	}
}

// TestSQLiteRepositoryUpsertSettingsMissingUser verifies upserting settings for a missing user fails with a user-not-found error.
func TestSQLiteRepositoryUpsertSettingsMissingUser(t *testing.T) {
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
	if _, err := repo.UpsertUserSettingsPreservingTaskCreateLastUsed(ctx, &models.UserSettings{UserID: "missing-user"}, nil, 0); err == nil {
		t.Fatal("missing user upsert = nil, want user not found")
	} else if errors.Is(err, ErrUserSettingsRevisionConflict) {
		t.Fatalf("missing user upsert returned conflict, want user not found: %v", err)
	} else if !strings.Contains(err.Error(), "user not found") {
		t.Fatalf("missing user upsert error = %v, want user not found", err)
	}
}

// TestPostgresRepositoryUpsertSettingsRevisionConflict verifies a stale revision write fails with ErrUserSettingsRevisionConflict on Postgres.
func TestPostgresRepositoryUpsertSettingsRevisionConflict(t *testing.T) {
	conn := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := newSQLiteRepositoryWithDB(conn, conn)
	if err != nil {
		t.Fatalf("new postgres repo: %v", err)
	}

	ctx := context.Background()
	settings, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get defaults: %v", err)
	}
	staleRevision := settings.Revision
	settings.AppStatusBarEnabled = true
	if _, err := repo.UpsertUserSettingsPreservingTaskCreateLastUsed(ctx, settings, nil, settings.Revision); err != nil {
		t.Fatalf("initial write: %v", err)
	}

	stale := *settings
	stale.AppStatusBarEnabled = false
	if _, err := repo.UpsertUserSettingsPreservingTaskCreateLastUsed(ctx, &stale, nil, staleRevision); !errors.Is(err, ErrUserSettingsRevisionConflict) {
		t.Fatalf("stale write error = %v, want ErrUserSettingsRevisionConflict", err)
	}
}

// TestPostgresRepositoryUpsertSettingsMissingUser verifies upserting settings for a missing user fails with a user-not-found error on Postgres.
func TestPostgresRepositoryUpsertSettingsMissingUser(t *testing.T) {
	conn := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := newSQLiteRepositoryWithDB(conn, conn)
	if err != nil {
		t.Fatalf("new postgres repo: %v", err)
	}

	ctx := context.Background()
	if _, err := repo.UpsertUserSettingsPreservingTaskCreateLastUsed(ctx, &models.UserSettings{UserID: "missing-user"}, nil, 0); err == nil {
		t.Fatal("missing user upsert = nil, want user not found")
	} else if errors.Is(err, ErrUserSettingsRevisionConflict) {
		t.Fatalf("missing user upsert returned conflict, want user not found: %v", err)
	} else if !strings.Contains(err.Error(), "user not found") {
		t.Fatalf("missing user upsert error = %v, want user not found", err)
	}
}

// TestSQLiteRepositoryConcurrentUpsertOneSucceedsOneConflicts is the migrated
// unique-revision test: two candidates read the same revision, exactly one
// write commits and the other reports a revision conflict (retry lives in the
// service layer).
func TestSQLiteRepositoryConcurrentUpsertOneSucceedsOneConflicts(t *testing.T) {
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
	expectedRevision := settings.Revision
	first := *settings
	second := *settings
	first.AppStatusBarEnabled = true
	second.ChangesPanelLayout = "flat"

	start := make(chan struct{})
	revisions := make(chan int64, 2)
	conflicts := make(chan error, 2)
	other := make(chan error, 2)
	var workers sync.WaitGroup
	for _, candidate := range []*models.UserSettings{&first, &second} {
		workers.Add(1)
		go func(candidate *models.UserSettings) {
			defer workers.Done()
			<-start
			updated, updateErr := repo.UpsertUserSettingsPreservingTaskCreateLastUsed(ctx, candidate, nil, expectedRevision)
			if updateErr != nil {
				if errors.Is(updateErr, ErrUserSettingsRevisionConflict) {
					conflicts <- updateErr
				} else {
					other <- updateErr
				}
				return
			}
			revisions <- updated.Revision
		}(candidate)
	}
	close(start)
	workers.Wait()
	close(revisions)
	close(conflicts)
	close(other)

	for updateErr := range other {
		t.Fatalf("unexpected concurrent update error: %v", updateErr)
	}
	var got []int64
	for revision := range revisions {
		got = append(got, revision)
	}
	conflictCount := 0
	for range conflicts {
		conflictCount++
	}
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("successful revisions = %v, want exactly [1]", got)
	}
	if conflictCount != 1 {
		t.Fatalf("conflict count = %d, want exactly 1", conflictCount)
	}
	final, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get final settings: %v", err)
	}
	if final.Revision != 1 {
		t.Fatalf("final revision = %d, want 1", final.Revision)
	}
}

// TestPostgresRepositoryTaskCreateLastUsedRoundTrip verifies the full-blob
// write merges the current task_create_last_used over the passed blob when a
// stale caller re-reads the current revision.
func TestPostgresRepositoryTaskCreateLastUsedRoundTrip(t *testing.T) {
	conn := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := newSQLiteRepositoryWithDB(conn, conn)
	if err != nil {
		t.Fatalf("new postgres repo: %v", err)
	}

	ctx := context.Background()
	staleSettings, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get defaults: %v", err)
	}
	staleSettings.SidebarActiveViewID = "view-before"
	staleSettings.TaskCreateLastUsed = models.TaskCreateLastUsed{
		RepositoryID:      "repo-before",
		Branch:            "main",
		AgentProfileID:    "agent-before",
		ExecutorProfileID: "exec-before",
	}
	upsertUserSettingsForTest(t, repo, ctx, staleSettings)

	got, err := repo.UpdateTaskCreateLastUsed(ctx, DefaultUserID, models.TaskCreateLastUsed{
		RepositoryID:   "repo-after",
		Branch:         "feature",
		AgentProfileID: "agent-after",
	})
	if err != nil {
		t.Fatalf("update postgres task-create last-used: %v", err)
	}
	if got.SidebarActiveViewID != "view-before" {
		t.Fatalf("unrelated postgres setting should be preserved, got %q", got.SidebarActiveViewID)
	}
	if got.TaskCreateLastUsed.RepositoryID != "repo-after" ||
		got.TaskCreateLastUsed.Branch != "feature" ||
		got.TaskCreateLastUsed.AgentProfileID != "agent-after" ||
		got.TaskCreateLastUsed.ExecutorProfileID != "exec-before" {
		t.Fatalf("postgres task-create update mismatch: %+v", got.TaskCreateLastUsed)
	}
	if got.Revision < 2 {
		t.Fatalf("postgres settings revision = %d, want at least 2", got.Revision)
	}
	firstRevision := got.Revision

	// A stale caller re-reads the current revision, then writes an unrelated
	// change carrying stale task-create fields; the merge must keep the stored
	// current values.
	current, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("re-read current settings: %v", err)
	}
	current.SidebarActiveViewID = "view-after"
	current.TaskCreateLastUsed = models.TaskCreateLastUsed{
		RepositoryID: "repo-stale",
		Branch:       "stale",
	}
	got, err = repo.UpsertUserSettingsPreservingTaskCreateLastUsed(ctx, current, nil, current.Revision)
	if err != nil {
		t.Fatalf("upsert preserving postgres task-create last-used: %v", err)
	}
	if got.SidebarActiveViewID != "view-after" {
		t.Fatalf("expected unrelated postgres setting to update, got %q", got.SidebarActiveViewID)
	}
	if got.TaskCreateLastUsed.RepositoryID != "repo-after" ||
		got.TaskCreateLastUsed.Branch != "feature" ||
		got.TaskCreateLastUsed.AgentProfileID != "agent-after" ||
		got.TaskCreateLastUsed.ExecutorProfileID != "exec-before" {
		t.Fatalf("postgres preserving upsert should keep current task-create values: %+v", got.TaskCreateLastUsed)
	}
	if got.Revision != firstRevision+1 {
		t.Fatalf("postgres settings revision = %d, want %d", got.Revision, firstRevision+1)
	}
}

// TestSQLiteRepositoryUpsertSettingsPreservesCurrentTaskCreateLastUsed is the
// migrated stale-blob merge test: the caller re-reads the current revision
// before its unrelated write so the task_create_last_used merge survives.
func TestSQLiteRepositoryUpsertSettingsPreservesCurrentTaskCreateLastUsed(t *testing.T) {
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
	staleSettings, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get defaults: %v", err)
	}
	staleSettings.SidebarActiveViewID = "view-before"
	staleSettings.TaskCreateLastUsed = models.TaskCreateLastUsed{RepositoryID: "repo-before"}
	upsertUserSettingsForTest(t, repo, ctx, staleSettings)

	// Simulate a task-create write that lands after another settings caller
	// read the old blob but before that caller writes its unrelated change.
	if _, err := repo.UpdateTaskCreateLastUsed(ctx, DefaultUserID, models.TaskCreateLastUsed{
		RepositoryID: "repo-after",
		Branch:       "feature",
	}); err != nil {
		t.Fatalf("update task-create last-used: %v", err)
	}

	current, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("re-read current settings: %v", err)
	}
	current.SidebarActiveViewID = "view-after"
	got, err := repo.UpsertUserSettingsPreservingTaskCreateLastUsed(ctx, current, nil, current.Revision)
	if err != nil {
		t.Fatalf("upsert preserving task-create last-used: %v", err)
	}

	if got.SidebarActiveViewID != "view-after" {
		t.Fatalf("expected unrelated setting to update, got %q", got.SidebarActiveViewID)
	}
	if got.TaskCreateLastUsed.RepositoryID != "repo-after" {
		t.Fatalf("expected current task-create repository to survive stale write, got %q", got.TaskCreateLastUsed.RepositoryID)
	}
	if got.TaskCreateLastUsed.Branch != "feature" {
		t.Fatalf("expected current task-create branch to survive stale write, got %q", got.TaskCreateLastUsed.Branch)
	}
}

// TestSQLiteRepositoryUpsertSettingsPreservingTaskCreateLastUsedAppliesPatch is
// the migrated patch-path test, re-reading the current revision before the
// direct upsert.
func TestSQLiteRepositoryUpsertSettingsPreservingTaskCreateLastUsedAppliesPatch(t *testing.T) {
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
	staleSettings, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get defaults: %v", err)
	}
	staleSettings.SidebarActiveViewID = "view-before"
	staleSettings.TaskCreateLastUsed = models.TaskCreateLastUsed{
		RepositoryID:      "repo-before",
		Branch:            "main",
		AgentProfileID:    "agent-before",
		ExecutorProfileID: "exec-before",
	}
	upsertUserSettingsForTest(t, repo, ctx, staleSettings)
	if _, err := repo.UpdateTaskCreateLastUsed(ctx, DefaultUserID, models.TaskCreateLastUsed{
		RepositoryID:      "repo-current",
		Branch:            "current",
		ExecutorProfileID: "exec-current",
	}); err != nil {
		t.Fatalf("update current task-create last-used: %v", err)
	}

	current, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("re-read current settings: %v", err)
	}
	current.SidebarActiveViewID = "view-after"
	current.TaskCreateLastUsed = models.TaskCreateLastUsed{
		RepositoryID:   "repo-stale",
		Branch:         "stale",
		AgentProfileID: "agent-stale",
	}
	patch := models.TaskCreateLastUsed{AgentProfileID: "agent-after"}
	got, err := repo.UpsertUserSettingsPreservingTaskCreateLastUsed(ctx, current, &patch, current.Revision)
	if err != nil {
		t.Fatalf("upsert preserving task-create last-used with patch: %v", err)
	}

	if got.SidebarActiveViewID != "view-after" {
		t.Fatalf("expected unrelated setting to update, got %q", got.SidebarActiveViewID)
	}
	if got.TaskCreateLastUsed.RepositoryID != "repo-current" {
		t.Fatalf("expected current repository to survive stale write, got %q", got.TaskCreateLastUsed.RepositoryID)
	}
	if got.TaskCreateLastUsed.Branch != "current" {
		t.Fatalf("expected current branch to survive stale write, got %q", got.TaskCreateLastUsed.Branch)
	}
	if got.TaskCreateLastUsed.AgentProfileID != "agent-after" {
		t.Fatalf("expected patch agent profile to apply, got %q", got.TaskCreateLastUsed.AgentProfileID)
	}
	if got.TaskCreateLastUsed.ExecutorProfileID != "exec-current" {
		t.Fatalf("expected current executor profile to survive stale write, got %q", got.TaskCreateLastUsed.ExecutorProfileID)
	}
}
