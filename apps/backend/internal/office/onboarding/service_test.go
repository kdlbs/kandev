package onboarding

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/configloader"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routing"
)

// --- mock implementations ---

type mockWorkspaceCreator struct {
	workspaces map[string]string // name -> ID (name is used as ID for simplicity)
}

func newMockWorkspaceCreator() *mockWorkspaceCreator {
	return &mockWorkspaceCreator{workspaces: make(map[string]string)}
}

func (m *mockWorkspaceCreator) CreateWorkspace(_ context.Context, name, _ string) error {
	m.workspaces[name] = name
	return nil
}

func (m *mockWorkspaceCreator) FindWorkspaceIDByName(_ context.Context, name string) (string, error) {
	return m.workspaces[name], nil
}

func (m *mockWorkspaceCreator) ListWorkspaceNames(_ context.Context) ([]string, error) {
	names := make([]string, 0, len(m.workspaces))
	for n := range m.workspaces {
		names = append(names, n)
	}
	return names, nil
}

type mockTaskCreatorOnboarding struct {
	calls []mockTaskCallOnboarding
}

type mockTaskCallOnboarding struct {
	WorkspaceID, ProjectID, AssigneeAgentID, WorkflowID, Title, Description string
}

func (m *mockTaskCreatorOnboarding) CreateOfficeTask(_ context.Context, wsID, projID, agentID, title, desc string) (string, error) {
	m.calls = append(m.calls, mockTaskCallOnboarding{
		WorkspaceID: wsID, ProjectID: projID,
		AssigneeAgentID: agentID, Title: title, Description: desc,
	})
	return "task-001", nil
}

func (m *mockTaskCreatorOnboarding) CreateOfficeTaskInWorkflow(
	_ context.Context, wsID, projID, agentID, workflowID, title, desc string,
) (string, error) {
	m.calls = append(m.calls, mockTaskCallOnboarding{
		WorkspaceID: wsID, ProjectID: projID,
		AssigneeAgentID: agentID, WorkflowID: workflowID,
		Title: title, Description: desc,
	})
	return "task-coord", nil
}

// mockWorkflowEnsurer is a stub WorkflowEnsurer that returns deterministic
// workflow IDs so onboarding tests can verify routing without wiring the
// full task repository.
type mockWorkflowEnsurer struct {
	officeID  string
	defaultID string
}

func (m *mockWorkflowEnsurer) EnsureOfficeWorkflow(_ context.Context, _ string) (string, error) {
	return m.officeID, nil
}

func (m *mockWorkflowEnsurer) EnsureOfficeDefaultWorkflow(_ context.Context, _ string) (string, error) {
	return m.defaultID, nil
}

// newMockWorkflowEnsurer returns a workflow ensurer pre-loaded with the
// canonical built-in workflow IDs the onboarding tests assert on.
func newMockWorkflowEnsurer() *mockWorkflowEnsurer {
	return &mockWorkflowEnsurer{
		officeID:  "wf-office",
		defaultID: "wf-office",
	}
}

type fakeSourceProfileReader struct {
	profiles map[string]*models.AgentInstance
	agents   map[string]*settingsmodels.Agent
}

func (f fakeSourceProfileReader) GetAgentProfile(_ context.Context, id string) (*models.AgentInstance, error) {
	profile := f.profiles[id]
	if profile == nil {
		return nil, os.ErrNotExist
	}
	return profile, nil
}

func (f fakeSourceProfileReader) GetAgent(_ context.Context, id string) (*settingsmodels.Agent, error) {
	agent := f.agents[id]
	if agent == nil {
		return nil, os.ErrNotExist
	}
	return agent, nil
}

type capturingAgentCreator struct {
	agent *models.AgentInstance
}

func TestResolveProviderIDRejectsUnsupportedAgentName(t *testing.T) {
	svc := &OnboardingService{sourceProfile: fakeSourceProfileReader{
		agents: map[string]*settingsmodels.Agent{
			"provider-db-id": {ID: "provider-db-id", Name: "custom-unknown-provider"},
		},
	}}
	if _, err := svc.resolveProviderID(context.Background(), "provider-db-id"); err == nil {
		t.Fatal("expected unsupported provider to be rejected")
	}
}

func (c *capturingAgentCreator) CreateAgentInstance(_ context.Context, agent *models.AgentInstance) error {
	c.agent = agent
	return nil
}

// newTestOnboardingServiceWithRepo is like newTestOnboardingService but
// also returns the concrete sqlite repo so callers can assert on rows
// the service writes (e.g. office_workspace_routing) without needing to
// re-open the in-memory DB.
func newTestOnboardingServiceWithRepo(t *testing.T) (*OnboardingService, *mockWorkspaceCreator, *sqlite.Repository, string) {
	t.Helper()
	svc, wsCreator, tmpDir := newTestOnboardingService(t)
	return svc, wsCreator, svc.repo.(*sqlite.Repository), tmpDir
}

// newTestOnboardingService creates an OnboardingService for testing.
// Returns the service, a mock workspace creator, and the tmpDir path.
func newTestOnboardingService(t *testing.T, opts ...func(*OnboardingService)) (*OnboardingService, *mockWorkspaceCreator, string) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}

	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	tmpDir := t.TempDir()
	wsDir := filepath.Join(tmpDir, "workspaces", "default")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsDir, "kandev.yml"), []byte("name: default\nslug: default\n"), 0o644); err != nil {
		t.Fatalf("write kandev.yml: %v", err)
	}
	loader := configloader.NewConfigLoader(tmpDir)
	if err := loader.Load(); err != nil {
		t.Fatalf("load config: %v", err)
	}
	writer := configloader.NewFileWriter(tmpDir, loader)
	log := logger.Default()

	wsCreator := newMockWorkspaceCreator()

	svc := NewOnboardingService(
		repo, loader, writer, log,
		nil,       // agents (AgentReader) - not needed for these tests
		nil,       // sourceProfile (SourceProfileReader) - not needed for these tests
		repo,      // agentCreator - sqlite.Repository implements CreateAgentInstance
		wsCreator, // workspaceCreator
		nil,       // workflowEnsurer
		nil,       // taskCreator - overridden per test as needed
		nil,       // runQueuer
		nil,       // configSyncer
	)
	return svc, wsCreator, tmpDir
}

func TestGetOnboardingState_InitiallyNotCompleted(t *testing.T) {
	svc, _, _ := newTestOnboardingService(t)
	ctx := context.Background()

	state, err := svc.GetOnboardingState(ctx)
	if err != nil {
		t.Fatalf("get onboarding state: %v", err)
	}
	if state.Completed {
		t.Error("expected completed=false initially")
	}
}

func TestGetOnboardingState_ReportsFSWorkspaces(t *testing.T) {
	svc, _, _ := newTestOnboardingService(t)
	ctx := context.Background()

	state, err := svc.GetOnboardingState(ctx)
	if err != nil {
		t.Fatalf("get onboarding state: %v", err)
	}
	// newTestOnboardingService creates a "default" workspace on the FS.
	if len(state.FSWorkspaces) == 0 {
		t.Error("expected at least one FS workspace")
	}
	if state.FSWorkspaces[0].Name != "default" {
		t.Errorf("expected FS workspace name 'default', got %q", state.FSWorkspaces[0].Name)
	}
}

func TestCompleteOnboarding_CreatesEntities(t *testing.T) {
	svc, _, _ := newTestOnboardingService(t)
	ctx := context.Background()

	result, err := svc.CompleteOnboarding(ctx, CompleteRequest{
		WorkspaceName:      "default",
		TaskPrefix:         "TST",
		AgentName:          "CEO",
		AgentProfileID:     "",
		ExecutorPreference: "local_pc",
	})
	if err != nil {
		t.Fatalf("complete onboarding: %v", err)
	}
	if result.WorkspaceID == "" {
		t.Error("expected non-empty workspace ID")
	}
	if result.AgentID == "" {
		t.Error("expected non-empty agent ID")
	}

	// Verify onboarding is now completed.
	state, err := svc.GetOnboardingState(ctx)
	if err != nil {
		t.Fatalf("get state after complete: %v", err)
	}
	if !state.Completed {
		t.Error("expected completed=true after CompleteOnboarding")
	}
	if state.WorkspaceID != result.WorkspaceID {
		t.Errorf("state.WorkspaceID = %q, want %q", state.WorkspaceID, result.WorkspaceID)
	}
	if state.CEOAgentID != result.AgentID {
		t.Errorf("state.CEOAgentID = %q, want %q", state.CEOAgentID, result.AgentID)
	}
}

func TestCreateOnboardingAgent_KeepsRuntimeConfigurationIndirect(t *testing.T) {
	svc, _, _ := newTestOnboardingService(t)
	source := &models.AgentInstance{
		AgentID:       "provider-db-id",
		Model:         "opus",
		Mode:          "bypassPermissions",
		ConfigOptions: map[string]string{"effort": "high"},
		AutoApprove:   true,
		CLIFlags: []settingsmodels.CLIFlag{
			{Flag: "--dangerously-skip-permissions", Enabled: true},
		},
		EnvVars: []settingsmodels.ProfileEnvVar{
			{Key: "CLAUDE_CONFIG_DIR", Value: "/data/home/.claude-work"},
		},
	}
	svc.sourceProfile = fakeSourceProfileReader{profiles: map[string]*models.AgentInstance{
		"source-profile": source,
	}}
	capture := &capturingAgentCreator{}
	svc.agentCreator = capture

	if _, err := svc.createOnboardingAgent(context.Background(), "ws-1", CompleteRequest{
		AgentName: "CEO", AgentProfileID: "source-profile",
	}); err != nil {
		t.Fatalf("create onboarding agent: %v", err)
	}
	if capture.agent == nil {
		t.Fatal("agent was not created")
	}
	if capture.agent.AgentID != "provider-db-id" {
		t.Errorf("legacy provider family = %q", capture.agent.AgentID)
	}
	if len(capture.agent.ConfigOptions) != 0 || capture.agent.AutoApprove {
		t.Errorf("runtime options copied onto Office identity: %+v", capture.agent)
	}
	if len(capture.agent.CLIFlags) != 0 || len(capture.agent.EnvVars) != 0 {
		t.Errorf("CLI configuration copied onto Office identity: flags=%+v env=%+v",
			capture.agent.CLIFlags, capture.agent.EnvVars)
	}
}

func findOnboardingTaskCall(calls []mockTaskCallOnboarding, title string) *mockTaskCallOnboarding {
	for i := range calls {
		if calls[i].Title == title {
			return &calls[i]
		}
	}
	return nil
}

func TestCompleteOnboarding_CreatesTask(t *testing.T) {
	svc, _, _ := newTestOnboardingService(t)
	mock := &mockTaskCreatorOnboarding{}
	svc.taskCreator = mock
	wfEnsurer := newMockWorkflowEnsurer()
	svc.workflowEnsurer = wfEnsurer
	ctx := context.Background()

	result, err := svc.CompleteOnboarding(ctx, CompleteRequest{
		WorkspaceName:      "default",
		TaskPrefix:         "TST",
		AgentName:          "CEO",
		AgentProfileID:     "",
		ExecutorPreference: "local_pc",
		TaskTitle:          "Explore the codebase",
		TaskDescription:    "Create an engineering roadmap",
	})
	if err != nil {
		t.Fatalf("complete onboarding: %v", err)
	}
	if result.TaskID != "task-001" {
		t.Errorf("expected TaskID 'task-001', got %q", result.TaskID)
	}
	// Onboarding creates a single task — the user-supplied onboarding task.
	// The standing coordination task is retired post-PR3 of office-heartbeat-
	// rework; the agent-level heartbeat path produces fresh taskless runs.
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 CreateOfficeTask call (onboarding only), got %d", len(mock.calls))
	}
	onboarding := findOnboardingTaskCall(mock.calls, "Explore the codebase")
	if onboarding == nil {
		t.Fatalf("onboarding task call not found")
	}
	if onboarding.Description != "Create an engineering roadmap" {
		t.Errorf("task description = %q, want 'Create an engineering roadmap'", onboarding.Description)
	}
	if onboarding.AssigneeAgentID != result.AgentID {
		t.Errorf("task assignee = %q, want %q", onboarding.AssigneeAgentID, result.AgentID)
	}
	if onboarding.WorkflowID != "" {
		t.Errorf("onboarding task workflow_id = %q, want empty (routes via office_workflow_id)", onboarding.WorkflowID)
	}
	// No standing coordination task is created — the agent-level
	// heartbeat path produces fresh taskless runs instead.
	if len(mock.calls) != 1 {
		t.Errorf("expected 1 task creation (the onboarding task), got %d", len(mock.calls))
	}
}

func TestDefaultOnboardingBriefMutationsHaveCEOCapabilityCatalog(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve onboarding test source path")
	}
	backendDir := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "../../.."))
	appsDir := filepath.Dir(backendDir)

	brief := readOnboardingContractFile(t,
		filepath.Join(appsDir, "web/app/office/setup/setup-task-defaults.ts"))
	officeContext := readOnboardingContractFile(t,
		filepath.Join(backendDir, "config/prompts/office-context.md"))
	projectSkill := readOnboardingContractFile(t,
		filepath.Join(backendDir, "internal/office/configloader/skills/kandev-projects/SKILL.md"))
	hiringSkill := readOnboardingContractFile(t,
		filepath.Join(backendDir, "internal/office/configloader/skills/kandev-team-admin/references/hiring.md"))

	contracts := []struct {
		briefMutation string
		catalog       string
		capability    string
	}{
		{"create one project per repository", projectSkill, "kandev projects create"},
		{"create the agent team", hiringSkill, "kandev agents create"},
		{"responsibilities, permissions, and operating guidance", hiringSkill, "--role"},
		{"draft a proposed plan", officeContext, "create_task_plan_kandev"},
		{"questions for the human", officeContext, "ask_user_question_kandev"},
	}
	brief = strings.ToLower(brief)
	for _, contract := range contracts {
		t.Run(contract.briefMutation, func(t *testing.T) {
			if !strings.Contains(brief, contract.briefMutation) {
				t.Fatalf("default onboarding brief no longer contains %q; update this contract", contract.briefMutation)
			}
			if !strings.Contains(strings.ToLower(contract.catalog), strings.ToLower(contract.capability)) {
				t.Fatalf("brief mutation %q has no advertised CEO capability %q",
					contract.briefMutation, contract.capability)
			}
		})
	}
	for _, unsupported := range []string{"create a workspace", "create workspaces", "step_complete_kandev"} {
		if strings.Contains(brief, unsupported) {
			t.Errorf("default onboarding brief requests unsupported Office mutation %q", unsupported)
		}
	}
}

func readOnboardingContractFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read onboarding contract file %s: %v", path, err)
	}
	return string(content)
}

func TestCompleteOnboarding_NoTaskWhenTitleEmpty(t *testing.T) {
	svc, _, _ := newTestOnboardingService(t)
	mock := &mockTaskCreatorOnboarding{}
	svc.taskCreator = mock
	svc.workflowEnsurer = newMockWorkflowEnsurer()
	ctx := context.Background()

	_, err := svc.CompleteOnboarding(ctx, CompleteRequest{
		WorkspaceName:      "default",
		TaskPrefix:         "TST",
		AgentName:          "CEO",
		AgentProfileID:     "",
		ExecutorPreference: "local_pc",
		TaskTitle:          "",
	})
	if err != nil {
		t.Fatalf("complete onboarding: %v", err)
	}
	// No standing coordination task is created (retired post-PR3) and
	// the onboarding task is skipped because TaskTitle is empty.
	if len(mock.calls) != 0 {
		t.Errorf("expected 0 task creations, got %d", len(mock.calls))
	}
}

func TestGenerateWorkspaceSlug(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"default", "default"},
		{"My Workspace", "my-workspace"},
		{"Hello  World!!", "hello-world"},
		{"---trimmed---", "trimmed"},
		{"", "workspace"},
		{"UPPER_case MiXed", "upper-case-mixed"},
		{
			"a-very-long-workspace-name-that-exceeds-fifty-characters-limit-here",
			"a-very-long-workspace-name-that-exceeds-fifty-char",
		},
	}
	for _, tt := range tests {
		got := generateSlug(tt.input)
		if got != tt.want {
			t.Errorf("generateSlug(%q) = %q, want %q", tt.input, tt.want, got)
		}
	}
}

func TestCompleteOnboarding_RequiresWorkspaceName(t *testing.T) {
	svc, _, _ := newTestOnboardingService(t)
	ctx := context.Background()

	_, err := svc.CompleteOnboarding(ctx, CompleteRequest{
		WorkspaceName: "",
		AgentName:     "CEO",
	})
	if err == nil {
		t.Error("expected error for empty workspace name")
	}
}

func TestGetOnboardingState_FiltersImportedFSWorkspaces(t *testing.T) {
	svc, wsCreator, _ := newTestOnboardingService(t)
	ctx := context.Background()

	// Simulate that the "default" FS workspace already has a DB row.
	wsCreator.workspaces["default"] = "default"

	state, err := svc.GetOnboardingState(ctx)
	if err != nil {
		t.Fatalf("get onboarding state: %v", err)
	}
	for _, ws := range state.FSWorkspaces {
		if ws.Name == "default" {
			t.Error("expected imported FS workspace 'default' to be filtered out")
		}
	}
}

func TestImportFromFS_SkipsAlreadyImported(t *testing.T) {
	// Set up a temp dir with two FS workspaces.
	tmpDir := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		wsDir := filepath.Join(tmpDir, "workspaces", name)
		if err := os.MkdirAll(wsDir, 0o755); err != nil {
			t.Fatalf("mkdir workspace %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(wsDir, "kandev.yml"), []byte("name: "+name+"\nslug: "+name+"\n"), 0o644); err != nil {
			t.Fatalf("write kandev.yml for %s: %v", name, err)
		}
	}
	loader := configloader.NewConfigLoader(tmpDir)
	if err := loader.Load(); err != nil {
		t.Fatalf("load config: %v", err)
	}
	writer := configloader.NewFileWriter(tmpDir, loader)

	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}

	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	wsCreator := newMockWorkspaceCreator()
	wsCreator.workspaces["alpha"] = "alpha-id" // simulate alpha already imported

	svc := NewOnboardingService(
		repo, loader, writer, logger.Default(),
		nil, nil, repo, wsCreator, nil, nil, nil, nil,
	)
	ctx := context.Background()

	result, err := svc.ImportFromFS(ctx)
	if err != nil {
		t.Fatalf("import from fs: %v", err)
	}
	if len(result.WorkspaceIDs) != 1 {
		t.Fatalf("expected 1 imported workspace, got %d", len(result.WorkspaceIDs))
	}
	if result.WorkspaceIDs[0] != "beta" {
		t.Errorf("expected imported workspace 'beta', got %q", result.WorkspaceIDs[0])
	}
}

// onboardingRoutingScenario is one row in the default-tier seeding
// table. It captures the wizard's tier input plus the expected
// outcome on the persisted office_workspace_routing row.
type onboardingRoutingScenario struct {
	name        string
	inputTier   string
	wantTier    routing.Tier
	wantTierKey string // which TierMap field carries the chosen model
}

// completeOnboardingForRoutingSeed runs CompleteOnboarding against a
// fresh repo so each scenario gets its own workspace + agent. Returns
// the persisted routing config + the freshly created agent.
func completeOnboardingForRoutingSeed(
	t *testing.T, sc onboardingRoutingScenario,
) (*routing.WorkspaceConfig, *seededAgent) {
	t.Helper()
	svc, _, repo, _ := newTestOnboardingServiceWithRepo(t)
	ctx := context.Background()

	result, err := svc.CompleteOnboarding(ctx, CompleteRequest{
		WorkspaceName:      sc.name,
		TaskPrefix:         "RTG",
		AgentName:          "CEO",
		AgentProfileID:     "",
		ExecutorPreference: "local_pc",
		DefaultTier:        sc.inputTier,
	})
	if err != nil {
		t.Fatalf("complete onboarding: %v", err)
	}

	cfg, err := repo.GetWorkspaceRouting(ctx, result.WorkspaceID)
	if err != nil {
		t.Fatalf("get workspace routing: %v", err)
	}
	agents, err := svc.repo.ListAgentInstances(ctx, result.WorkspaceID)
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	var agent *seededAgent
	for _, a := range agents {
		if a != nil && a.ID == result.AgentID {
			agent = &seededAgent{ID: a.ID, AgentID: a.AgentID, Model: a.Model, Settings: a.Settings}
			break
		}
	}
	return cfg, agent
}

type seededAgent struct {
	ID       string
	AgentID  string
	Model    string
	Settings string
}

func TestCompleteOnboarding_SeedsWorkspaceRoutingTier(t *testing.T) {
	scenarios := []onboardingRoutingScenario{
		{name: "frontier-wins", inputTier: "frontier", wantTier: routing.TierFrontier, wantTierKey: "frontier"},
		{name: "empty-defaults-balanced", inputTier: "", wantTier: routing.TierBalanced, wantTierKey: "balanced"},
		{name: "invalid-defaults-balanced", inputTier: "gibberish", wantTier: routing.TierBalanced, wantTierKey: "balanced"},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			cfg, _ := completeOnboardingForRoutingSeed(t, sc)
			if cfg == nil {
				t.Fatalf("expected workspace routing row")
			}
			if cfg.Enabled {
				t.Errorf("expected enabled=false (advanced opt-in), got true")
			}
			if cfg.DefaultTier != sc.wantTier {
				t.Errorf("default tier = %q, want %q", cfg.DefaultTier, sc.wantTier)
			}
		})
	}
}

func TestCompleteOnboarding_SeedsWorkspaceRoutingTierProfiles(t *testing.T) {
	svc, _, repo, _ := newTestOnboardingServiceWithRepo(t)
	svc.sourceProfile = fakeSourceProfileReader{
		agents: map[string]*settingsmodels.Agent{
			"provider-db-id": {ID: "provider-db-id", Name: "codex-acp"},
		},
		profiles: map[string]*models.AgentInstance{
			"profile-frontier": {
				AgentID: "provider-db-id",
				Model:   "gpt-5.5-high",
				Mode:    "max",
			},
			"profile-balanced": {
				AgentID: "provider-db-id",
				Model:   "gpt-5.5-medium",
				Mode:    "medium",
			},
			"profile-economy": {
				AgentID: "provider-db-id",
				Model:   "gpt-5.5-low",
				Mode:    "low",
			},
		}}
	ctx := context.Background()

	result, err := svc.CompleteOnboarding(ctx, CompleteRequest{
		WorkspaceName:      "tier-profiles",
		TaskPrefix:         "TP",
		AgentName:          "CEO",
		AgentProfileID:     "profile-balanced",
		ExecutorPreference: "local_pc",
		DefaultTier:        "balanced",
		TierProfiles: TierProfileIDs{
			Frontier: "profile-frontier",
			Balanced: "profile-balanced",
			Economy:  "profile-economy",
		},
	})
	if err != nil {
		t.Fatalf("complete onboarding: %v", err)
	}

	cfg, err := repo.GetWorkspaceRouting(ctx, result.WorkspaceID)
	if err != nil {
		t.Fatalf("get workspace routing: %v", err)
	}
	profile := cfg.ProviderProfiles["codex-acp"]
	if profile.TierMap.Frontier != "gpt-5.5-high" {
		t.Errorf("frontier tier map = %q", profile.TierMap.Frontier)
	}
	if profile.TierMap.Balanced != "gpt-5.5-medium" {
		t.Errorf("balanced tier map = %q", profile.TierMap.Balanced)
	}
	if profile.TierMap.Economy != "gpt-5.5-low" {
		t.Errorf("economy tier map = %q", profile.TierMap.Economy)
	}
	if profile.TierProfileIDs != (routing.TierProfileIDs{
		Frontier: "profile-frontier",
		Balanced: "profile-balanced",
		Economy:  "profile-economy",
	}) {
		t.Errorf("tier profile ids = %+v", profile.TierProfileIDs)
	}
}

func TestCompleteOnboarding_TierProfileLookupFailureKeepsValidSelections(t *testing.T) {
	svc, _, repo, _ := newTestOnboardingServiceWithRepo(t)
	svc.sourceProfile = fakeSourceProfileReader{profiles: map[string]*models.AgentInstance{
		"profile-balanced": {
			AgentID: "codex-acp",
			Model:   "gpt-5.5-medium",
			Mode:    "medium",
		},
		"profile-economy": {
			AgentID: "codex-acp",
			Model:   "gpt-5.5-low",
			Mode:    "low",
		},
	}}
	ctx := context.Background()

	result, err := svc.CompleteOnboarding(ctx, CompleteRequest{
		WorkspaceName:      "partial-tier-profiles",
		TaskPrefix:         "PTP",
		AgentName:          "CEO",
		AgentProfileID:     "profile-balanced",
		ExecutorPreference: "local_pc",
		DefaultTier:        "balanced",
		TierProfiles: TierProfileIDs{
			Frontier: "missing-frontier",
			Balanced: "profile-balanced",
			Economy:  "profile-economy",
		},
	})
	if err != nil {
		t.Fatalf("complete onboarding: %v", err)
	}

	cfg, err := repo.GetWorkspaceRouting(ctx, result.WorkspaceID)
	if err != nil {
		t.Fatalf("get workspace routing: %v", err)
	}
	profile := cfg.ProviderProfiles["codex-acp"]
	if profile.TierMap.Balanced != "gpt-5.5-medium" {
		t.Errorf("balanced tier map = %q", profile.TierMap.Balanced)
	}
	if profile.TierMap.Economy != "gpt-5.5-low" {
		t.Errorf("economy tier map = %q", profile.TierMap.Economy)
	}
	if profile.TierProfileIDs.Balanced != "profile-balanced" ||
		profile.TierProfileIDs.Economy != "profile-economy" {
		t.Errorf("valid tier profile ids were not preserved: %+v", profile.TierProfileIDs)
	}
}

func TestCompleteOnboarding_SeedsCEOInheritMarkers(t *testing.T) {
	_, agent := completeOnboardingForRoutingSeed(t, onboardingRoutingScenario{
		name: "inherit-markers", inputTier: "balanced",
		wantTier: routing.TierBalanced, wantTierKey: "balanced",
	})
	if agent == nil {
		t.Fatalf("expected seeded agent")
	}
	ov, err := routing.ReadAgentOverrides(agent.Settings)
	if err != nil {
		t.Fatalf("read overrides: %v", err)
	}
	if ov.TierSource != "inherit" {
		t.Errorf("tier_source = %q, want inherit", ov.TierSource)
	}
	if ov.ProviderOrderSource != "inherit" {
		t.Errorf("provider_order_source = %q, want inherit", ov.ProviderOrderSource)
	}
}

func TestCompleteOnboarding_TwiceCreatesTwoWorkspaces(t *testing.T) {
	svc, _, _ := newTestOnboardingService(t)
	ctx := context.Background()

	result1, err := svc.CompleteOnboarding(ctx, CompleteRequest{
		WorkspaceName:      "First WS",
		TaskPrefix:         "TST",
		AgentName:          "CEO",
		AgentProfileID:     "",
		ExecutorPreference: "local_pc",
	})
	if err != nil {
		t.Fatalf("first complete onboarding: %v", err)
	}
	if result1.WorkspaceID == "" {
		t.Error("expected non-empty workspace ID for first workspace")
	}

	result2, err := svc.CompleteOnboarding(ctx, CompleteRequest{
		WorkspaceName:      "Second WS",
		TaskPrefix:         "TST",
		AgentName:          "CEO",
		AgentProfileID:     "",
		ExecutorPreference: "local_pc",
	})
	if err != nil {
		t.Fatalf("second complete onboarding: %v", err)
	}
	if result2.WorkspaceID == "" {
		t.Error("expected non-empty workspace ID for second workspace")
	}
	if result1.WorkspaceID == result2.WorkspaceID {
		t.Error("expected distinct workspace IDs for two completions")
	}
}
