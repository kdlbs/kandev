package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// seedExecutorForProfiles creates the executors row that executor_profiles
// references through its foreign key.
func seedExecutorForProfiles(t *testing.T, repo *Repository, id string) {
	t.Helper()
	if err := repo.CreateExecutor(context.Background(), &models.Executor{
		ID: id, Name: id, Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive,
	}); err != nil {
		t.Fatalf("CreateExecutor(%s): %v", id, err)
	}
}

func assertExecutorProfileEqual(t *testing.T, got, want *models.ExecutorProfile) {
	t.Helper()
	if got == nil {
		t.Fatal("profile is nil")
	}
	if got.ID != want.ID {
		t.Errorf("ID = %q, want %q", got.ID, want.ID)
	}
	if got.ExecutorID != want.ExecutorID {
		t.Errorf("ExecutorID = %q, want %q", got.ExecutorID, want.ExecutorID)
	}
	if got.Name != want.Name {
		t.Errorf("Name = %q, want %q", got.Name, want.Name)
	}
	if got.McpPolicy != want.McpPolicy {
		t.Errorf("McpPolicy = %q, want %q", got.McpPolicy, want.McpPolicy)
	}
	if got.PrepareScript != want.PrepareScript {
		t.Errorf("PrepareScript = %q, want %q", got.PrepareScript, want.PrepareScript)
	}
	if got.CleanupScript != want.CleanupScript {
		t.Errorf("CleanupScript = %q, want %q", got.CleanupScript, want.CleanupScript)
	}
	if len(got.Config) != len(want.Config) {
		t.Errorf("Config = %v, want %v", got.Config, want.Config)
	}
	for key, value := range want.Config {
		if got.Config[key] != value {
			t.Errorf("Config[%q] = %q, want %q", key, got.Config[key], value)
		}
	}
	if len(got.EnvVars) != len(want.EnvVars) {
		t.Errorf("EnvVars = %+v, want %+v", got.EnvVars, want.EnvVars)
	} else {
		for i := range want.EnvVars {
			if got.EnvVars[i] != want.EnvVars[i] {
				t.Errorf("EnvVars[%d] = %+v, want %+v", i, got.EnvVars[i], want.EnvVars[i])
			}
		}
	}
	assertTimeEqual(t, "CreatedAt", got.CreatedAt, want.CreatedAt)
	assertTimeEqual(t, "UpdatedAt", got.UpdatedAt, want.UpdatedAt)
}

func TestCreateExecutorProfileRoundTripsEveryField(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedExecutorForProfiles(t, repo, "executor-profile-full")

	before := time.Now().UTC().Add(-time.Second)
	want := &models.ExecutorProfile{
		ID:            "profile-full",
		ExecutorID:    "executor-profile-full",
		Name:          "Full profile",
		McpPolicy:     "allow_all",
		Config:        map[string]string{"docker_host": "unix:///var/run/docker.sock", "cpus": "4"},
		PrepareScript: "pnpm install --frozen-lockfile",
		CleanupScript: "rm -rf node_modules",
		EnvVars: []models.ProfileEnvVar{
			{Key: "NODE_ENV", Value: "test"},
			{Key: "API_TOKEN", SecretID: "secret-123"},
		},
	}
	if err := repo.CreateExecutorProfile(ctx, want); err != nil {
		t.Fatalf("CreateExecutorProfile: %v", err)
	}
	if want.CreatedAt.Before(before) || !want.CreatedAt.Equal(want.UpdatedAt) {
		t.Fatalf("CreateExecutorProfile stamped CreatedAt=%v UpdatedAt=%v, want equal times after %v",
			want.CreatedAt, want.UpdatedAt, before)
	}

	got, err := repo.GetExecutorProfile(ctx, "profile-full")
	if err != nil {
		t.Fatalf("GetExecutorProfile: %v", err)
	}
	assertExecutorProfileEqual(t, got, want)

	listed, err := repo.ListExecutorProfiles(ctx, "executor-profile-full")
	if err != nil {
		t.Fatalf("ListExecutorProfiles: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("ListExecutorProfiles returned %d rows, want 1", len(listed))
	}
	assertExecutorProfileEqual(t, listed[0], want)
}

func TestCreateExecutorProfileGeneratesIDAndNormalizesEmptyCollections(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedExecutorForProfiles(t, repo, "executor-profile-defaults")

	profile := &models.ExecutorProfile{ExecutorID: "executor-profile-defaults", Name: "Bare"}
	if err := repo.CreateExecutorProfile(ctx, profile); err != nil {
		t.Fatalf("CreateExecutorProfile: %v", err)
	}
	if profile.ID == "" {
		t.Fatal("ID left empty; a UUID should have been generated")
	}
	got, err := repo.GetExecutorProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("GetExecutorProfile: %v", err)
	}
	if got.Config != nil {
		t.Errorf("Config = %v, want nil for a profile written without one", got.Config)
	}
	if got.EnvVars != nil {
		t.Errorf("EnvVars = %+v, want nil for a profile written without any", got.EnvVars)
	}
	if got.McpPolicy != "" || got.PrepareScript != "" || got.CleanupScript != "" {
		t.Errorf("empty string columns = %q/%q/%q, want all empty",
			got.McpPolicy, got.PrepareScript, got.CleanupScript)
	}

	// An explicitly empty map serializes to the "{}" sentinel and is skipped
	// on read; an explicitly empty slice serializes to "[]" and decodes back
	// as an empty (non-nil) slice.
	empty := &models.ExecutorProfile{
		ID: "profile-empty", ExecutorID: "executor-profile-defaults", Name: "Empty",
		Config: map[string]string{}, EnvVars: []models.ProfileEnvVar{},
	}
	if err := repo.CreateExecutorProfile(ctx, empty); err != nil {
		t.Fatalf("CreateExecutorProfile(empty): %v", err)
	}
	gotEmpty, err := repo.GetExecutorProfile(ctx, "profile-empty")
	if err != nil {
		t.Fatalf("GetExecutorProfile(empty): %v", err)
	}
	if gotEmpty.Config != nil {
		t.Errorf("Config = %v, want nil (an empty map serializes to the {} sentinel)", gotEmpty.Config)
	}
	if gotEmpty.EnvVars == nil || len(gotEmpty.EnvVars) != 0 {
		t.Errorf("EnvVars = %+v, want an empty non-nil slice", gotEmpty.EnvVars)
	}
}

func TestCreateExecutorProfileRejectsUnknownExecutor(t *testing.T) {
	repo := newRepoForEntityTests(t)

	err := repo.CreateExecutorProfile(context.Background(), &models.ExecutorProfile{
		ID: "profile-orphan", ExecutorID: "executor-does-not-exist", Name: "Orphan",
	})
	if err == nil {
		t.Fatal("CreateExecutorProfile accepted a profile for a non-existent executor")
	}
	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM executor_profiles WHERE id = ?`, "profile-orphan"); got != 0 {
		t.Errorf("rows = %d, want 0", got)
	}
}

func TestGetExecutorProfileReportsMissingID(t *testing.T) {
	repo := newRepoForEntityTests(t)

	got, err := repo.GetExecutorProfile(context.Background(), "profile-nope")
	if got != nil {
		t.Errorf("profile = %+v, want nil", got)
	}
	if err == nil {
		t.Fatal("GetExecutorProfile on a missing id returned nil error")
	}
	if !strings.Contains(err.Error(), "executor profile not found: profile-nope") {
		t.Errorf("error = %q, want an executor-profile-not-found message naming the id", err)
	}
}

func TestUpdateExecutorProfileWritesMutableFieldsAndReportsMissing(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedExecutorForProfiles(t, repo, "executor-profile-update")

	profile := &models.ExecutorProfile{
		ID: "profile-update", ExecutorID: "executor-profile-update", Name: "Before",
		McpPolicy: "deny", Config: map[string]string{"old": "value"},
		PrepareScript: "old prepare", CleanupScript: "old cleanup",
		EnvVars: []models.ProfileEnvVar{{Key: "OLD", Value: "1"}},
	}
	if err := repo.CreateExecutorProfile(ctx, profile); err != nil {
		t.Fatalf("CreateExecutorProfile: %v", err)
	}
	createdAt := profile.CreatedAt

	profile.Name = "After"
	profile.McpPolicy = "allow"
	profile.Config = map[string]string{"new": "value", "extra": "x"}
	profile.PrepareScript = "new prepare"
	profile.CleanupScript = "new cleanup"
	profile.EnvVars = []models.ProfileEnvVar{{Key: "NEW", SecretID: "secret-9"}}
	if err := repo.UpdateExecutorProfile(ctx, profile); err != nil {
		t.Fatalf("UpdateExecutorProfile: %v", err)
	}

	got, err := repo.GetExecutorProfile(ctx, "profile-update")
	if err != nil {
		t.Fatalf("GetExecutorProfile: %v", err)
	}
	assertExecutorProfileEqual(t, got, profile)
	if !got.CreatedAt.Equal(createdAt) {
		t.Errorf("CreatedAt = %v, want it preserved at %v", got.CreatedAt, createdAt)
	}
	if !got.UpdatedAt.After(createdAt) {
		t.Errorf("UpdatedAt = %v, want it bumped past CreatedAt %v", got.UpdatedAt, createdAt)
	}
	if got.ExecutorID != "executor-profile-update" {
		t.Errorf("ExecutorID = %q, want it untouched by the update", got.ExecutorID)
	}

	err = repo.UpdateExecutorProfile(ctx, &models.ExecutorProfile{ID: "profile-nowhere", Name: "x"})
	if err == nil {
		t.Fatal("UpdateExecutorProfile on a missing row returned nil")
	}
	if !strings.Contains(err.Error(), "executor profile not found: profile-nowhere") {
		t.Errorf("error = %q, want an executor-profile-not-found message", err)
	}
}

func TestDeleteExecutorProfileRemovesRowAndReportsMissing(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedExecutorForProfiles(t, repo, "executor-profile-delete")

	for _, id := range []string{"profile-del", "profile-keep"} {
		if err := repo.CreateExecutorProfile(ctx, &models.ExecutorProfile{
			ID: id, ExecutorID: "executor-profile-delete", Name: id,
		}); err != nil {
			t.Fatalf("CreateExecutorProfile(%s): %v", id, err)
		}
	}

	if err := repo.DeleteExecutorProfile(ctx, "profile-del"); err != nil {
		t.Fatalf("DeleteExecutorProfile: %v", err)
	}
	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM executor_profiles WHERE executor_id = ?`, "executor-profile-delete"); got != 1 {
		t.Errorf("rows after delete = %d, want 1", got)
	}
	if _, err := repo.GetExecutorProfile(ctx, "profile-del"); err == nil {
		t.Error("GetExecutorProfile returned a deleted profile")
	}
	if _, err := repo.GetExecutorProfile(ctx, "profile-keep"); err != nil {
		t.Errorf("GetExecutorProfile(profile-keep): %v", err)
	}

	err := repo.DeleteExecutorProfile(ctx, "profile-del")
	if err == nil {
		t.Fatal("second DeleteExecutorProfile returned nil; a missing row must be reported")
	}
	if !strings.Contains(err.Error(), "executor profile not found: profile-del") {
		t.Errorf("error = %q, want an executor-profile-not-found message", err)
	}
}

func TestListExecutorProfilesOrdersByNameAndScopesToExecutor(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedExecutorForProfiles(t, repo, "executor-list-a")
	seedExecutorForProfiles(t, repo, "executor-list-b")

	for _, name := range []string{"zulu", "alpha", "mike"} {
		if err := repo.CreateExecutorProfile(ctx, &models.ExecutorProfile{
			ID: "profile-a-" + name, ExecutorID: "executor-list-a", Name: name,
		}); err != nil {
			t.Fatalf("CreateExecutorProfile(%s): %v", name, err)
		}
	}
	if err := repo.CreateExecutorProfile(ctx, &models.ExecutorProfile{
		ID: "profile-b-aaa", ExecutorID: "executor-list-b", Name: "aaa",
	}); err != nil {
		t.Fatalf("CreateExecutorProfile(b): %v", err)
	}

	got, err := repo.ListExecutorProfiles(ctx, "executor-list-a")
	if err != nil {
		t.Fatalf("ListExecutorProfiles: %v", err)
	}
	want := []string{"alpha", "mike", "zulu"}
	if len(got) != len(want) {
		t.Fatalf("ListExecutorProfiles returned %d rows, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Name != want[i] {
			t.Fatalf("names = %q at index %d, want %q (ORDER BY name ASC)", got[i].Name, i, want[i])
		}
		if got[i].ExecutorID != "executor-list-a" {
			t.Fatalf("ExecutorID = %q at index %d, want executor-list-a", got[i].ExecutorID, i)
		}
	}

	empty, err := repo.ListExecutorProfiles(ctx, "executor-list-nothing")
	if err != nil {
		t.Fatalf("ListExecutorProfiles(unknown executor): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("unknown executor returned %+v, want empty", empty)
	}
}

func TestListAllExecutorProfilesOrdersByExecutorThenName(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedExecutorForProfiles(t, repo, "executor-all-2")
	seedExecutorForProfiles(t, repo, "executor-all-1")

	// The repository seeds default profiles at init; measure that baseline
	// rather than hardcoding today's count.
	baseline, err := repo.ListAllExecutorProfiles(ctx)
	if err != nil {
		t.Fatalf("ListAllExecutorProfiles(baseline): %v", err)
	}
	seeded := len(baseline)

	rows := []struct {
		id, executorID, name string
	}{
		{"profile-all-2b", "executor-all-2", "beta"},
		{"profile-all-1b", "executor-all-1", "beta"},
		{"profile-all-2a", "executor-all-2", "alpha"},
		{"profile-all-1a", "executor-all-1", "alpha"},
	}
	for _, row := range rows {
		if err := repo.CreateExecutorProfile(ctx, &models.ExecutorProfile{
			ID: row.id, ExecutorID: row.executorID, Name: row.name,
			Config: map[string]string{"row": row.id},
		}); err != nil {
			t.Fatalf("CreateExecutorProfile(%s): %v", row.id, err)
		}
	}

	listed, err := repo.ListAllExecutorProfiles(ctx)
	if err != nil {
		t.Fatalf("ListAllExecutorProfiles: %v", err)
	}
	if len(listed) != seeded+len(rows) {
		t.Errorf("ListAllExecutorProfiles returned %d rows, want %d seeded + %d owned — ListAll must not filter",
			len(listed), seeded, len(rows))
	}

	// Assert on the relative order of the rows this test owns; the seeded
	// defaults are interleaved by executor id.
	ordered := make([]string, 0, len(rows))
	for _, profile := range listed {
		if strings.HasPrefix(profile.ID, "profile-all-") {
			ordered = append(ordered, profile.ID)
			if profile.Config["row"] != profile.ID {
				t.Errorf("Config for %s = %v, want per-row decoding", profile.ID, profile.Config)
			}
		}
	}
	want := []string{"profile-all-1a", "profile-all-1b", "profile-all-2a", "profile-all-2b"}
	if len(ordered) != len(want) {
		t.Fatalf("ListAllExecutorProfiles owned rows = %v, want %v", ordered, want)
	}
	for i := range want {
		if ordered[i] != want[i] {
			t.Fatalf("ListAllExecutorProfiles = %v, want %v (ORDER BY executor_id, name)", ordered, want)
		}
	}

}

func TestExecutorProfileEnvVarsSurviveListPaths(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedExecutorForProfiles(t, repo, "executor-envvars")

	want := []models.ProfileEnvVar{
		{Key: "PLAIN", Value: "visible"},
		{Key: "SECRET", SecretID: "secret-ref"},
		{Key: "EMPTY"},
	}
	if err := repo.CreateExecutorProfile(ctx, &models.ExecutorProfile{
		ID: "profile-envvars", ExecutorID: "executor-envvars", Name: "Env vars", EnvVars: want,
	}); err != nil {
		t.Fatalf("CreateExecutorProfile: %v", err)
	}

	scoped, err := repo.ListExecutorProfiles(ctx, "executor-envvars")
	if err != nil {
		t.Fatalf("ListExecutorProfiles: %v", err)
	}
	if len(scoped) != 1 {
		t.Fatalf("ListExecutorProfiles returned %d rows, want 1", len(scoped))
	}
	assertProfileEnvVars(t, "ListExecutorProfiles", scoped[0].EnvVars, want)

	all, err := repo.ListAllExecutorProfiles(ctx)
	if err != nil {
		t.Fatalf("ListAllExecutorProfiles: %v", err)
	}
	var found *models.ExecutorProfile
	for _, profile := range all {
		if profile.ID == "profile-envvars" {
			found = profile
		}
	}
	if found == nil {
		t.Fatal("profile-envvars missing from ListAllExecutorProfiles")
	}
	assertProfileEnvVars(t, "ListAllExecutorProfiles", found.EnvVars, want)
}

func assertProfileEnvVars(t *testing.T, label string, got, want []models.ProfileEnvVar) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s EnvVars = %+v, want %+v", label, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s EnvVars[%d] = %+v, want %+v", label, i, got[i], want[i])
		}
	}
}

func TestExecutorProfileMethodsPropagateBackendErrors(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedExecutorForProfiles(t, repo, "executor-broken")
	if err := repo.CreateExecutorProfile(ctx, &models.ExecutorProfile{
		ID: "profile-broken", ExecutorID: "executor-broken", Name: "Broken",
	}); err != nil {
		t.Fatalf("CreateExecutorProfile: %v", err)
	}
	closeRepoDB(t, repo)

	if err := repo.CreateExecutorProfile(ctx, &models.ExecutorProfile{
		ID: "profile-after-close", ExecutorID: "executor-broken", Name: "x",
	}); err == nil {
		t.Error("CreateExecutorProfile on a closed database returned nil")
	}
	if _, err := repo.GetExecutorProfile(ctx, "profile-broken"); err == nil {
		t.Error("GetExecutorProfile on a closed database returned nil")
	}
	if err := repo.UpdateExecutorProfile(ctx, &models.ExecutorProfile{ID: "profile-broken", Name: "x"}); err == nil {
		t.Error("UpdateExecutorProfile on a closed database returned nil")
	}
	if err := repo.DeleteExecutorProfile(ctx, "profile-broken"); err == nil {
		t.Error("DeleteExecutorProfile on a closed database returned nil")
	}
	if _, err := repo.ListExecutorProfiles(ctx, "executor-broken"); err == nil {
		t.Error("ListExecutorProfiles on a closed database returned nil")
	}
	if _, err := repo.ListAllExecutorProfiles(ctx); err == nil {
		t.Error("ListAllExecutorProfiles on a closed database returned nil")
	}
}
