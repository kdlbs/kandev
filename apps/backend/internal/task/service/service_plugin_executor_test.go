package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

type pluginExecutorCatalogFake struct {
	providers      []models.ExecutorProvider
	fieldErrors    []models.ExecutorProviderFieldError
	validationErr  error
	lastExecutorID string
	lastProfile    models.ExecutorProviderProfile
}

func (f *pluginExecutorCatalogFake) ListExecutorProviders(context.Context) ([]models.ExecutorProvider, error) {
	return append([]models.ExecutorProvider(nil), f.providers...), nil
}

func (f *pluginExecutorCatalogFake) ValidateExecutorProviderProfile(_ context.Context, executorID string, profile models.ExecutorProviderProfile) ([]models.ExecutorProviderFieldError, error) {
	f.lastExecutorID = executorID
	f.lastProfile = profile
	return f.fieldErrors, f.validationErr
}

type pluginProfileSecretStore struct {
	items map[string]*secrets.SecretWithValue
}

func newPluginProfileSecretStore() *pluginProfileSecretStore {
	return &pluginProfileSecretStore{items: make(map[string]*secrets.SecretWithValue)}
}

func (s *pluginProfileSecretStore) Create(_ context.Context, secret *secrets.SecretWithValue) error {
	if secret.ID == "" {
		secret.ID = uuid.NewString()
	}
	copy := *secret
	s.items[secret.ID] = &copy
	return nil
}

func (s *pluginProfileSecretStore) Get(_ context.Context, id string) (*secrets.Secret, error) {
	secret := s.items[id]
	if secret == nil {
		return nil, errors.New("secret not found")
	}
	return &secret.Secret, nil
}

func (s *pluginProfileSecretStore) Reveal(_ context.Context, id string) (string, error) {
	secret := s.items[id]
	if secret == nil {
		return "", errors.New("secret not found")
	}
	return secret.Value, nil
}

func (s *pluginProfileSecretStore) Update(_ context.Context, id string, req *secrets.UpdateSecretRequest) error {
	secret := s.items[id]
	if secret == nil {
		return errors.New("secret not found")
	}
	if req.Name != nil {
		secret.Name = *req.Name
	}
	if req.Value != nil {
		secret.Value = *req.Value
	}
	return nil
}

func (s *pluginProfileSecretStore) Delete(_ context.Context, id string) error {
	delete(s.items, id)
	return nil
}

func (s *pluginProfileSecretStore) List(context.Context) ([]*secrets.SecretListItem, error) {
	items := make([]*secrets.SecretListItem, 0, len(s.items))
	for id, secret := range s.items {
		items = append(items, &secrets.SecretListItem{ID: id, Name: secret.Name, Scope: secret.Scope})
	}
	return items, nil
}

func (s *pluginProfileSecretStore) Close() error { return nil }

func pluginExecutorTestCatalog(available bool) *pluginExecutorCatalogFake {
	identity := "plugin:example-provider:lambda"
	return &pluginExecutorCatalogFake{providers: []models.ExecutorProvider{{
		ExecutorID: models.ExecutorProviderExecutorID(identity), Identity: identity,
		PluginID: "example-provider", InstallationID: "installation-1", Key: "lambda",
		ContractVersion: 1, DisplayName: "Lambda", Available: available,
		ProfileSchema: map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{
				"region":     map[string]any{"type": "string"},
				"credential": map[string]any{"type": "string", "secret": true},
			},
			"required": []any{"region"},
		},
		Capabilities: models.ExecutorProviderCapabilities{Terminal: true, Reattach: true, Retention: "bounded", MaximumLifetimeSecs: 3600},
	}}}
}

func TestPluginExecutorProfileSecrets(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	catalog := pluginExecutorTestCatalog(true)
	secretStore := newPluginProfileSecretStore()
	svc.SetExecutorProviderCatalog(catalog)
	svc.SetSecretStore(secretStore)

	executors, err := svc.ListExecutors(ctx)
	if err != nil {
		t.Fatalf("ListExecutors(): %v", err)
	}
	var remote *models.Executor
	for _, candidate := range executors {
		if candidate.Type == models.ExecutorTypePluginRemote {
			remote = candidate
			break
		}
	}
	if remote == nil {
		t.Fatal("provider catalog did not expose a remote executor")
	}

	profile, err := svc.CreateExecutorProfile(ctx, &CreateExecutorProfileRequest{
		ExecutorID: remote.ID, Name: "Build", Config: map[string]string{"region": "eu-west-1", "credential": "first-secret"},
	})
	if err != nil {
		t.Fatalf("CreateExecutorProfile(): %v", err)
	}
	firstID, ok := models.ExecutorProfileSecretID(profile.Config["credential"])
	if !ok || secretStore.items[firstID] == nil || secretStore.items[firstID].Value != "first-secret" {
		t.Fatalf("stored profile secret = %q, vault item = %#v", profile.Config["credential"], secretStore.items[firstID])
	}
	if catalog.lastProfile.SecretValues["credential"] != "first-secret" {
		t.Fatalf("provider validation secret values = %#v", catalog.lastProfile.SecretValues)
	}
	if _, leaked := catalog.lastProfile.Config["credential"]; leaked {
		t.Fatal("secret field was also sent through the public scalar config")
	}
	seedPluginProfileInventory(t, repo, profile.ID, firstID)
	if inUse, refs, err := svc.retainedPluginExecutorReferences(ctx, profile.ID, []string{firstID}); err != nil || !inUse || !refs[firstID] {
		t.Fatalf("retained inventory references = inUse:%v refs:%v err:%v", inUse, refs, err)
	}

	unchanged, err := svc.UpdateExecutorProfile(ctx, profile.ID, &UpdateExecutorProfileRequest{
		Config: map[string]string{"region": "us-west-2"},
	})
	if err != nil {
		t.Fatalf("UpdateExecutorProfile(unchanged secret): %v", err)
	}
	if unchanged.Config["credential"] != profile.Config["credential"] || secretStore.items[firstID] == nil {
		t.Fatal("omitting a secret field must preserve its vault binding")
	}

	replaced, err := svc.UpdateExecutorProfile(ctx, profile.ID, &UpdateExecutorProfileRequest{
		Config: map[string]string{"credential": "replacement-secret"},
	})
	if err != nil {
		t.Fatalf("UpdateExecutorProfile(replace secret): %v", err)
	}
	secondID, ok := models.ExecutorProfileSecretID(replaced.Config["credential"])
	if !ok || secondID == firstID || secretStore.items[firstID] == nil || secretStore.items[secondID].Value != "replacement-secret" {
		t.Fatalf("replacement secret binding = %q; first=%#v second=%#v", replaced.Config["credential"], secretStore.items[firstID], secretStore.items[secondID])
	}
	recovered, err := svc.ExecutorProviderProfileForRecovery(ctx, profile.ID, "task-plugin-profile-secrets", "environment-plugin-profile-secrets", catalog.providers[0].Identity, 7,
		map[string]string{"region": "eu-west-1"}, map[string]string{"credential": firstID})
	if err != nil || recovered.SecretValues["credential"] != "first-secret" {
		t.Fatalf("recovery after rotation = %#v, %v; want original credential", recovered, err)
	}

	_, err = svc.UpdateExecutorProfile(ctx, profile.ID, &UpdateExecutorProfileRequest{
		Config: map[string]string{"credential": ""},
	})
	if !errors.Is(err, ErrExecutorProfileInUse) {
		t.Fatalf("UpdateExecutorProfile(clear retained secret) = %v, want ErrExecutorProfileInUse", err)
	}
	if secretStore.items[secondID] == nil {
		t.Fatal("failed clear removed the profile's current secret")
	}
	if err := svc.DeleteExecutorProfile(ctx, profile.ID); !errors.Is(err, ErrExecutorProfileInUse) {
		t.Fatalf("DeleteExecutorProfile(retained environment) = %v, want ErrExecutorProfileInUse", err)
	}

	markPluginProfileInventoryAbsent(t, repo)
	cleared, err := svc.UpdateExecutorProfile(ctx, profile.ID, &UpdateExecutorProfileRequest{Config: map[string]string{"credential": ""}})
	if err != nil {
		t.Fatalf("UpdateExecutorProfile(clear secret after cleanup): %v", err)
	}
	if _, configured := cleared.Config["credential"]; configured {
		t.Fatal("empty secret input must clear its vault binding")
	}
	if secretStore.items[firstID] == nil || secretStore.items[secondID] == nil {
		t.Fatal("profile edits must retain historical credentials until profile deletion")
	}
	if err := svc.DeleteExecutorProfile(ctx, profile.ID); err != nil {
		t.Fatalf("DeleteExecutorProfile(after cleanup): %v", err)
	}
	if secretStore.items[firstID] != nil || secretStore.items[secondID] != nil {
		t.Fatal("profile deletion must remove historical provider credentials")
	}

	_, err = svc.CreateExecutorProfile(ctx, &CreateExecutorProfileRequest{
		ExecutorID: remote.ID, Name: "Cross-provider", Config: map[string]string{
			"region": "eu-west-1", "credential": models.ExecutorProfileSecretReference(firstID),
		},
	})
	if !errors.Is(err, ErrInvalidExecutorConfig) {
		t.Fatalf("cross-profile secret reference error = %v, want invalid config", err)
	}
}

func seedPluginProfileInventory(t *testing.T, repo interface {
	CreateTask(context.Context, *models.Task) error
	CreateTaskEnvironment(context.Context, *models.TaskEnvironment) error
	CreateTaskSession(context.Context, *models.TaskSession) error
	UpsertExecutorRunning(context.Context, *models.ExecutorRunning) error
}, profileID, secretID string) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-plugin-profile-secrets", Title: "Retained plugin environment"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "environment-plugin-profile-secrets", TaskID: "task-plugin-profile-secrets",
		ExecutorType: string(models.ExecutorTypePluginRemote), OwnershipGeneration: 7,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-plugin-profile-secrets", TaskID: "task-plugin-profile-secrets",
		TaskEnvironmentID: "environment-plugin-profile-secrets", State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session-plugin-profile-secrets", SessionID: "session-plugin-profile-secrets",
		TaskID: "task-plugin-profile-secrets", ExecutorID: "plugin-provider",
		Runtime: agentruntime.RuntimePluginRemote, Status: models.ExecutorRunningStatusStopped,
		AgentExecutionID: "execution-plugin-profile-secrets",
		Metadata: map[string]interface{}{"plugin_executor": map[string]interface{}{
			"profile_id": profileID, "phase": "ready", "environment_id": "environment-plugin-profile-secrets",
			"environment_generation": int64(7), "secret_references": map[string]string{"credential": secretID},
		}},
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}
}

func markPluginProfileInventoryAbsent(t *testing.T, repo interface {
	GetExecutorRunningBySessionID(context.Context, string) (*models.ExecutorRunning, error)
	UpsertExecutorRunning(context.Context, *models.ExecutorRunning) error
}) {
	t.Helper()
	ctx := context.Background()
	running, err := repo.GetExecutorRunningBySessionID(ctx, "session-plugin-profile-secrets")
	if err != nil {
		t.Fatalf("GetExecutorRunningBySessionID: %v", err)
	}
	envelope, ok := running.Metadata["plugin_executor"].(map[string]interface{})
	if !ok {
		t.Fatalf("plugin executor metadata = %#v", running.Metadata["plugin_executor"])
	}
	envelope["phase"] = "absent"
	if err := repo.UpsertExecutorRunning(ctx, running); err != nil {
		t.Fatalf("UpsertExecutorRunning(absent): %v", err)
	}
}

func TestPluginExecutorAdmissionPaths(t *testing.T) {
	svc, _, _ := createTestService(t)
	ctx := context.Background()
	catalog := pluginExecutorTestCatalog(true)
	svc.SetExecutorProviderCatalog(catalog)
	executors, err := svc.ListExecutors(ctx)
	if err != nil {
		t.Fatalf("ListExecutors(): %v", err)
	}
	var remote *models.Executor
	for _, candidate := range executors {
		if candidate.Type == models.ExecutorTypePluginRemote {
			remote = candidate
			break
		}
	}
	if remote == nil {
		t.Fatal("provider executor is missing")
	}
	profile, err := svc.CreateExecutorProfile(ctx, &CreateExecutorProfileRequest{ExecutorID: remote.ID, Name: "Launch", Config: map[string]string{"region": "eu-west-1"}})
	if err != nil {
		t.Fatalf("CreateExecutorProfile(): %v", err)
	}
	if err := svc.ValidateExecutorProfileAdmission(ctx, profile.ID); err != nil {
		t.Fatalf("available profile admission: %v", err)
	}
	compatible := &CreateTaskRequest{StartAgent: true, ExecutorProfileID: profile.ID}
	if err := svc.validateRequestedExecutorAdmission(ctx, compatible); err != nil {
		t.Fatalf("isolated provider launch admission: %v", err)
	}
	for _, tc := range []struct {
		name string
		edit func(*CreateTaskRequest)
	}{
		{name: "shared workspace", edit: func(req *CreateTaskRequest) { req.WorkspacePolicy = &WorkspacePolicy{Mode: workspaceModeSharedGroup} }},
		{name: "inherited workspace", edit: func(req *CreateTaskRequest) { req.WorkspacePolicy = &WorkspacePolicy{Mode: workspaceModeInheritParent} }},
		{name: "host workspace", edit: func(req *CreateTaskRequest) { req.WorkspacePath = "/work/project" }},
		{name: "local repository", edit: func(req *CreateTaskRequest) { req.Repositories = []TaskRepositoryInput{{LocalPath: "/work/project"}} }},
		{name: "file repository URL", edit: func(req *CreateTaskRequest) {
			req.Repositories = []TaskRepositoryInput{{RemoteURL: "file:///work/project"}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &CreateTaskRequest{StartAgent: true, ExecutorProfileID: profile.ID}
			tc.edit(req)
			if err := svc.validateRequestedExecutorAdmission(ctx, req); !errors.Is(err, ErrInvalidExecutorConfig) {
				t.Fatalf("launch admission error = %v, want invalid provider configuration", err)
			}
		})
	}

	catalog.providers[0].Available = false
	if err := svc.ValidateExecutorProfileAdmission(ctx, profile.ID); !errors.Is(err, ErrExecutorProviderUnavailable) {
		t.Fatalf("unavailable profile admission error = %v, want unavailable", err)
	}
}

func TestPluginExecutorRepositoryURLCompatibility(t *testing.T) {
	for _, tc := range []struct {
		url   string
		local bool
	}{
		{url: "https://github.com/acme/project.git"},
		{url: "ssh://git@github.com/acme/project.git"},
		{url: "git@github.com:acme/project.git"},
		{url: "file:///work/project", local: true},
		{url: "/work/project", local: true},
	} {
		if got := isLocalRepositoryURL(tc.url); got != tc.local {
			t.Errorf("isLocalRepositoryURL(%q) = %v, want %v", tc.url, got, tc.local)
		}
	}
}

func TestPluginExecutorNoLocalFallback(t *testing.T) {
	if got := executor.ExecutorTypeToBackend(models.ExecutorTypePluginRemote); got != executor.NamePluginRemote {
		t.Fatalf("plugin remote executor runtime = %q, want %q", got, executor.NamePluginRemote)
	}
	if got := executor.ExecutorTypeToBackend(models.ExecutorType("missing-provider")); got != executor.NameUnknown {
		t.Fatalf("unknown executor runtime = %q, want unknown", got)
	}
	if got := models.ExecutorType("missing-provider").Runtime(); got != "" {
		t.Fatalf("unknown executor model runtime = %q, want empty fail-closed value", got)
	}
}
