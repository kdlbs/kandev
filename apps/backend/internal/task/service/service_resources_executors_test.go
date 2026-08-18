package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func createTestExecutor(t *testing.T, svc *Service, name string, executorType models.ExecutorType) *models.Executor {
	t.Helper()
	executor, err := svc.CreateExecutor(context.Background(), &CreateExecutorRequest{
		Name: name, Type: executorType, Status: models.ExecutorStatusActive, Resumable: true,
	})
	if err != nil {
		t.Fatalf("CreateExecutor %s: %v", name, err)
	}
	return executor
}

func TestCreateExecutorPersistsAndPublishes(t *testing.T) {
	svc, bus, repo := createTestService(t)
	ctx := context.Background()
	bus.ClearEvents()

	executor, err := svc.CreateExecutor(ctx, &CreateExecutorRequest{
		Name: "Local runner", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive,
		Resumable: true, Config: map[string]string{"mcp_policy": `{"allow":["fs"]}`},
	})
	if err != nil {
		t.Fatalf("CreateExecutor: %v", err)
	}
	if executor.ID == "" || executor.Name != "Local runner" || executor.Type != models.ExecutorTypeLocal {
		t.Fatalf("executor = %+v", executor)
	}
	if !executor.Resumable || executor.Status != models.ExecutorStatusActive {
		t.Fatalf("executor flags = %+v", executor)
	}

	stored, err := repo.GetExecutor(ctx, executor.ID)
	if err != nil {
		t.Fatalf("GetExecutor: %v", err)
	}
	if stored.Config["mcp_policy"] != `{"allow":["fs"]}` {
		t.Fatalf("stored config = %v", stored.Config)
	}
	if types := eventTypes(bus.GetPublishedEvents()); len(types) != 1 || types[0] != events.ExecutorCreated {
		t.Fatalf("published %v, want exactly one %s", types, events.ExecutorCreated)
	}
}

func TestCreateExecutorRejectsInvalidMCPPolicy(t *testing.T) {
	svc, bus, _ := createTestService(t)
	ctx := context.Background()
	bus.ClearEvents()

	for name, policy := range map[string]string{
		"not json":      "{oops",
		"not an object": `["a"]`,
	} {
		_, err := svc.CreateExecutor(ctx, &CreateExecutorRequest{
			Name: "bad", Type: models.ExecutorTypeLocal, Config: map[string]string{"mcp_policy": policy},
		})
		if !errors.Is(err, ErrInvalidExecutorConfig) {
			t.Fatalf("%s: error = %v, want ErrInvalidExecutorConfig", name, err)
		}
	}
	if published := bus.GetPublishedEvents(); len(published) != 0 {
		t.Fatalf("rejected create published %d events, want none", len(published))
	}
}

func TestUpdateExecutorAppliesOnlySuppliedFields(t *testing.T) {
	svc, bus, _ := createTestService(t)
	ctx := context.Background()
	executor := createTestExecutor(t, svc, "Runner", models.ExecutorTypeLocal)
	bus.ClearEvents()

	newName := "Renamed runner"
	newStatus := models.ExecutorStatusDisabled
	updated, err := svc.UpdateExecutor(ctx, executor.ID, &UpdateExecutorRequest{
		Name: &newName, Status: &newStatus,
	})
	if err != nil {
		t.Fatalf("UpdateExecutor: %v", err)
	}
	if updated.Name != newName || updated.Status != newStatus {
		t.Fatalf("updated = %+v", updated)
	}
	if updated.Type != models.ExecutorTypeLocal || !updated.Resumable {
		t.Fatalf("unsupplied fields changed: %+v", updated)
	}

	reloaded, err := svc.GetExecutor(ctx, executor.ID)
	if err != nil {
		t.Fatalf("GetExecutor: %v", err)
	}
	if reloaded.Name != newName || reloaded.Status != newStatus {
		t.Fatalf("persisted executor = %+v", reloaded)
	}
	if types := eventTypes(bus.GetPublishedEvents()); len(types) != 1 || types[0] != events.ExecutorUpdated {
		t.Fatalf("published %v, want exactly one %s", types, events.ExecutorUpdated)
	}
}

func TestUpdateExecutorRejectsInvalidConfigAndMissingExecutor(t *testing.T) {
	svc, _, _ := createTestService(t)
	ctx := context.Background()
	executor := createTestExecutor(t, svc, "Runner", models.ExecutorTypeLocal)

	if _, err := svc.UpdateExecutor(ctx, executor.ID, &UpdateExecutorRequest{
		Config: map[string]string{"mcp_policy": "{oops"},
	}); !errors.Is(err, ErrInvalidExecutorConfig) {
		t.Fatalf("invalid config = %v, want ErrInvalidExecutorConfig", err)
	}
	if _, err := svc.UpdateExecutor(ctx, "no-such-executor", &UpdateExecutorRequest{}); err == nil {
		t.Fatal("updating a missing executor must fail")
	}
}

func TestSystemExecutorsAreImmutableAndUndeletable(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	system := &models.Executor{
		ID: "exec-system", Name: "System", Type: models.ExecutorTypeLocal,
		Status: models.ExecutorStatusActive, IsSystem: true, Resumable: true,
	}
	if err := repo.CreateExecutor(ctx, system); err != nil {
		t.Fatalf("seed system executor: %v", err)
	}

	otherName := "Hijacked"
	otherType := models.ExecutorTypeLocalDocker
	otherStatus := models.ExecutorStatusDisabled
	otherResumable := false
	rejections := map[string]*UpdateExecutorRequest{
		"name":      {Name: &otherName},
		"type":      {Type: &otherType},
		"status":    {Status: &otherStatus},
		"resumable": {Resumable: &otherResumable},
	}
	for field, req := range rejections {
		if _, err := svc.UpdateExecutor(ctx, system.ID, req); err == nil ||
			!strings.Contains(err.Error(), "system executors cannot be modified") {
			t.Fatalf("%s update error = %v, want a system-executor rejection", field, err)
		}
	}

	// A no-op update that matches the current values is allowed through.
	sameName := system.Name
	if _, err := svc.UpdateExecutor(ctx, system.ID, &UpdateExecutorRequest{Name: &sameName}); err != nil {
		t.Fatalf("identical-value update on a system executor: %v", err)
	}

	if err := svc.DeleteExecutor(ctx, system.ID); err == nil ||
		!strings.Contains(err.Error(), "system executors cannot be deleted") {
		t.Fatalf("delete error = %v, want a system-executor rejection", err)
	}
}

func TestDeleteExecutorBlocksOnActiveSessions(t *testing.T) {
	svc, bus, repo := createTestService(t)
	ctx := context.Background()
	executor := createTestExecutor(t, svc, "Runner", models.ExecutorTypeLocal)
	seedExecutorSession(t, repo, "exec-active", executor.ID)
	bus.ClearEvents()

	if err := svc.DeleteExecutor(ctx, executor.ID); !errors.Is(err, ErrActiveTaskSessions) {
		t.Fatalf("delete with an active session = %v, want ErrActiveTaskSessions", err)
	}
	if _, err := svc.GetExecutor(ctx, executor.ID); err != nil {
		t.Fatalf("blocked delete must leave the executor: %v", err)
	}
	if published := bus.GetPublishedEvents(); len(published) != 0 {
		t.Fatalf("blocked delete published %d events, want none", len(published))
	}
}

func TestDeleteExecutorRemovesIdleExecutor(t *testing.T) {
	svc, bus, _ := createTestService(t)
	ctx := context.Background()
	executor := createTestExecutor(t, svc, "Idle runner", models.ExecutorTypeLocal)
	bus.ClearEvents()

	if err := svc.DeleteExecutor(ctx, executor.ID); err != nil {
		t.Fatalf("DeleteExecutor: %v", err)
	}
	if _, err := svc.GetExecutor(ctx, executor.ID); err == nil {
		t.Fatal("deleted executor must not be readable")
	}
	if types := eventTypes(bus.GetPublishedEvents()); len(types) != 1 || types[0] != events.ExecutorDeleted {
		t.Fatalf("published %v, want exactly one %s", types, events.ExecutorDeleted)
	}
	if err := svc.DeleteExecutor(ctx, "no-such-executor"); err == nil {
		t.Fatal("deleting a missing executor must fail")
	}
}

func TestListExecutorsReturnsCreatedExecutors(t *testing.T) {
	svc, _, _ := createTestService(t)
	ctx := context.Background()
	before, err := svc.ListExecutors(ctx)
	if err != nil {
		t.Fatalf("ListExecutors: %v", err)
	}
	created := createTestExecutor(t, svc, "Listed", models.ExecutorTypeLocal)

	after, err := svc.ListExecutors(ctx)
	if err != nil {
		t.Fatalf("ListExecutors: %v", err)
	}
	if len(after) != len(before)+1 {
		t.Fatalf("list grew from %d to %d, want exactly one more", len(before), len(after))
	}
	found := false
	for _, executor := range after {
		if executor.ID == created.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("created executor %s missing from the list", created.ID)
	}
}

// seedExecutorSession attaches an active session to an executor so the
// delete-blocking checks have something to find.
func seedExecutorSession(t *testing.T, repo *sqliterepo.Repository, sessionID, executorID string) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-" + sessionID, Name: sessionID}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-" + sessionID, WorkspaceID: "ws-" + sessionID, Name: sessionID}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-" + sessionID, WorkspaceID: "ws-" + sessionID, WorkflowID: "wf-" + sessionID,
		WorkflowStepID: "step-1", Title: sessionID, State: v1.TaskStateCreated, Priority: "medium",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: "task-" + sessionID, ExecutorID: executorID,
		State: models.TaskSessionStateRunning,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
}

func TestCreateExecutorProfileValidatesRequest(t *testing.T) {
	svc, bus, _ := createTestService(t)
	ctx := context.Background()
	executor := createTestExecutor(t, svc, "Runner", models.ExecutorTypeLocal)
	bus.ClearEvents()

	if _, err := svc.CreateExecutorProfile(ctx, &CreateExecutorProfileRequest{ExecutorID: executor.ID}); err == nil ||
		!strings.Contains(err.Error(), "profile name is required") {
		t.Fatalf("missing name error = %v", err)
	}
	if _, err := svc.CreateExecutorProfile(ctx, &CreateExecutorProfileRequest{Name: "p"}); err == nil ||
		!strings.Contains(err.Error(), "executor_id is required") {
		t.Fatalf("missing executor error = %v", err)
	}
	if _, err := svc.CreateExecutorProfile(ctx, &CreateExecutorProfileRequest{
		Name: "p", ExecutorID: "no-such-executor",
	}); err == nil || !strings.Contains(err.Error(), "executor not found") {
		t.Fatalf("unknown executor error = %v", err)
	}
	if published := bus.GetPublishedEvents(); len(published) != 0 {
		t.Fatalf("rejected creates published %d events, want none", len(published))
	}
}

func TestCreateExecutorProfilePersistsAndPublishes(t *testing.T) {
	svc, bus, repo := createTestService(t)
	ctx := context.Background()
	executor := createTestExecutor(t, svc, "Runner", models.ExecutorTypeLocal)
	bus.ClearEvents()

	profile, err := svc.CreateExecutorProfile(ctx, &CreateExecutorProfileRequest{
		ExecutorID: executor.ID, Name: "default", McpPolicy: "allow",
		PrepareScript: "make setup", CleanupScript: "make clean",
		Config:  map[string]string{"cpu": "2"},
		EnvVars: []models.ProfileEnvVar{{Key: "FOO", Value: "bar"}},
	})
	if err != nil {
		t.Fatalf("CreateExecutorProfile: %v", err)
	}
	if profile.Name != "default" || profile.ExecutorID != executor.ID {
		t.Fatalf("profile = %+v", profile)
	}

	stored, err := repo.GetExecutorProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("GetExecutorProfile: %v", err)
	}
	if stored.PrepareScript != "make setup" || stored.CleanupScript != "make clean" {
		t.Fatalf("stored scripts = %q/%q", stored.PrepareScript, stored.CleanupScript)
	}
	if len(stored.EnvVars) != 1 || stored.EnvVars[0].Key != "FOO" || stored.EnvVars[0].Value != "bar" {
		t.Fatalf("stored env vars = %+v", stored.EnvVars)
	}
	if types := eventTypes(bus.GetPublishedEvents()); len(types) != 1 || types[0] != events.ExecutorProfileCreated {
		t.Fatalf("published %v, want exactly one %s", types, events.ExecutorProfileCreated)
	}
}

func TestCreateExecutorProfileRequiresSpritesToken(t *testing.T) {
	svc, _, _ := createTestService(t)
	ctx := context.Background()
	sprites := createTestExecutor(t, svc, "Sprites", models.ExecutorTypeSprites)

	if _, err := svc.CreateExecutorProfile(ctx, &CreateExecutorProfileRequest{
		ExecutorID: sprites.ID, Name: "sprites profile",
	}); err == nil || !strings.Contains(err.Error(), spritesTokenEnvKey) {
		t.Fatalf("missing token error = %v, want a %s requirement", err, spritesTokenEnvKey)
	}
	// A blank-valued token entry does not satisfy the requirement.
	if _, err := svc.CreateExecutorProfile(ctx, &CreateExecutorProfileRequest{
		ExecutorID: sprites.ID, Name: "sprites profile",
		EnvVars: []models.ProfileEnvVar{{Key: spritesTokenEnvKey, Value: "  "}},
	}); err == nil {
		t.Fatal("a blank sprites token must be rejected")
	}

	profile, err := svc.CreateExecutorProfile(ctx, &CreateExecutorProfileRequest{
		ExecutorID: sprites.ID, Name: "sprites profile",
		EnvVars: []models.ProfileEnvVar{{Key: spritesTokenEnvKey, Value: "tok"}},
	})
	if err != nil {
		t.Fatalf("CreateExecutorProfile with a token: %v", err)
	}
	if len(profile.EnvVars) != 1 {
		t.Fatalf("env vars = %+v", profile.EnvVars)
	}
}

func TestHasSpritesTokenAcceptsSecretReference(t *testing.T) {
	cases := []struct {
		name    string
		envVars []models.ProfileEnvVar
		want    bool
	}{
		{"empty", nil, false},
		{"other key", []models.ProfileEnvVar{{Key: "FOO", Value: "bar"}}, false},
		{"blank value", []models.ProfileEnvVar{{Key: spritesTokenEnvKey, Value: " "}}, false},
		{"literal value", []models.ProfileEnvVar{{Key: spritesTokenEnvKey, Value: "tok"}}, true},
		{"secret reference", []models.ProfileEnvVar{{Key: spritesTokenEnvKey, SecretID: "secret-1"}}, true},
	}
	for _, tc := range cases {
		if got := hasSpritesToken(tc.envVars); got != tc.want {
			t.Fatalf("%s: hasSpritesToken = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestMergeSpritesTokenEnvVarsPreservesUnlessExplicitlyRemoved(t *testing.T) {
	existing := []models.ProfileEnvVar{
		{Key: "OTHER", Value: "keep-me"},
		{Key: spritesTokenEnvKey, Value: "existing-token"},
	}

	// An update that omits the token keeps the stored one.
	merged := mergeSpritesTokenEnvVars(existing, []models.ProfileEnvVar{{Key: "NEW", Value: "1"}})
	if len(merged) != 2 || merged[0].Key != "NEW" || merged[1].Key != spritesTokenEnvKey ||
		merged[1].Value != "existing-token" {
		t.Fatalf("merged = %+v, want the existing token carried over", merged)
	}

	// An explicit blank entry removes it.
	merged = mergeSpritesTokenEnvVars(existing, []models.ProfileEnvVar{{Key: spritesTokenEnvKey, Value: ""}})
	if len(merged) != 0 {
		t.Fatalf("merged = %+v, want the token explicitly removed", merged)
	}

	// An incoming token wins over the stored one.
	merged = mergeSpritesTokenEnvVars(existing, []models.ProfileEnvVar{{Key: spritesTokenEnvKey, Value: "new-token"}})
	if len(merged) != 1 || merged[0].Value != "new-token" {
		t.Fatalf("merged = %+v, want the incoming token", merged)
	}

	// With no stored token there is nothing to preserve.
	merged = mergeSpritesTokenEnvVars(nil, []models.ProfileEnvVar{{Key: "A", Value: "1"}})
	if len(merged) != 1 || merged[0].Key != "A" {
		t.Fatalf("merged = %+v", merged)
	}
}

func TestUpdateExecutorProfileAppliesSuppliedFields(t *testing.T) {
	svc, bus, repo := createTestService(t)
	ctx := context.Background()
	executor := createTestExecutor(t, svc, "Runner", models.ExecutorTypeLocal)
	profile, err := svc.CreateExecutorProfile(ctx, &CreateExecutorProfileRequest{
		ExecutorID: executor.ID, Name: "default", PrepareScript: "old prepare",
	})
	if err != nil {
		t.Fatalf("CreateExecutorProfile: %v", err)
	}
	bus.ClearEvents()

	name := "renamed"
	policy := "deny"
	prepare := "new prepare"
	cleanup := "new cleanup"
	updated, err := svc.UpdateExecutorProfile(ctx, profile.ID, &UpdateExecutorProfileRequest{
		Name: &name, McpPolicy: &policy, PrepareScript: &prepare, CleanupScript: &cleanup,
		Config:  map[string]string{"cpu": "4"},
		EnvVars: []models.ProfileEnvVar{{Key: "BAR", Value: "baz"}},
	})
	if err != nil {
		t.Fatalf("UpdateExecutorProfile: %v", err)
	}
	if updated.Name != name || updated.McpPolicy != policy ||
		updated.PrepareScript != prepare || updated.CleanupScript != cleanup {
		t.Fatalf("updated profile = %+v", updated)
	}

	stored, err := repo.GetExecutorProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("GetExecutorProfile: %v", err)
	}
	if stored.Config["cpu"] != "4" || len(stored.EnvVars) != 1 || stored.EnvVars[0].Key != "BAR" {
		t.Fatalf("stored profile = %+v", stored)
	}
	if types := eventTypes(bus.GetPublishedEvents()); len(types) != 1 || types[0] != events.ExecutorProfileUpdated {
		t.Fatalf("published %v, want exactly one %s", types, events.ExecutorProfileUpdated)
	}

	if _, err := svc.UpdateExecutorProfile(ctx, "no-such-profile", &UpdateExecutorProfileRequest{}); err == nil {
		t.Fatal("updating a missing profile must fail")
	}
}

func TestUpdateExecutorProfileKeepsSpritesTokenWhenOmitted(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	sprites := createTestExecutor(t, svc, "Sprites", models.ExecutorTypeSprites)
	profile, err := svc.CreateExecutorProfile(ctx, &CreateExecutorProfileRequest{
		ExecutorID: sprites.ID, Name: "sprites profile",
		EnvVars: []models.ProfileEnvVar{{Key: spritesTokenEnvKey, Value: "tok"}},
	})
	if err != nil {
		t.Fatalf("CreateExecutorProfile: %v", err)
	}

	updated, err := svc.UpdateExecutorProfile(ctx, profile.ID, &UpdateExecutorProfileRequest{
		EnvVars: []models.ProfileEnvVar{{Key: "OTHER", Value: "1"}},
	})
	if err != nil {
		t.Fatalf("UpdateExecutorProfile: %v", err)
	}
	if len(updated.EnvVars) != 2 {
		t.Fatalf("env vars = %+v, want the sprites token preserved alongside OTHER", updated.EnvVars)
	}
	stored, err := repo.GetExecutorProfile(ctx, profile.ID)
	if err != nil {
		t.Fatalf("GetExecutorProfile: %v", err)
	}
	if !hasSpritesToken(stored.EnvVars) {
		t.Fatalf("stored env vars = %+v, want the sprites token preserved", stored.EnvVars)
	}
}

func TestDeleteAndListExecutorProfiles(t *testing.T) {
	svc, bus, _ := createTestService(t)
	ctx := context.Background()
	executor := createTestExecutor(t, svc, "Runner", models.ExecutorTypeLocal)
	profile, err := svc.CreateExecutorProfile(ctx, &CreateExecutorProfileRequest{
		ExecutorID: executor.ID, Name: "default",
	})
	if err != nil {
		t.Fatalf("CreateExecutorProfile: %v", err)
	}

	scoped, err := svc.ListExecutorProfiles(ctx, executor.ID)
	if err != nil {
		t.Fatalf("ListExecutorProfiles: %v", err)
	}
	if len(scoped) != 1 || scoped[0].ID != profile.ID {
		t.Fatalf("profiles for executor = %+v, want just the created one", scoped)
	}
	all, err := svc.ListAllExecutorProfiles(ctx)
	if err != nil {
		t.Fatalf("ListAllExecutorProfiles: %v", err)
	}
	if len(all) < 1 {
		t.Fatal("ListAllExecutorProfiles returned nothing")
	}

	bus.ClearEvents()
	if err := svc.DeleteExecutorProfile(ctx, profile.ID); err != nil {
		t.Fatalf("DeleteExecutorProfile: %v", err)
	}
	if _, err := svc.GetExecutorProfile(ctx, profile.ID); err == nil {
		t.Fatal("deleted profile must not be readable")
	}
	if types := eventTypes(bus.GetPublishedEvents()); len(types) != 1 || types[0] != events.ExecutorProfileDeleted {
		t.Fatalf("published %v, want exactly one %s", types, events.ExecutorProfileDeleted)
	}
	if err := svc.DeleteExecutorProfile(ctx, "no-such-profile"); err == nil {
		t.Fatal("deleting a missing profile must fail")
	}
}

func TestEnvironmentCRUDPublishesEvents(t *testing.T) {
	svc, bus, _ := createTestService(t)
	ctx := context.Background()
	bus.ClearEvents()

	environment, err := svc.CreateEnvironment(ctx, &CreateEnvironmentRequest{
		Name: "docker env", Kind: models.EnvironmentKindDockerImage,
		WorktreeRoot: "/tmp/worktrees", ImageTag: "kandev:test", Dockerfile: "FROM alpine",
		BuildConfig: map[string]string{"target": "dev"},
	})
	if err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	if environment.ID == "" || environment.IsSystem {
		t.Fatalf("environment = %+v, want a non-system row with an id", environment)
	}
	if types := eventTypes(bus.GetPublishedEvents()); len(types) != 1 || types[0] != events.EnvironmentCreated {
		t.Fatalf("published %v, want exactly one %s", types, events.EnvironmentCreated)
	}

	fetched, err := svc.GetEnvironment(ctx, environment.ID)
	if err != nil {
		t.Fatalf("GetEnvironment: %v", err)
	}
	if fetched.ImageTag != "kandev:test" || fetched.BuildConfig["target"] != "dev" {
		t.Fatalf("fetched = %+v", fetched)
	}

	bus.ClearEvents()
	name := "renamed env"
	root := "/tmp/other"
	tag := "kandev:next"
	updated, err := svc.UpdateEnvironment(ctx, environment.ID, &UpdateEnvironmentRequest{
		Name: &name, WorktreeRoot: &root, ImageTag: &tag,
	})
	if err != nil {
		t.Fatalf("UpdateEnvironment: %v", err)
	}
	if updated.Name != name || updated.WorktreeRoot != root || updated.ImageTag != tag {
		t.Fatalf("updated = %+v", updated)
	}
	if updated.Dockerfile != "FROM alpine" {
		t.Fatalf("unsupplied dockerfile changed to %q", updated.Dockerfile)
	}
	if types := eventTypes(bus.GetPublishedEvents()); len(types) != 1 || types[0] != events.EnvironmentUpdated {
		t.Fatalf("published %v, want exactly one %s", types, events.EnvironmentUpdated)
	}

	listed, err := svc.ListEnvironments(ctx)
	if err != nil {
		t.Fatalf("ListEnvironments: %v", err)
	}
	found := false
	for _, env := range listed {
		if env.ID == environment.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("created environment missing from %d listed environments", len(listed))
	}

	bus.ClearEvents()
	if err := svc.DeleteEnvironment(ctx, environment.ID); err != nil {
		t.Fatalf("DeleteEnvironment: %v", err)
	}
	if types := eventTypes(bus.GetPublishedEvents()); len(types) != 1 || types[0] != events.EnvironmentDeleted {
		t.Fatalf("published %v, want exactly one %s", types, events.EnvironmentDeleted)
	}
	if _, err := svc.GetEnvironment(ctx, environment.ID); err == nil {
		t.Fatal("deleted environment must not be readable")
	}
	if err := svc.DeleteEnvironment(ctx, "no-such-environment"); err == nil {
		t.Fatal("deleting a missing environment must fail")
	}
	if _, err := svc.UpdateEnvironment(ctx, "no-such-environment", &UpdateEnvironmentRequest{}); err == nil {
		t.Fatal("updating a missing environment must fail")
	}
}

func TestSystemEnvironmentAllowsOnlyWorktreeRootUpdates(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	system := &models.Environment{
		ID: "env-system", Name: "System", Kind: models.EnvironmentKindLocalPC, IsSystem: true,
	}
	if err := repo.CreateEnvironment(ctx, system); err != nil {
		t.Fatalf("seed system environment: %v", err)
	}

	name := "renamed"
	kind := models.EnvironmentKindDockerImage
	tag := "img"
	dockerfile := "FROM alpine"
	rejections := map[string]*UpdateEnvironmentRequest{
		"name":         {Name: &name},
		"kind":         {Kind: &kind},
		"image tag":    {ImageTag: &tag},
		"dockerfile":   {Dockerfile: &dockerfile},
		"build config": {BuildConfig: map[string]string{"a": "b"}},
	}
	for field, req := range rejections {
		if _, err := svc.UpdateEnvironment(ctx, system.ID, req); err == nil ||
			!strings.Contains(err.Error(), "system environments can only update the worktree root") {
			t.Fatalf("%s update error = %v, want a system-environment rejection", field, err)
		}
	}

	root := "/tmp/system-worktrees"
	updated, err := svc.UpdateEnvironment(ctx, system.ID, &UpdateEnvironmentRequest{WorktreeRoot: &root})
	if err != nil {
		t.Fatalf("worktree root update on a system environment: %v", err)
	}
	if updated.WorktreeRoot != root {
		t.Fatalf("worktree root = %q, want %q", updated.WorktreeRoot, root)
	}

	if err := svc.DeleteEnvironment(ctx, system.ID); err == nil ||
		!strings.Contains(err.Error(), "system environments cannot be deleted") {
		t.Fatalf("delete error = %v, want a system-environment rejection", err)
	}
}

func TestDeleteEnvironmentBlocksOnActiveSessions(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	environment, err := svc.CreateEnvironment(ctx, &CreateEnvironmentRequest{
		Name: "busy", Kind: models.EnvironmentKindLocalPC,
	})
	if err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	seedEnvironmentSession(t, repo, "env-busy-session", environment.ID)

	if err := svc.DeleteEnvironment(ctx, environment.ID); !errors.Is(err, ErrActiveTaskSessions) {
		t.Fatalf("delete with an active session = %v, want ErrActiveTaskSessions", err)
	}
	if _, err := svc.GetEnvironment(ctx, environment.ID); err != nil {
		t.Fatalf("blocked delete must leave the environment: %v", err)
	}
}

func seedEnvironmentSession(t *testing.T, repo *sqliterepo.Repository, sessionID, environmentID string) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-" + sessionID, Name: sessionID}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-" + sessionID, WorkspaceID: "ws-" + sessionID, Name: sessionID}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-" + sessionID, WorkspaceID: "ws-" + sessionID, WorkflowID: "wf-" + sessionID,
		WorkflowStepID: "step-1", Title: sessionID, State: v1.TaskStateCreated, Priority: "medium",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: "task-" + sessionID, EnvironmentID: environmentID,
		State: models.TaskSessionStateRunning,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
}

func TestRepositoryScriptUpdateDeleteAreWorkspaceScoped(t *testing.T) {
	svc, bus, repo := createTestService(t)
	seedScopedWorkspaces(t, repo)
	ctx := context.Background()
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-b", WorkspaceID: "ws-b", Name: "b"}); err != nil {
		t.Fatalf("seed repository: %v", err)
	}
	script, err := svc.CreateRepositoryScript(ctxAs("user-b"), &CreateRepositoryScriptRequest{
		RepositoryID: "repo-b", Name: "setup", Command: "make", Position: 1,
	})
	if err != nil {
		t.Fatalf("CreateRepositoryScript: %v", err)
	}
	bus.ClearEvents()

	hijacked := "evil"
	if _, err := svc.UpdateRepositoryScript(ctxAs("user-a"), script.ID, &UpdateRepositoryScriptRequest{
		Command: &hijacked,
	}); !errors.Is(err, repoerrors.ErrRepositoryNotFound) {
		t.Fatalf("foreign update = %v, want ErrRepositoryNotFound", err)
	}
	if err := svc.DeleteRepositoryScript(ctxAs("user-a"), script.ID); !errors.Is(err, repoerrors.ErrRepositoryNotFound) {
		t.Fatalf("foreign delete = %v, want ErrRepositoryNotFound", err)
	}
	if published := bus.GetPublishedEvents(); len(published) != 0 {
		t.Fatalf("denied script mutations published %d events, want none", len(published))
	}

	name := "renamed"
	command := "make test"
	position := 5
	updated, err := svc.UpdateRepositoryScript(ctxAs("user-b"), script.ID, &UpdateRepositoryScriptRequest{
		Name: &name, Command: &command, Position: &position,
	})
	if err != nil {
		t.Fatalf("owner UpdateRepositoryScript: %v", err)
	}
	if updated.Name != name || updated.Command != command || updated.Position != position {
		t.Fatalf("updated script = %+v", updated)
	}
	if types := eventTypes(bus.GetPublishedEvents()); len(types) != 1 || types[0] != events.RepositoryScriptUpdated {
		t.Fatalf("published %v, want exactly one %s", types, events.RepositoryScriptUpdated)
	}

	bus.ClearEvents()
	if err := svc.DeleteRepositoryScript(ctxAs("user-b"), script.ID); err != nil {
		t.Fatalf("owner DeleteRepositoryScript: %v", err)
	}
	if types := eventTypes(bus.GetPublishedEvents()); len(types) != 1 || types[0] != events.RepositoryScriptDeleted {
		t.Fatalf("published %v, want exactly one %s", types, events.RepositoryScriptDeleted)
	}
	remaining, err := repo.ListRepositoryScripts(ctx, "repo-b")
	if err != nil {
		t.Fatalf("ListRepositoryScripts: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("scripts after delete = %+v, want none", remaining)
	}
}

func TestListScriptsByRepositoryIDsGroupsByRepository(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-scripts", Name: "scripts"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	for _, id := range []string{"repo-1", "repo-2"} {
		if err := repo.CreateRepository(ctx, &models.Repository{ID: id, WorkspaceID: "ws-scripts", Name: id}); err != nil {
			t.Fatalf("create repository %s: %v", id, err)
		}
		if _, err := svc.CreateRepositoryScript(ctx, &CreateRepositoryScriptRequest{
			RepositoryID: id, Name: "setup-" + id, Command: "make",
		}); err != nil {
			t.Fatalf("create script for %s: %v", id, err)
		}
	}

	grouped, err := svc.ListScriptsByRepositoryIDs(ctx, []string{"repo-1", "repo-2"})
	if err != nil {
		t.Fatalf("ListScriptsByRepositoryIDs: %v", err)
	}
	if len(grouped["repo-1"]) != 1 || grouped["repo-1"][0].Name != "setup-repo-1" {
		t.Fatalf("repo-1 scripts = %+v", grouped["repo-1"])
	}
	if len(grouped["repo-2"]) != 1 || grouped["repo-2"][0].Name != "setup-repo-2" {
		t.Fatalf("repo-2 scripts = %+v", grouped["repo-2"])
	}
}

func TestCountActiveSessionsByRepositoryIsScoped(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedScopedWorkspaces(t, repo)
	ctx := context.Background()
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-b", WorkspaceID: "ws-b", Name: "b"}); err != nil {
		t.Fatalf("seed repository: %v", err)
	}

	if _, err := svc.CountActiveSessionsByRepository(ctxAs("user-a"), "repo-b"); !errors.Is(err, repoerrors.ErrRepositoryNotFound) {
		t.Fatalf("foreign count = %v, want ErrRepositoryNotFound", err)
	}
	count, err := svc.CountActiveSessionsByRepository(ctxAs("user-b"), "repo-b")
	if err != nil {
		t.Fatalf("owner count: %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0 with no attached sessions", count)
	}
	if _, err := svc.CountActiveSessionsByRepository(ctx, "no-such-repository"); err == nil {
		t.Fatal("counting for a missing repository must fail")
	}
}
