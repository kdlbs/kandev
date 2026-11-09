package service

import (
	"context"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/remoteauth"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

func TestWorkflowCredentialAdmission(t *testing.T) {
	fixture := newWorkflowChangeFixture(t)
	ctx := context.Background()

	executor, err := fixture.repo.GetExecutor(ctx, "executor-task")
	require.NoError(t, err)
	executor.Type = models.ExecutorTypeSSH
	executor.OwnerUserID = "alice"
	require.NoError(t, fixture.repo.UpdateExecutor(ctx, executor))
	require.NoError(t, fixture.repo.CreateExecutorProfile(ctx, &models.ExecutorProfile{
		ID: "ssh-profile", ExecutorID: "executor-task", OwnerUserID: "alice", Name: "SSH",
	}))

	task, err := fixture.repo.GetTask(ctx, fixture.task.ID)
	require.NoError(t, err)
	if task.Metadata == nil {
		task.Metadata = map[string]interface{}{}
	}
	task.Metadata[models.MetaKeyExecutorProfileID] = "ssh-profile"
	require.NoError(t, fixture.repo.UpdateTask(ctx, task))
	task, err = fixture.repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	fixture.request.ExpectedUpdatedAt = task.UpdatedAt

	_, err = fixture.svc.ValidateWorkflowChange(
		ctx, task.ID, "workflow-target", "target-analysis", fixture.request,
	)
	require.ErrorIs(t, err, ErrInvalidWorkflowChange)
	require.Empty(t, fixture.validator.profiles, "validator must not run without an established actor")

	bob := authn.WithIdentity(ctx, authn.Identity{UserID: "bob", Role: authn.RoleMember})
	_, err = fixture.svc.ValidateWorkflowChange(
		bob, task.ID, "workflow-target", "target-analysis", fixture.request,
	)
	require.ErrorIs(t, err, ErrInvalidWorkflowChange)
	require.Empty(t, fixture.validator.profiles, "validator must not run for a foreign executor owner")

	alice := authn.WithIdentity(ctx, authn.Identity{UserID: "alice", Role: authn.RoleMember})
	_, err = fixture.svc.ValidateWorkflowChange(
		alice, task.ID, "workflow-target", "target-analysis", fixture.request,
	)
	require.NoError(t, err)
	require.NotEmpty(t, fixture.validator.profiles)
}

type scopedClaudeProfileValidator struct {
	agent agents.Agent
	store secrets.SecretStore
}

func (v scopedClaudeProfileValidator) ValidateAgentProfileForExecutor(
	ctx context.Context,
	profile *settingsmodels.AgentProfile,
	executor *models.Executor,
	executorProfile *models.ExecutorProfile,
) error {
	if executor == nil || !models.IsRemoteExecutorType(executor.Type) {
		return nil
	}
	if executorProfile == nil {
		return errors.New("remote executor profile is required")
	}
	return remoteauth.ValidateCredentialAdmission(ctx, remoteauth.AdmissionInput{
		AgentID:         v.agent.ID(),
		ProfileEnvVars:  profile.EnvVars,
		ExecutorEnvVars: executorProfile.EnvVars,
		ExecutorConfig:  executorProfile.Config,
		Catalog:         remoteauth.BuildCatalog([]agents.Agent{v.agent}),
		SecretStore:     v.store,
	})
}

type claudeWorkflowAdmissionFixture struct {
	workflowChangeFixture
	store  secrets.SecretStore
	secret *secrets.SecretWithValue
	actor  context.Context
}

func newClaudeWorkflowAdmissionFixture(t *testing.T) claudeWorkflowAdmissionFixture {
	t.Helper()
	base := newWorkflowChangeFixture(t)
	actor := authn.WithIdentity(context.Background(), authn.Identity{UserID: "alice", Role: authn.RoleMember})

	sqlxDB := sqlx.NewDb(base.repo.DB(), "sqlite3")
	crypto, err := secrets.NewMasterKeyProvider(t.TempDir())
	require.NoError(t, err)
	rawStore, closeStore, err := secrets.Provide(sqlxDB, sqlxDB, crypto)
	require.NoError(t, err)
	t.Cleanup(func() { _ = closeStore() })
	store := secrets.NewUserVisibleStore(rawStore)
	secret := &secrets.SecretWithValue{
		Secret: secrets.Secret{Name: "Claude workflow integration key"},
		Value:  "sk-ant-workflow-integration",
	}
	require.NoError(t, store.Create(actor, secret))

	executor, err := base.repo.GetExecutor(actor, "executor-task")
	require.NoError(t, err)
	executor.Type = models.ExecutorTypeSSH
	executor.OwnerUserID = "alice"
	require.NoError(t, base.repo.UpdateExecutor(actor, executor))
	require.NoError(t, base.repo.CreateExecutorProfile(actor, &models.ExecutorProfile{
		ID: "claude-ssh-profile", ExecutorID: "executor-task", OwnerUserID: "alice", Name: "Claude SSH",
	}))
	session, err := base.repo.GetTaskSession(actor, "session-workflow-change")
	require.NoError(t, err)
	session.ExecutorProfileID = "claude-ssh-profile"
	require.NoError(t, base.repo.UpdateTaskSession(actor, session))

	profiles := make(map[string]*settingsmodels.AgentProfile, 2)
	for _, id := range []string{"source-profile", "replacement-profile"} {
		profiles[id] = &settingsmodels.AgentProfile{
			ID: id, AgentID: "claude-agent", Name: id, WorkspaceID: "workspace-change", Enabled: true,
			EnvVars: []settingsmodels.ProfileEnvVar{{Key: "ANTHROPIC_API_KEY", SecretID: secret.ID}},
		}
	}
	base.svc.agentProfiles = workflowAgentOverrideProfilesStub{profiles: profiles}
	base.svc.agentProfileExecutorValidator = scopedClaudeProfileValidator{
		agent: agents.NewClaudeACP(), store: store,
	}
	base.svc.SetSecretStore(store)
	return claudeWorkflowAdmissionFixture{
		workflowChangeFixture: base, store: store, secret: secret, actor: actor,
	}
}

func TestClaudeWorkflowAdmissionDefaultAndReplacement(t *testing.T) {
	t.Run("destination default", func(t *testing.T) {
		fixture := newClaudeWorkflowAdmissionFixture(t)
		request := *fixture.request
		request.AgentOverrides = map[string]string{}

		preview, err := fixture.svc.ValidateWorkflowChange(
			fixture.actor, fixture.task.ID, "workflow-target", "target-analysis", &request,
		)
		require.NoError(t, err)
		require.True(t, preview == nil || len(preview.Steps) == 0, "defaults must not add bindings")

		fixture.bus.ClearEvents()
		result, err := fixture.svc.MoveTaskWithOptions(
			fixture.actor, fixture.task.ID, "workflow-target", "target-analysis", 0,
			MoveTaskOptions{AllowActivePrimarySession: true, WorkflowChange: &request},
		)
		require.NoError(t, err)
		require.Equal(t, "workflow-target", result.Task.WorkflowID)
		require.Equal(t, "target-analysis", result.Task.WorkflowStepID)
		require.True(t, result.Task.WorkflowAgentOverrides == nil || len(result.Task.WorkflowAgentOverrides.Steps) == 0)
		requireTaskMovedEvent(t, fixture.bus)
	})

	t.Run("explicit replacement", func(t *testing.T) {
		fixture := newClaudeWorkflowAdmissionFixture(t)
		request := *fixture.request
		request.AgentOverrides = map[string]string{"source-profile": "replacement-profile"}

		preview, err := fixture.svc.ValidateWorkflowChange(
			fixture.actor, fixture.task.ID, "workflow-target", "target-analysis", &request,
		)
		require.NoError(t, err)
		require.Len(t, preview.Steps, 2)
		for _, binding := range preview.Steps {
			require.Equal(t, "source-profile", binding.SourceProfileID)
			require.Equal(t, "replacement-profile", binding.ReplacementProfileID)
		}

		fixture.bus.ClearEvents()
		result, err := fixture.svc.MoveTaskWithOptions(
			fixture.actor, fixture.task.ID, "workflow-target", "target-analysis", 0,
			MoveTaskOptions{AllowActivePrimarySession: true, WorkflowChange: &request},
		)
		require.NoError(t, err)
		require.Equal(t, "workflow-target", result.Task.WorkflowID)
		require.Equal(t, "target-analysis", result.Task.WorkflowStepID)
		require.Len(t, result.Task.WorkflowAgentOverrides.Steps, 2)
		for _, binding := range result.Task.WorkflowAgentOverrides.Steps {
			require.Equal(t, "replacement-profile", binding.ReplacementProfileID)
		}
		requireTaskMovedEvent(t, fixture.bus)
	})
}

func TestClaudeWorkflowAdmissionDeniesDeletedBindingBeforeApply(t *testing.T) {
	fixture := newClaudeWorkflowAdmissionFixture(t)
	request := *fixture.request
	request.AgentOverrides = map[string]string{"source-profile": "replacement-profile"}
	_, err := fixture.svc.ValidateWorkflowChange(
		fixture.actor, fixture.task.ID, "workflow-target", "target-analysis", &request,
	)
	require.NoError(t, err)

	require.NoError(t, fixture.store.Delete(fixture.actor, fixture.secret.ID))
	fixture.bus.ClearEvents()
	_, err = fixture.svc.MoveTaskWithOptions(
		fixture.actor, fixture.task.ID, "workflow-target", "target-analysis", 0,
		MoveTaskOptions{AllowActivePrimarySession: true, WorkflowChange: &request},
	)
	require.ErrorIs(t, err, ErrInvalidWorkflowChange)
	requireNoTaskMovedEvent(t, fixture.bus)

	stored, err := fixture.repo.GetTask(fixture.actor, fixture.task.ID)
	require.NoError(t, err)
	require.Equal(t, "workflow-source", stored.WorkflowID)
	require.Equal(t, "source-step", stored.WorkflowStepID)
	require.Equal(t, fixture.task.WorkflowAgentOverrides, stored.WorkflowAgentOverrides)
}

func TestClaudeWorkflowAdmissionDeniesForeignActorBeforeApply(t *testing.T) {
	fixture := newClaudeWorkflowAdmissionFixture(t)
	request := *fixture.request
	request.AgentOverrides = map[string]string{"source-profile": "replacement-profile"}
	_, err := fixture.svc.ValidateWorkflowChange(
		fixture.actor, fixture.task.ID, "workflow-target", "target-analysis", &request,
	)
	require.NoError(t, err)

	bob := authn.WithIdentity(context.Background(), authn.Identity{UserID: "bob", Role: authn.RoleMember})
	fixture.bus.ClearEvents()
	_, err = fixture.svc.MoveTaskWithOptions(
		bob, fixture.task.ID, "workflow-target", "target-analysis", 0,
		MoveTaskOptions{AllowActivePrimarySession: true, WorkflowChange: &request},
	)
	require.ErrorIs(t, err, ErrInvalidWorkflowChange)
	requireNoTaskMovedEvent(t, fixture.bus)

	stored, err := fixture.repo.GetTask(fixture.actor, fixture.task.ID)
	require.NoError(t, err)
	require.Equal(t, "workflow-source", stored.WorkflowID)
	require.Equal(t, "source-step", stored.WorkflowStepID)
	require.Equal(t, fixture.task.WorkflowAgentOverrides, stored.WorkflowAgentOverrides)
}

func requireTaskMovedEvent(t *testing.T, bus *MockEventBus) {
	t.Helper()
	for _, event := range bus.GetPublishedEvents() {
		if event != nil && event.Type == events.TaskMoved {
			return
		}
	}
	t.Fatal("expected task.moved event")
}

func requireNoTaskMovedEvent(t *testing.T, bus *MockEventBus) {
	t.Helper()
	for _, event := range bus.GetPublishedEvents() {
		if event != nil && event.Type == events.TaskMoved {
			t.Fatalf("unexpected task.moved event after failed apply: %#v", event)
		}
	}
}
