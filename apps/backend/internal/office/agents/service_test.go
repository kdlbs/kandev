package agents

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"
)

type testActivityLogger struct{}

type failingProfileStore struct{}

func (f failingProfileStore) GetAgentProfile(context.Context, string) (*models.AgentInstance, error) {
	return nil, errors.New("profile unavailable")
}

func (f failingProfileStore) UpdateAgentProfile(context.Context, *models.AgentInstance) error {
	return errors.New("profile update failed")
}

func (t *testActivityLogger) LogActivity(_ context.Context, _, _, _, _, _, _, _ string) {}
func (t *testActivityLogger) LogActivityWithRun(_ context.Context, _, _, _, _, _, _, _, _, _ string) {
}

type fakeGovernanceSettings struct {
	requireNewAgents bool
}

func (f *fakeGovernanceSettings) GetRequireApprovalForNewAgents(
	_ context.Context,
	_ string,
) (bool, error) {
	return f.requireNewAgents, nil
}

type fakeApprovalCreator struct {
	approvals []*models.Approval
}

func (f *fakeApprovalCreator) CreateApprovalWithActivity(
	_ context.Context,
	approval *models.Approval,
) error {
	f.approvals = append(f.approvals, approval)
	return nil
}

func newTestAgentService(t *testing.T) (*AgentService, *sqlite.Repository) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}

	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	svc := NewAgentService(repo, logger.Default(), &testActivityLogger{})
	return svc, repo
}

func newTestAgentServiceWithProfileStore(
	t *testing.T,
) (*AgentService, *sqlite.Repository, settingsstore.Repository) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	profileStore, _, err := settingsstore.Provide(db, db, nil)
	if err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	svc := NewAgentService(repo, logger.Default(), &testActivityLogger{})
	svc.SetProfileStore(profileStore)
	return svc, repo, profileStore
}

func TestCreateAgentInstance_RollsBackWhenCanonicalProfileUpdateFails(t *testing.T) {
	svc, _ := newTestAgentService(t)
	svc.SetProfileStore(failingProfileStore{})
	agent := &models.AgentInstance{
		WorkspaceID: "ws-1", Name: "CTO", Role: models.AgentRoleSpecialist,
	}

	if err := svc.CreateAgentInstance(context.Background(), agent); err == nil {
		t.Fatal("expected canonical profile update failure")
	}
	if _, err := svc.GetAgentFromConfig(context.Background(), agent.Name); err == nil {
		t.Fatal("agent row survived failed canonical profile update")
	}
}

func TestUpdateAgent_ProfileSelectionDirectsClientsToRouting(t *testing.T) {
	svc, _, profileStore := newTestAgentServiceWithProfileStore(t)
	ctx := context.Background()
	provider := &settingsmodels.Agent{ID: "provider-db-id", Name: "claude-acp"}
	if err := profileStore.CreateAgent(ctx, provider); err != nil {
		t.Fatalf("create provider: %v", err)
	}
	source := &settingsmodels.AgentProfile{
		ID:               "source-profile",
		AgentID:          provider.ID,
		Name:             "Work",
		AgentDisplayName: "Claude",
		Model:            "opus",
		Mode:             "bypassPermissions",
		ConfigOptions:    map[string]string{"effort": "high"},
		AutoApprove:      true,
		CLIFlags: []settingsmodels.CLIFlag{
			{Flag: "--dangerously-skip-permissions", Enabled: true},
		},
		EnvVars: []settingsmodels.ProfileEnvVar{
			{Key: "CLAUDE_CONFIG_DIR", Value: "/data/home/.claude-work"},
		},
	}
	if err := profileStore.CreateAgentProfile(ctx, source); err != nil {
		t.Fatalf("create source profile: %v", err)
	}
	target := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "Researcher",
		Role:        models.AgentRoleSpecialist,
	}
	if err := svc.CreateAgentInstance(ctx, target); err != nil {
		t.Fatalf("create office agent: %v", err)
	}

	rec := newPatchAgentRecorder(t, svc, target.ID, `{"agent_profile_id":"source-profile"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	stored, err := profileStore.GetAgentProfile(ctx, target.ID)
	if err != nil {
		t.Fatalf("get updated office agent: %v", err)
	}
	if stored.Name != "Researcher" || stored.Role != models.AgentRoleSpecialist || stored.WorkspaceID != "ws-1" {
		t.Fatalf("office identity changed: name=%q role=%q workspace=%q", stored.Name, stored.Role, stored.WorkspaceID)
	}
	if stored.Model == "opus" || stored.Mode == "bypassPermissions" {
		t.Errorf("Office runtime fields were overwritten: model=%q mode=%q", stored.Model, stored.Mode)
	}
	if stored.AutoApprove || len(stored.CLIFlags) != 0 {
		t.Errorf("Office permission fields were overwritten: auto=%v flags=%+v", stored.AutoApprove, stored.CLIFlags)
	}
	if len(stored.ConfigOptions) != 0 {
		t.Errorf("Office config options were overwritten: %+v", stored.ConfigOptions)
	}
	if len(stored.EnvVars) != 0 {
		t.Errorf("Office environment was overwritten: %+v", stored.EnvVars)
	}
}

func TestApplyProfileConfigurationCopiesOnlyProviderFamily(t *testing.T) {
	svc, _, profileStore := newTestAgentServiceWithProfileStore(t)
	ctx := context.Background()
	provider := &settingsmodels.Agent{ID: "provider-db-id", Name: "claude-acp"}
	if err := profileStore.CreateAgent(ctx, provider); err != nil {
		t.Fatalf("create provider: %v", err)
	}
	source := &settingsmodels.AgentProfile{
		ID: "source-profile", AgentID: provider.ID, Name: "Work", Model: "opus",
		AutoApprove: true, EnvVars: []settingsmodels.ProfileEnvVar{{Key: "SECRET", Value: "value"}},
	}
	if err := profileStore.CreateAgentProfile(ctx, source); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	target := &models.AgentInstance{WorkspaceID: "ws-1", Name: "CTO", Model: "identity-model"}
	if err := svc.ApplyProfileConfiguration(ctx, target, source.ID); err != nil {
		t.Fatalf("assign provider family: %v", err)
	}
	if target.AgentID != provider.ID {
		t.Fatalf("agent family = %q, want %q", target.AgentID, provider.ID)
	}
	if target.Model != "identity-model" || target.AutoApprove || len(target.EnvVars) != 0 {
		t.Fatalf("runtime config copied onto Office identity: %+v", target)
	}
}

func TestApplyProfileConfiguration_RejectsCrossWorkspaceSource(t *testing.T) {
	svc, _, profileStore := newTestAgentServiceWithProfileStore(t)
	ctx := context.Background()
	provider := &settingsmodels.Agent{ID: "provider-db-id", Name: "claude-acp"}
	if err := profileStore.CreateAgent(ctx, provider); err != nil {
		t.Fatalf("create provider: %v", err)
	}
	source := &settingsmodels.AgentProfile{
		ID:          "other-workspace-profile",
		AgentID:     provider.ID,
		Name:        "Private",
		WorkspaceID: "ws-other",
		EnvVars: []settingsmodels.ProfileEnvVar{
			{Key: "PRIVATE_CONFIG", Value: "/private/path"},
		},
	}
	if err := profileStore.CreateAgentProfile(ctx, source); err != nil {
		t.Fatalf("create source profile: %v", err)
	}
	target := &models.AgentInstance{WorkspaceID: "ws-1", Name: "Worker"}

	err := svc.ApplyProfileConfiguration(ctx, target, source.ID)
	if err == nil {
		t.Fatal("expected cross-workspace source profile to be rejected")
	}
	if len(target.EnvVars) != 0 {
		t.Fatalf("cross-workspace environment was copied: %+v", target.EnvVars)
	}
}

func TestCreateAgentInstanceWithCaller_PersistsPendingAgentWhenApprovalRequired(t *testing.T) {
	svc, repo := newTestAgentService(t)
	ctx := context.Background()
	approvalCreator := &fakeApprovalCreator{}
	svc.SetGovernanceSettings(&fakeGovernanceSettings{requireNewAgents: true})
	svc.SetGovernanceApproval(approvalCreator)

	caller := &models.AgentInstance{ID: "creator-1", Name: "CEO", Role: models.AgentRoleCEO}
	agent := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "QA Reviewer",
		Role:        models.AgentRoleQA,
	}

	if err := svc.CreateAgentInstanceWithCaller(ctx, agent, caller, "expand testing"); err != nil {
		t.Fatalf("CreateAgentInstanceWithCaller: %v", err)
	}

	stored, err := repo.GetAgentInstance(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetAgentInstance: %v", err)
	}
	if stored.Status != models.AgentStatusPendingApproval {
		t.Fatalf("status = %q, want pending_approval", stored.Status)
	}
	if stored.Permissions == "" || stored.Permissions == "{}" {
		t.Fatal("expected default permissions on pending agent")
	}
	if len(approvalCreator.approvals) != 1 {
		t.Fatalf("approvals = %d, want 1", len(approvalCreator.approvals))
	}

	approval := approvalCreator.approvals[0]
	if approval.Type != models.ApprovalTypeHireAgent {
		t.Fatalf("approval type = %q, want hire_agent", approval.Type)
	}
	if approval.RequestedByAgentProfileID != caller.ID {
		t.Fatalf("requested_by = %q, want %q", approval.RequestedByAgentProfileID, caller.ID)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(approval.Payload), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["agent_profile_id"] != agent.ID {
		t.Errorf("agent_profile_id = %v, want %q", payload["agent_profile_id"], agent.ID)
	}
	if payload["creator_agent_id"] != caller.ID {
		t.Errorf("creator_agent_id = %v, want %q", payload["creator_agent_id"], caller.ID)
	}
	if payload["permissions"] != shared.DefaultPermissions(shared.AgentRoleQA) {
		t.Errorf("permissions = %v, want role defaults", payload["permissions"])
	}
}

func TestCreateAgentInstanceWithCaller_InheritsCallerExecutorPreference(t *testing.T) {
	svc, repo := newTestAgentService(t)
	ctx := context.Background()

	caller := &models.AgentInstance{
		ID:                 "creator-1",
		WorkspaceID:        "ws-1",
		Name:               "CEO",
		Role:               models.AgentRoleCEO,
		ExecutorPreference: `{"type":"local_pc"}`,
	}
	agent := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "Worker",
		Role:        models.AgentRoleWorker,
	}

	if err := svc.CreateAgentInstanceWithCaller(ctx, agent, caller, "delegate"); err != nil {
		t.Fatalf("CreateAgentInstanceWithCaller: %v", err)
	}

	stored, err := repo.GetAgentInstance(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetAgentInstance: %v", err)
	}
	if stored.ExecutorPreference != caller.ExecutorPreference {
		t.Fatalf("executor_preference = %q, want inherited %q",
			stored.ExecutorPreference, caller.ExecutorPreference)
	}
}

func TestCreateAgentInstanceWithCaller_UICreateBypassesGovernance(t *testing.T) {
	svc, _ := newTestAgentService(t)
	ctx := context.Background()
	approvalCreator := &fakeApprovalCreator{}
	svc.SetGovernanceSettings(&fakeGovernanceSettings{requireNewAgents: true})
	svc.SetGovernanceApproval(approvalCreator)

	agent := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "Frontend Worker",
		Role:        models.AgentRoleWorker,
	}

	if err := svc.CreateAgentInstanceWithCaller(ctx, agent, nil, ""); err != nil {
		t.Fatalf("CreateAgentInstanceWithCaller: %v", err)
	}
	if agent.Status != models.AgentStatusIdle {
		t.Fatalf("status = %q, want idle", agent.Status)
	}
	if len(approvalCreator.approvals) != 0 {
		t.Fatalf("approvals = %d, want 0", len(approvalCreator.approvals))
	}
}
