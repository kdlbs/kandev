package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	agentkubernetes "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

func TestCreateKubernetesExecutorRejectsMember(t *testing.T) {
	svc, _, _ := createTestService(t)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "member-1",
		Role:   authn.RoleMember,
	})

	_, err := svc.CreateExecutor(ctx, &CreateExecutorRequest{
		Name:   "Cluster",
		Type:   models.ExecutorTypeKubernetes,
		Status: models.ExecutorStatusActive,
		Config: validKubernetesExecutorConfig(),
	})
	if err == nil {
		t.Fatal("member created a Kubernetes executor, want authorization error")
	}
}

func TestCreateKubernetesExecutorValidatesTypedConfig(t *testing.T) {
	svc, _, _ := createTestService(t)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "admin-1",
		Role:   authn.RoleAdmin,
	})

	_, err := svc.CreateExecutor(ctx, &CreateExecutorRequest{
		Name:   "Cluster",
		Type:   models.ExecutorTypeKubernetes,
		Status: models.ExecutorStatusActive,
		Config: map[string]string{
			"auth_mode":               "kubeconfig",
			"kubeconfig_path":         "relative/config",
			"namespace":               "kandev",
			"request_timeout_seconds": "30",
		},
	})
	var fieldErr *agentkubernetes.FieldError
	if !errors.As(err, &fieldErr) || fieldErr.Path != "config.kubeconfig_path" {
		t.Fatalf("error = %v, want config.kubeconfig_path FieldError", err)
	}
}

func TestCreateKubernetesExecutorRejectsMissingIdentity(t *testing.T) {
	svc, _, _ := createTestService(t)

	_, err := svc.CreateExecutor(context.Background(), &CreateExecutorRequest{
		Name: "Cluster", Type: models.ExecutorTypeKubernetes,
		Status: models.ExecutorStatusActive, Config: validKubernetesExecutorConfig(),
	})
	if !errors.Is(err, ErrKubernetesAdminRequired) {
		t.Fatalf("error = %v, want ErrKubernetesAdminRequired", err)
	}
}

func TestKubernetesMutationsAllowSyntheticAdmin(t *testing.T) {
	svc, _, _ := createTestService(t)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "default-user", Role: authn.RoleAdmin, Synthetic: true,
	})

	executor, err := svc.CreateExecutor(ctx, &CreateExecutorRequest{
		Name: "Cluster", Type: models.ExecutorTypeKubernetes,
		Status: models.ExecutorStatusActive, Config: validKubernetesExecutorConfig(),
	})
	if err != nil || executor == nil {
		t.Fatalf("CreateExecutor() = %+v, %v; want success", executor, err)
	}
}

func TestUpdateExecutorRejectsMemberTransitionToKubernetes(t *testing.T) {
	svc, _, _ := createTestService(t)
	executor := createTestExecutor(t, svc, "Runner", models.ExecutorTypeLocal)
	kubernetesType := models.ExecutorTypeKubernetes
	ctx := authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "member-1",
		Role:   authn.RoleMember,
	})

	_, err := svc.UpdateExecutor(ctx, executor.ID, &UpdateExecutorRequest{
		Type:   &kubernetesType,
		Config: validKubernetesExecutorConfig(),
	})
	if !errors.Is(err, ErrKubernetesAdminRequired) {
		t.Fatalf("error = %v, want ErrKubernetesAdminRequired", err)
	}
}

func TestUpdateExecutorValidatesTransitionToKubernetes(t *testing.T) {
	svc, _, _ := createTestService(t)
	executor := createTestExecutor(t, svc, "Runner", models.ExecutorTypeLocal)
	kubernetesType := models.ExecutorTypeKubernetes
	ctx := authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "admin-1",
		Role:   authn.RoleAdmin,
	})

	_, err := svc.UpdateExecutor(ctx, executor.ID, &UpdateExecutorRequest{
		Type: &kubernetesType,
		Config: map[string]string{
			"auth_mode":               "kubeconfig",
			"kubeconfig_path":         "relative/config",
			"namespace":               "kandev",
			"request_timeout_seconds": "30",
		},
	})
	var fieldErr *agentkubernetes.FieldError
	if !errors.As(err, &fieldErr) || fieldErr.Path != "config.kubeconfig_path" {
		t.Fatalf("error = %v, want config.kubeconfig_path FieldError", err)
	}
}

func TestUpdateKubernetesExecutorRejectsTypeChangeWhileRunningInventoryIsRetained(t *testing.T) {
	svc, _, repo := createTestService(t)
	executor, err := svc.CreateExecutor(kubernetesAdminContext(), &CreateExecutorRequest{
		Name: "Cluster", Type: models.ExecutorTypeKubernetes,
		Status: models.ExecutorStatusActive, Config: validKubernetesExecutorConfig(),
	})
	if err != nil {
		t.Fatalf("CreateExecutor: %v", err)
	}
	if err := repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		SessionID: "session-retained", TaskID: "task-retained", ExecutorID: executor.ID,
		Runtime: agentruntime.RuntimeKubernetes,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}
	localType := models.ExecutorTypeLocal

	_, err = svc.UpdateExecutor(kubernetesAdminContext(), executor.ID, &UpdateExecutorRequest{
		Type: &localType,
	})

	if !errors.Is(err, ErrActiveTaskSessions) {
		t.Fatalf("UpdateExecutor() error = %v, want ErrActiveTaskSessions", err)
	}
	stored, getErr := repo.GetExecutor(context.Background(), executor.ID)
	if getErr != nil {
		t.Fatalf("GetExecutor: %v", getErr)
	}
	if stored.Type != models.ExecutorTypeKubernetes {
		t.Fatalf("blocked update changed executor type to %q", stored.Type)
	}
}

func TestDeleteKubernetesExecutorRejectsMember(t *testing.T) {
	svc, _, _ := createTestService(t)
	adminCtx := authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "admin-1",
		Role:   authn.RoleAdmin,
	})
	executor, err := svc.CreateExecutor(adminCtx, &CreateExecutorRequest{
		Name:   "Cluster",
		Type:   models.ExecutorTypeKubernetes,
		Status: models.ExecutorStatusActive,
		Config: validKubernetesExecutorConfig(),
	})
	if err != nil {
		t.Fatalf("CreateExecutor: %v", err)
	}
	memberCtx := authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "member-1",
		Role:   authn.RoleMember,
	})

	err = svc.DeleteExecutor(memberCtx, executor.ID)
	if !errors.Is(err, ErrKubernetesAdminRequired) {
		t.Fatalf("error = %v, want ErrKubernetesAdminRequired", err)
	}
}

func TestDeleteKubernetesExecutorRejectsAuthoritativeRunningInventoryAfterSessionRepoint(t *testing.T) {
	svc, _, repo := createTestService(t)
	executor, err := svc.CreateExecutor(kubernetesAdminContext(), &CreateExecutorRequest{
		Name: "Cluster", Type: models.ExecutorTypeKubernetes,
		Status: models.ExecutorStatusActive, Config: validKubernetesExecutorConfig(),
	})
	if err != nil {
		t.Fatalf("CreateExecutor: %v", err)
	}
	repointed := createTestExecutor(t, svc, "Local replacement", models.ExecutorTypeLocal)
	seedExecutorSession(t, repo, "session-repointed", repointed.ID)
	if err := repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		SessionID:  "session-repointed",
		TaskID:     "task-session-repointed",
		ExecutorID: executor.ID,
		Runtime:    agentruntime.RuntimeKubernetes,
		Metadata: map[string]interface{}{
			"kubernetes_resource_executor_id": executor.ID,
		},
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}

	err = svc.DeleteExecutor(kubernetesAdminContext(), executor.ID)

	if !errors.Is(err, ErrActiveTaskSessions) {
		t.Fatalf("DeleteExecutor() error = %v, want ErrActiveTaskSessions", err)
	}
	if _, getErr := repo.GetExecutor(context.Background(), executor.ID); getErr != nil {
		t.Fatalf("blocked delete removed executor needed for Kubernetes cleanup: %v", getErr)
	}
}

func TestDeleteKubernetesExecutorFailsClosedForAmbiguousRunningInventory(t *testing.T) {
	tests := []struct {
		name       string
		executorID string
		metadata   map[string]interface{}
	}{
		{name: "both associations missing"},
		{
			name: "current association missing", metadata: map[string]interface{}{
				"kubernetes_resource_executor_id": "other-executor",
			},
		},
		{name: "recorded association missing", executorID: "other-executor"},
		{
			name: "associations conflict", executorID: "current-executor",
			metadata: map[string]interface{}{
				"kubernetes_resource_executor_id": "recorded-executor",
			},
		},
		{
			name: "recorded association has invalid type", executorID: "other-executor",
			metadata: map[string]interface{}{
				"kubernetes_resource_executor_id": 42,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			executor, err := svc.CreateExecutor(kubernetesAdminContext(), &CreateExecutorRequest{
				Name: "Cluster", Type: models.ExecutorTypeKubernetes,
				Status: models.ExecutorStatusActive, Config: validKubernetesExecutorConfig(),
			})
			if err != nil {
				t.Fatalf("CreateExecutor: %v", err)
			}
			if err := repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
				SessionID: "session-ambiguous", TaskID: "task-ambiguous",
				ExecutorID: test.executorID, Runtime: agentruntime.RuntimeKubernetes,
				Metadata: test.metadata,
			}); err != nil {
				t.Fatalf("UpsertExecutorRunning: %v", err)
			}

			err = svc.DeleteExecutor(kubernetesAdminContext(), executor.ID)

			if !errors.Is(err, ErrActiveTaskSessions) {
				t.Fatalf("DeleteExecutor() error = %v, want ErrActiveTaskSessions", err)
			}
			if _, getErr := repo.GetExecutor(context.Background(), executor.ID); getErr != nil {
				t.Fatalf("blocked delete removed executor: %v", getErr)
			}
		})
	}
}

func TestDeleteKubernetesExecutorIgnoresUnrelatedNonKubernetesInventory(t *testing.T) {
	svc, _, repo := createTestService(t)
	executor, err := svc.CreateExecutor(kubernetesAdminContext(), &CreateExecutorRequest{
		Name: "Cluster", Type: models.ExecutorTypeKubernetes,
		Status: models.ExecutorStatusActive, Config: validKubernetesExecutorConfig(),
	})
	if err != nil {
		t.Fatalf("CreateExecutor: %v", err)
	}
	if err := repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		SessionID: "session-docker", TaskID: "task-docker", Runtime: agentruntime.RuntimeDocker,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}

	if err := svc.DeleteExecutor(kubernetesAdminContext(), executor.ID); err != nil {
		t.Fatalf("DeleteExecutor() error = %v, want unrelated Docker row ignored", err)
	}
}

func TestUpdateKubernetesExecutorFailsClosedForAmbiguousRunningInventory(t *testing.T) {
	svc, _, repo := createTestService(t)
	executor, err := svc.CreateExecutor(kubernetesAdminContext(), &CreateExecutorRequest{
		Name: "Cluster", Type: models.ExecutorTypeKubernetes,
		Status: models.ExecutorStatusActive, Config: validKubernetesExecutorConfig(),
	})
	if err != nil {
		t.Fatalf("CreateExecutor: %v", err)
	}
	if err := repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		SessionID: "session-ambiguous-update", TaskID: "task-ambiguous-update",
		Runtime: agentruntime.RuntimeKubernetes,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}
	localType := models.ExecutorTypeLocal

	_, err = svc.UpdateExecutor(kubernetesAdminContext(), executor.ID, &UpdateExecutorRequest{
		Type: &localType,
	})

	if !errors.Is(err, ErrActiveTaskSessions) {
		t.Fatalf("UpdateExecutor() error = %v, want ErrActiveTaskSessions", err)
	}
	stored, getErr := repo.GetExecutor(context.Background(), executor.ID)
	if getErr != nil {
		t.Fatalf("GetExecutor: %v", getErr)
	}
	if stored.Type != models.ExecutorTypeKubernetes {
		t.Fatalf("blocked update changed executor type to %q", stored.Type)
	}
}

func TestDeleteKubernetesExecutorIgnoresUnambiguousOtherKubernetesInventory(t *testing.T) {
	svc, _, repo := createTestService(t)
	executor, err := svc.CreateExecutor(kubernetesAdminContext(), &CreateExecutorRequest{
		Name: "Cluster", Type: models.ExecutorTypeKubernetes,
		Status: models.ExecutorStatusActive, Config: validKubernetesExecutorConfig(),
	})
	if err != nil {
		t.Fatalf("CreateExecutor: %v", err)
	}
	if err := repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		SessionID: "session-other-k8s", TaskID: "task-other-k8s",
		ExecutorID: "other-executor", Runtime: agentruntime.RuntimeKubernetes,
		Metadata: map[string]interface{}{
			"kubernetes_resource_executor_id": "other-executor",
		},
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}

	if err := svc.DeleteExecutor(kubernetesAdminContext(), executor.ID); err != nil {
		t.Fatalf("DeleteExecutor() error = %v, want fully-associated other row ignored", err)
	}
}

func TestDeleteKubernetesExecutorFailsClosedOnInventoryReadError(t *testing.T) {
	svc, _, repo := createTestService(t)
	executor, err := svc.CreateExecutor(kubernetesAdminContext(), &CreateExecutorRequest{
		Name: "Cluster", Type: models.ExecutorTypeKubernetes,
		Status: models.ExecutorStatusActive, Config: validKubernetesExecutorConfig(),
	})
	if err != nil {
		t.Fatalf("CreateExecutor: %v", err)
	}
	inventoryErr := errors.New("inventory unavailable")
	svc.executors = runningInventoryErrorRepository{
		ExecutorRepository: repo,
		err:                inventoryErr,
	}

	err = svc.DeleteExecutor(kubernetesAdminContext(), executor.ID)

	if !errors.Is(err, inventoryErr) {
		t.Fatalf("DeleteExecutor() error = %v, want inventory error", err)
	}
	if _, getErr := repo.GetExecutor(context.Background(), executor.ID); getErr != nil {
		t.Fatalf("inventory error removed executor: %v", getErr)
	}
}

type runningInventoryErrorRepository struct {
	repository.ExecutorRepository
	err error
}

func (r runningInventoryErrorRepository) ListExecutorsRunning(context.Context) ([]*models.ExecutorRunning, error) {
	return nil, r.err
}

func TestCreateKubernetesProfileRejectsMember(t *testing.T) {
	svc, _, _ := createTestService(t)
	executor, err := svc.CreateExecutor(kubernetesAdminContext(), &CreateExecutorRequest{
		Name:   "Cluster",
		Type:   models.ExecutorTypeKubernetes,
		Status: models.ExecutorStatusActive,
		Config: validKubernetesExecutorConfig(),
	})
	if err != nil {
		t.Fatalf("CreateExecutor: %v", err)
	}

	_, err = svc.CreateExecutorProfile(kubernetesMemberContext(), &CreateExecutorProfileRequest{
		ExecutorID: executor.ID,
		Name:       "Default",
		Config:     validKubernetesProfileConfig(),
	})
	if !errors.Is(err, ErrKubernetesAdminRequired) {
		t.Fatalf("error = %v, want ErrKubernetesAdminRequired", err)
	}
}

func TestCreateKubernetesProfileAuthorizesBeforeSecretLookup(t *testing.T) {
	svc, _, _ := createTestService(t)
	executor, err := svc.CreateExecutor(kubernetesAdminContext(), &CreateExecutorRequest{
		Name: "Cluster", Type: models.ExecutorTypeKubernetes,
		Status: models.ExecutorStatusActive, Config: validKubernetesExecutorConfig(),
	})
	if err != nil {
		t.Fatalf("CreateExecutor: %v", err)
	}
	secretStore := &recordingKubernetesSecretStore{}
	svc.SetSecretStore(secretStore)

	_, err = svc.CreateExecutorProfile(kubernetesMemberContext(), &CreateExecutorProfileRequest{
		ExecutorID: executor.ID, Name: "Default", Config: validKubernetesProfileConfig(),
		EnvVars: []models.ProfileEnvVar{{Key: "TOKEN", SecretID: "secret-1"}},
	})
	if !errors.Is(err, ErrKubernetesAdminRequired) {
		t.Fatalf("error = %v, want ErrKubernetesAdminRequired", err)
	}
	if secretStore.getCalls != 0 {
		t.Fatalf("secret Get calls = %d, want 0 before authorization", secretStore.getCalls)
	}
}

func TestCreateKubernetesProfileValidatesTypedConfig(t *testing.T) {
	svc, _, _ := createTestService(t)
	executor, err := svc.CreateExecutor(kubernetesAdminContext(), &CreateExecutorRequest{
		Name:   "Cluster",
		Type:   models.ExecutorTypeKubernetes,
		Status: models.ExecutorStatusActive,
		Config: validKubernetesExecutorConfig(),
	})
	if err != nil {
		t.Fatalf("CreateExecutor: %v", err)
	}
	config := validKubernetesProfileConfig()
	config["platform"] = "windows/amd64"

	_, err = svc.CreateExecutorProfile(kubernetesAdminContext(), &CreateExecutorProfileRequest{
		ExecutorID: executor.ID,
		Name:       "Default",
		Config:     config,
	})
	var fieldErr *agentkubernetes.FieldError
	if !errors.As(err, &fieldErr) || fieldErr.Path != "config.platform" {
		t.Fatalf("error = %v, want config.platform FieldError", err)
	}
}

func TestUpdateKubernetesProfileRejectsMember(t *testing.T) {
	svc, _, _ := createTestService(t)
	profile := createKubernetesProfile(t, svc)
	name := "Renamed"

	_, err := svc.UpdateExecutorProfile(kubernetesMemberContext(), profile.ID, &UpdateExecutorProfileRequest{
		Name: &name,
	})
	if !errors.Is(err, ErrKubernetesAdminRequired) {
		t.Fatalf("error = %v, want ErrKubernetesAdminRequired", err)
	}
}

func TestUpdateKubernetesProfileValidatesTypedConfig(t *testing.T) {
	svc, _, _ := createTestService(t)
	profile := createKubernetesProfile(t, svc)
	config := validKubernetesProfileConfig()
	config["workspace.mode"] = "host_path"

	_, err := svc.UpdateExecutorProfile(kubernetesAdminContext(), profile.ID, &UpdateExecutorProfileRequest{
		Config: config,
	})
	var fieldErr *agentkubernetes.FieldError
	if !errors.As(err, &fieldErr) || fieldErr.Path != "config.workspace.mode" {
		t.Fatalf("error = %v, want config.workspace.mode FieldError", err)
	}
}

func TestDeleteKubernetesProfileRejectsMember(t *testing.T) {
	svc, _, _ := createTestService(t)
	profile := createKubernetesProfile(t, svc)

	err := svc.DeleteExecutorProfile(kubernetesMemberContext(), profile.ID)
	if !errors.Is(err, ErrKubernetesAdminRequired) {
		t.Fatalf("error = %v, want ErrKubernetesAdminRequired", err)
	}
}

func TestDeleteExecutorProfileSucceedsAfterExecutorWasSoftDeleted(t *testing.T) {
	svc, _, _ := createTestService(t)
	profile := createKubernetesProfile(t, svc)

	require.NoError(t, svc.DeleteExecutor(kubernetesAdminContext(), profile.ExecutorID))
	require.NoError(t, svc.DeleteExecutorProfile(kubernetesAdminContext(), profile.ID))
	_, err := svc.GetExecutorProfile(context.Background(), profile.ID)
	require.Error(t, err)
}

func TestKubernetesExecutorAndProfileReadsRemainAvailableToMembers(t *testing.T) {
	svc, _, _ := createTestService(t)
	profile := createKubernetesProfile(t, svc)

	executor, err := svc.GetExecutor(kubernetesMemberContext(), profile.ExecutorID)
	if err != nil || executor == nil {
		t.Fatalf("GetExecutor() = %+v, %v; want member-readable", executor, err)
	}
	profiles, err := svc.ListExecutorProfiles(kubernetesMemberContext(), profile.ExecutorID)
	if err != nil || len(profiles) != 1 {
		t.Fatalf("ListExecutorProfiles() = %+v, %v; want one member-readable profile", profiles, err)
	}
}

func validKubernetesExecutorConfig() map[string]string {
	return map[string]string{
		"auth_mode":               "kubeconfig",
		"kubeconfig_path":         "/etc/kandev/kubeconfig",
		"namespace":               "kandev",
		"request_timeout_seconds": "30",
	}
}

func validKubernetesProfileConfig() map[string]string {
	return map[string]string{
		"platform":       "linux/amd64",
		"main_container": "kandev-agent",
		"pod_template_yaml": `apiVersion: v1
kind: PodTemplate
template:
  spec:
    containers:
      - name: kandev-agent
        image: alpine:3.20
`,
		"workspace.mode": "empty_dir",
	}
}

func kubernetesAdminContext() context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: "admin-1", Role: authn.RoleAdmin})
}

func kubernetesMemberContext() context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: "member-1", Role: authn.RoleMember})
}

func createKubernetesProfile(t *testing.T, svc *Service) *models.ExecutorProfile {
	t.Helper()
	executor, err := svc.CreateExecutor(kubernetesAdminContext(), &CreateExecutorRequest{
		Name: "Cluster", Type: models.ExecutorTypeKubernetes,
		Status: models.ExecutorStatusActive, Config: validKubernetesExecutorConfig(),
	})
	if err != nil {
		t.Fatalf("CreateExecutor: %v", err)
	}
	profile, err := svc.CreateExecutorProfile(kubernetesAdminContext(), &CreateExecutorProfileRequest{
		ExecutorID: executor.ID, Name: "Default", Config: validKubernetesProfileConfig(),
	})
	if err != nil {
		t.Fatalf("CreateExecutorProfile: %v", err)
	}
	return profile
}

type recordingKubernetesSecretStore struct {
	getCalls int
}

func (*recordingKubernetesSecretStore) Create(context.Context, *secrets.SecretWithValue) error {
	return nil
}

func (s *recordingKubernetesSecretStore) Get(context.Context, string) (*secrets.Secret, error) {
	s.getCalls++
	return nil, errors.New("secret lookup should not run")
}

func (*recordingKubernetesSecretStore) Reveal(context.Context, string) (string, error) {
	return "", nil
}

func (*recordingKubernetesSecretStore) Update(context.Context, string, *secrets.UpdateSecretRequest) error {
	return nil
}

func (*recordingKubernetesSecretStore) Delete(context.Context, string) error { return nil }

func (*recordingKubernetesSecretStore) List(context.Context) ([]*secrets.SecretListItem, error) {
	return nil, nil
}

func (*recordingKubernetesSecretStore) Close() error { return nil }
