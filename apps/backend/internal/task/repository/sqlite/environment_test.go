package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func assertEnvironmentEqual(t *testing.T, got, want *models.Environment) {
	t.Helper()
	if got == nil {
		t.Fatal("environment is nil")
	}
	if got.ID != want.ID {
		t.Errorf("ID = %q, want %q", got.ID, want.ID)
	}
	if got.Name != want.Name {
		t.Errorf("Name = %q, want %q", got.Name, want.Name)
	}
	if got.Kind != want.Kind {
		t.Errorf("Kind = %q, want %q", got.Kind, want.Kind)
	}
	if got.IsSystem != want.IsSystem {
		t.Errorf("IsSystem = %v, want %v", got.IsSystem, want.IsSystem)
	}
	if got.WorktreeRoot != want.WorktreeRoot {
		t.Errorf("WorktreeRoot = %q, want %q", got.WorktreeRoot, want.WorktreeRoot)
	}
	if got.ImageTag != want.ImageTag {
		t.Errorf("ImageTag = %q, want %q", got.ImageTag, want.ImageTag)
	}
	if got.Dockerfile != want.Dockerfile {
		t.Errorf("Dockerfile = %q, want %q", got.Dockerfile, want.Dockerfile)
	}
	if len(got.BuildConfig) != len(want.BuildConfig) {
		t.Errorf("BuildConfig = %v, want %v", got.BuildConfig, want.BuildConfig)
	}
	for key, value := range want.BuildConfig {
		if got.BuildConfig[key] != value {
			t.Errorf("BuildConfig[%q] = %q, want %q", key, got.BuildConfig[key], value)
		}
	}
	assertTimeEqual(t, "CreatedAt", got.CreatedAt, want.CreatedAt)
	assertTimeEqual(t, "UpdatedAt", got.UpdatedAt, want.UpdatedAt)
	if (got.DeletedAt == nil) != (want.DeletedAt == nil) {
		t.Errorf("DeletedAt = %v, want %v", got.DeletedAt, want.DeletedAt)
	}
}

func TestCreateEnvironmentRoundTripsEveryField(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()

	before := time.Now().UTC().Add(-time.Second)
	want := &models.Environment{
		ID:           "env-full",
		Name:         "Docker build env",
		Kind:         models.EnvironmentKindDockerImage,
		IsSystem:     true,
		WorktreeRoot: "~/kandev/worktrees",
		ImageTag:     "ghcr.io/kdlbs/kandev:latest",
		Dockerfile:   "FROM golang:1.26\nRUN echo hi\n",
		BuildConfig:  map[string]string{"context": ".", "target": "runtime"},
	}
	if err := repo.CreateEnvironment(ctx, want); err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	if want.CreatedAt.Before(before) || !want.CreatedAt.Equal(want.UpdatedAt) {
		t.Fatalf("CreateEnvironment stamped CreatedAt=%v UpdatedAt=%v, want equal times after %v",
			want.CreatedAt, want.UpdatedAt, before)
	}

	got, err := repo.GetEnvironment(ctx, "env-full")
	if err != nil {
		t.Fatalf("GetEnvironment: %v", err)
	}
	assertEnvironmentEqual(t, got, want)

	listed, err := repo.ListEnvironments(ctx)
	if err != nil {
		t.Fatalf("ListEnvironments: %v", err)
	}
	var found *models.Environment
	for _, environment := range listed {
		if environment.ID == "env-full" {
			found = environment
		}
	}
	if found == nil {
		t.Fatalf("ListEnvironments did not return env-full (got %d rows)", len(listed))
	}
	assertEnvironmentEqual(t, found, want)
}

func TestCreateEnvironmentGeneratesIDAndNormalizesEmptyBuildConfig(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()

	environment := &models.Environment{Name: "Plain", Kind: models.EnvironmentKindLocalPC}
	if err := repo.CreateEnvironment(ctx, environment); err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	if environment.ID == "" {
		t.Fatal("ID left empty; a UUID should have been generated")
	}
	got, err := repo.GetEnvironment(ctx, environment.ID)
	if err != nil {
		t.Fatalf("GetEnvironment: %v", err)
	}
	if got.IsSystem {
		t.Error("IsSystem = true, want false for an environment created without the flag")
	}
	if got.BuildConfig != nil {
		t.Errorf("BuildConfig = %v, want nil for an environment created without one", got.BuildConfig)
	}

	empty := &models.Environment{ID: "env-empty-config", Name: "Empty", Kind: models.EnvironmentKindLocalPC,
		BuildConfig: map[string]string{}}
	if err := repo.CreateEnvironment(ctx, empty); err != nil {
		t.Fatalf("CreateEnvironment(empty config): %v", err)
	}
	gotEmpty, err := repo.GetEnvironment(ctx, "env-empty-config")
	if err != nil {
		t.Fatalf("GetEnvironment: %v", err)
	}
	if gotEmpty.BuildConfig != nil {
		t.Errorf("BuildConfig = %v, want nil (an empty map serializes to the {} sentinel)", gotEmpty.BuildConfig)
	}
}

func TestGetEnvironmentReportsMissingID(t *testing.T) {
	repo := newRepoForEntityTests(t)

	got, err := repo.GetEnvironment(context.Background(), "env-nope")
	if got != nil {
		t.Errorf("environment = %+v, want nil", got)
	}
	if err == nil {
		t.Fatal("GetEnvironment on a missing id returned nil error")
	}
	if !strings.Contains(err.Error(), "environment not found: env-nope") {
		t.Errorf("error = %q, want an environment-not-found message naming the id", err)
	}
}

func TestUpdateEnvironmentWritesMutableFieldsAndReportsMissing(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()

	environment := &models.Environment{
		ID: "env-update", Name: "Before", Kind: models.EnvironmentKindLocalPC,
		WorktreeRoot: "/old", ImageTag: "old:tag", Dockerfile: "FROM old",
		BuildConfig: map[string]string{"a": "1"},
	}
	if err := repo.CreateEnvironment(ctx, environment); err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	createdAt := environment.CreatedAt

	environment.Name = "After"
	environment.Kind = models.EnvironmentKindDockerImage
	environment.IsSystem = true
	environment.WorktreeRoot = "/new"
	environment.ImageTag = "new:tag"
	environment.Dockerfile = "FROM new"
	environment.BuildConfig = map[string]string{"b": "2", "c": "3"}
	if err := repo.UpdateEnvironment(ctx, environment); err != nil {
		t.Fatalf("UpdateEnvironment: %v", err)
	}

	got, err := repo.GetEnvironment(ctx, "env-update")
	if err != nil {
		t.Fatalf("GetEnvironment: %v", err)
	}
	assertEnvironmentEqual(t, got, environment)
	if !got.CreatedAt.Equal(createdAt) {
		t.Errorf("CreatedAt = %v, want it preserved at %v", got.CreatedAt, createdAt)
	}
	if !got.UpdatedAt.After(createdAt) {
		t.Errorf("UpdatedAt = %v, want it bumped past CreatedAt %v", got.UpdatedAt, createdAt)
	}

	err = repo.UpdateEnvironment(ctx, &models.Environment{ID: "env-nowhere", Name: "x", Kind: models.EnvironmentKindLocalPC})
	if err == nil {
		t.Fatal("UpdateEnvironment on a missing row returned nil")
	}
	if !strings.Contains(err.Error(), "environment not found: env-nowhere") {
		t.Errorf("error = %q, want an environment-not-found message", err)
	}
}

func TestDeleteEnvironmentSoftDeletesAndHidesFromReads(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()

	if err := repo.CreateEnvironment(ctx, &models.Environment{
		ID: "env-soft-delete", Name: "Doomed", Kind: models.EnvironmentKindDockerImage,
	}); err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	if err := repo.CreateEnvironment(ctx, &models.Environment{
		ID: "env-survivor", Name: "Survivor", Kind: models.EnvironmentKindDockerImage,
	}); err != nil {
		t.Fatalf("CreateEnvironment(survivor): %v", err)
	}

	if err := repo.DeleteEnvironment(ctx, "env-soft-delete"); err != nil {
		t.Fatalf("DeleteEnvironment: %v", err)
	}

	// Soft delete: the row survives with deleted_at set.
	if got := countRows(t, repo, `SELECT COUNT(1) FROM environments WHERE id = ?`, "env-soft-delete"); got != 1 {
		t.Errorf("rows for the deleted environment = %d, want 1 (soft delete keeps the row)", got)
	}
	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM environments WHERE id = ? AND deleted_at IS NOT NULL`, "env-soft-delete"); got != 1 {
		t.Errorf("deleted_at-set rows = %d, want 1", got)
	}

	if _, err := repo.GetEnvironment(ctx, "env-soft-delete"); err == nil {
		t.Error("GetEnvironment returned a soft-deleted environment")
	}
	listed, err := repo.ListEnvironments(ctx)
	if err != nil {
		t.Fatalf("ListEnvironments: %v", err)
	}
	for _, environment := range listed {
		if environment.ID == "env-soft-delete" {
			t.Fatalf("ListEnvironments returned the soft-deleted row: %+v", environment)
		}
	}
	if _, err := repo.GetEnvironment(ctx, "env-survivor"); err != nil {
		t.Errorf("GetEnvironment(env-survivor): %v", err)
	}

	// A soft-deleted row can neither be updated nor deleted again.
	err = repo.UpdateEnvironment(ctx, &models.Environment{
		ID: "env-soft-delete", Name: "Resurrect", Kind: models.EnvironmentKindLocalPC,
	})
	if err == nil || !strings.Contains(err.Error(), "environment not found") {
		t.Errorf("UpdateEnvironment on a soft-deleted row = %v, want an environment-not-found error", err)
	}
	err = repo.DeleteEnvironment(ctx, "env-soft-delete")
	if err == nil || !strings.Contains(err.Error(), "environment not found") {
		t.Errorf("second DeleteEnvironment = %v, want an environment-not-found error", err)
	}
	err = repo.DeleteEnvironment(ctx, "env-never-existed")
	if err == nil || !strings.Contains(err.Error(), "environment not found") {
		t.Errorf("DeleteEnvironment(unknown) = %v, want an environment-not-found error", err)
	}
}

func TestListEnvironmentsOrdersByCreatedAtAscending(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()

	// CreateEnvironment stamps created_at itself, so pin explicit values
	// afterwards to make the ORDER BY assertion deterministic.
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	stamps := map[string]time.Time{
		"env-order-c": base.Add(3 * time.Hour),
		"env-order-a": base.Add(1 * time.Hour),
		"env-order-b": base.Add(2 * time.Hour),
	}
	for id, stamp := range stamps {
		if err := repo.CreateEnvironment(ctx, &models.Environment{
			ID: id, Name: id, Kind: models.EnvironmentKindDockerImage,
		}); err != nil {
			t.Fatalf("CreateEnvironment(%s): %v", id, err)
		}
		if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
			`UPDATE environments SET created_at = ? WHERE id = ?`), stamp, id); err != nil {
			t.Fatalf("pin created_at for %s: %v", id, err)
		}
	}
	// Push the seeded default environment before all three.
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE environments SET created_at = ? WHERE id = ?`), base, models.EnvironmentIDLocal); err != nil {
		t.Fatalf("pin created_at for the default environment: %v", err)
	}

	listed, err := repo.ListEnvironments(ctx)
	if err != nil {
		t.Fatalf("ListEnvironments: %v", err)
	}
	gotIDs := make([]string, 0, len(listed))
	for _, environment := range listed {
		gotIDs = append(gotIDs, environment.ID)
	}
	want := []string{models.EnvironmentIDLocal, "env-order-a", "env-order-b", "env-order-c"}
	if len(gotIDs) != len(want) {
		t.Fatalf("ListEnvironments = %v, want %v", gotIDs, want)
	}
	for i := range want {
		if gotIDs[i] != want[i] {
			t.Fatalf("ListEnvironments = %v, want %v (ORDER BY created_at ASC)", gotIDs, want)
		}
	}
}

func TestListEnvironmentsDecodesBuildConfigPerRow(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()

	if err := repo.CreateEnvironment(ctx, &models.Environment{
		ID: "env-decode-1", Name: "One", Kind: models.EnvironmentKindDockerImage, IsSystem: true,
		BuildConfig: map[string]string{"one": "1"},
	}); err != nil {
		t.Fatalf("CreateEnvironment(one): %v", err)
	}
	if err := repo.CreateEnvironment(ctx, &models.Environment{
		ID: "env-decode-2", Name: "Two", Kind: models.EnvironmentKindDockerImage,
		BuildConfig: map[string]string{"two": "2", "three": "3"},
	}); err != nil {
		t.Fatalf("CreateEnvironment(two): %v", err)
	}

	listed, err := repo.ListEnvironments(ctx)
	if err != nil {
		t.Fatalf("ListEnvironments: %v", err)
	}
	byID := make(map[string]*models.Environment, len(listed))
	for _, environment := range listed {
		byID[environment.ID] = environment
	}
	one, ok := byID["env-decode-1"]
	if !ok {
		t.Fatal("env-decode-1 missing from ListEnvironments")
	}
	if !one.IsSystem {
		t.Error("env-decode-1 IsSystem = false, want true")
	}
	if len(one.BuildConfig) != 1 || one.BuildConfig["one"] != "1" {
		t.Errorf("env-decode-1 BuildConfig = %v, want {one:1}", one.BuildConfig)
	}
	two, ok := byID["env-decode-2"]
	if !ok {
		t.Fatal("env-decode-2 missing from ListEnvironments")
	}
	if two.IsSystem {
		t.Error("env-decode-2 IsSystem = true, want false")
	}
	if len(two.BuildConfig) != 2 || two.BuildConfig["two"] != "2" || two.BuildConfig["three"] != "3" {
		t.Errorf("env-decode-2 BuildConfig = %v, want {two:2, three:3}", two.BuildConfig)
	}
}

// closeRepoDB closes the repository's shared connection so the next call must
// surface a backend error instead of silently returning a zero value.
func closeRepoDB(t *testing.T, repo *Repository) {
	t.Helper()
	if err := repo.db.Close(); err != nil {
		t.Fatalf("close repo db: %v", err)
	}
}

func TestEnvironmentMethodsPropagateBackendErrors(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	if err := repo.CreateEnvironment(ctx, &models.Environment{
		ID: "env-broken", Name: "Broken", Kind: models.EnvironmentKindLocalPC,
	}); err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	closeRepoDB(t, repo)

	if err := repo.CreateEnvironment(ctx, &models.Environment{
		ID: "env-after-close", Name: "x", Kind: models.EnvironmentKindLocalPC,
	}); err == nil {
		t.Error("CreateEnvironment on a closed database returned nil")
	}
	if _, err := repo.GetEnvironment(ctx, "env-broken"); err == nil {
		t.Error("GetEnvironment on a closed database returned nil")
	}
	if err := repo.UpdateEnvironment(ctx, &models.Environment{
		ID: "env-broken", Name: "x", Kind: models.EnvironmentKindLocalPC,
	}); err == nil {
		t.Error("UpdateEnvironment on a closed database returned nil")
	}
	if err := repo.DeleteEnvironment(ctx, "env-broken"); err == nil {
		t.Error("DeleteEnvironment on a closed database returned nil")
	}
	if _, err := repo.ListEnvironments(ctx); err == nil {
		t.Error("ListEnvironments on a closed database returned nil")
	}
}
