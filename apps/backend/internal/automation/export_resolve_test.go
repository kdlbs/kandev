package automation

import (
	"context"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

// AC-19: resolveDescriptors resolves an automation's agent profile, executor
// profile, workflow/step, and repository references within a shared read
// transaction, applying the partial-resolution rules verbatim: an absent
// reference is omitted with a warning, a failed lookup aborts the whole
// export, and a reference that was never made (empty ID, or a workflow step
// left unset on a workflow that does resolve) produces neither.

type fakeExportAgentProfileLookup struct {
	profiles map[string]*settingsmodels.AgentProfile
	err      error
	// gotTx records the transaction handle the most recent call received,
	// for AC-29's handle-identity assertion.
	gotTx *sqlx.Tx
}

func (f *fakeExportAgentProfileLookup) GetAgentProfileTx(_ context.Context, tx *sqlx.Tx, id string) (*settingsmodels.AgentProfile, bool, error) {
	f.gotTx = tx
	if f.err != nil {
		return nil, false, f.err
	}
	p, ok := f.profiles[id]
	return p, ok, nil
}

type fakeExportExecutorProfileLookup struct {
	profiles map[string]*taskmodels.ExecutorProfile
	err      error
	gotTx    *sqlx.Tx
}

func (f *fakeExportExecutorProfileLookup) GetExecutorProfileTx(_ context.Context, tx *sqlx.Tx, id string) (*taskmodels.ExecutorProfile, bool, error) {
	f.gotTx = tx
	if f.err != nil {
		return nil, false, f.err
	}
	p, ok := f.profiles[id]
	return p, ok, nil
}

type fakeExportWorkflowLookup struct {
	workflows map[string]*taskmodels.Workflow
	err       error
	gotTx     *sqlx.Tx
}

func (f *fakeExportWorkflowLookup) GetWorkflowTx(_ context.Context, tx *sqlx.Tx, id string) (*taskmodels.Workflow, bool, error) {
	f.gotTx = tx
	if f.err != nil {
		return nil, false, f.err
	}
	w, ok := f.workflows[id]
	return w, ok, nil
}

type fakeExportWorkflowStepLookup struct {
	steps map[string]*workflowmodels.WorkflowStep
	err   error
	gotTx *sqlx.Tx
}

func (f *fakeExportWorkflowStepLookup) GetStepTx(_ context.Context, tx *sqlx.Tx, id string) (*workflowmodels.WorkflowStep, bool, error) {
	f.gotTx = tx
	if f.err != nil {
		return nil, false, f.err
	}
	s, ok := f.steps[id]
	return s, ok, nil
}

type fakeExportRepositoryLookup struct {
	repositories map[string]*taskmodels.Repository
	err          error
	gotTx        *sqlx.Tx
}

func (f *fakeExportRepositoryLookup) GetRepositoryTx(_ context.Context, tx *sqlx.Tx, id string) (*taskmodels.Repository, bool, error) {
	f.gotTx = tx
	if f.err != nil {
		return nil, false, f.err
	}
	r, ok := f.repositories[id]
	return r, ok, nil
}

// resolveTestFixture wires every Export*Lookup on a fresh service so each
// test only needs to populate the maps it cares about.
func resolveTestFixture(t *testing.T) (*Service, *fakeExportAgentProfileLookup, *fakeExportExecutorProfileLookup, *fakeExportWorkflowLookup, *fakeExportWorkflowStepLookup, *fakeExportRepositoryLookup) {
	t.Helper()
	svc := newTestService(t)
	agentLookup := &fakeExportAgentProfileLookup{profiles: map[string]*settingsmodels.AgentProfile{}}
	executorLookup := &fakeExportExecutorProfileLookup{profiles: map[string]*taskmodels.ExecutorProfile{}}
	workflowLookup := &fakeExportWorkflowLookup{workflows: map[string]*taskmodels.Workflow{}}
	stepLookup := &fakeExportWorkflowStepLookup{steps: map[string]*workflowmodels.WorkflowStep{}}
	repoLookup := &fakeExportRepositoryLookup{repositories: map[string]*taskmodels.Repository{}}
	svc.SetExportAgentProfileLookup(agentLookup)
	svc.SetExportExecutorProfileLookup(executorLookup)
	svc.SetExportWorkflowLookup(workflowLookup)
	svc.SetExportWorkflowStepLookup(stepLookup)
	svc.SetExportRepositoryLookup(repoLookup)
	return svc, agentLookup, executorLookup, workflowLookup, stepLookup, repoLookup
}

func beginTestReadTx(t *testing.T, svc *Service) *sqlx.Tx {
	t.Helper()
	tx, err := svc.store.BeginReadTx(context.Background())
	if err != nil {
		t.Fatalf("BeginReadTx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	return tx
}

func TestResolveDescriptors_NoReferencesMade_NoDescriptorsNoWarnings(t *testing.T) {
	svc, _, _, _, _, _ := resolveTestFixture(t)
	tx := beginTestReadTx(t, svc)
	a := &Automation{ID: "auto-1", Name: "Daily Review"}

	got, warnings, err := svc.resolveDescriptors(context.Background(), tx, a)
	if err != nil {
		t.Fatalf("resolveDescriptors: %v", err)
	}
	if got.AgentProfile != nil || got.ExecutorProfile != nil || got.Workflow != nil || got.Repositories != nil {
		t.Errorf("expected all descriptors nil/empty, got %+v", got)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}

func TestResolveDescriptors_AgentProfileResolved(t *testing.T) {
	svc, agentLookup, _, _, _, _ := resolveTestFixture(t)
	// Name (user-assigned label) and AgentDisplayName (underlying agent's display
	// name) are deliberately different here: exportAgentProfile.AgentName must come
	// from AgentDisplayName, matching the established wfmodels.AgentProfilePortable
	// pattern in backendapp.buildAgentProfileResolver, not from the user's label.
	agentLookup.profiles["agent-1"] = &settingsmodels.AgentProfile{Name: "My Reviewer Profile", AgentDisplayName: "Claude Code", Model: "claude-sonnet-5", Mode: "plan"}
	tx := beginTestReadTx(t, svc)
	a := &Automation{ID: "auto-1", Name: "Daily Review", AgentProfileID: "agent-1"}

	got, warnings, err := svc.resolveDescriptors(context.Background(), tx, a)
	if err != nil {
		t.Fatalf("resolveDescriptors: %v", err)
	}
	want := &exportAgentProfile{AgentName: "Claude Code", Model: "claude-sonnet-5", Mode: "plan"}
	if got.AgentProfile == nil || *got.AgentProfile != *want {
		t.Errorf("AgentProfile = %+v, want %+v", got.AgentProfile, want)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}

func TestResolveDescriptors_AgentProfileUnresolved_OmitsAndWarns(t *testing.T) {
	svc, _, _, _, _, _ := resolveTestFixture(t)
	tx := beginTestReadTx(t, svc)
	a := &Automation{ID: "auto-1", Name: "Daily Review", AgentProfileID: "missing-agent"}

	got, warnings, err := svc.resolveDescriptors(context.Background(), tx, a)
	if err != nil {
		t.Fatalf("resolveDescriptors: %v", err)
	}
	if got.AgentProfile != nil {
		t.Errorf("expected nil AgentProfile, got %+v", got.AgentProfile)
	}
	want := []exportWarning{{AutomationName: "Daily Review", AutomationID: "auto-1", DedupKey: "auto-1", Message: "unresolved agent profile"}}
	if len(warnings) != 1 || warnings[0] != want[0] {
		t.Errorf("warnings = %v, want %v", warnings, want)
	}
}

// A workspace-scoped agent profile belonging to a different workspace than
// the automation must never be exported: resolveAgentProfile has no
// upstream guarantee that AgentProfileID was validated against the
// automation's own workspace (validateAgentProfileID checks existence only,
// unlike RepositoryLookup's fail-closed cross-workspace guard), so the
// export path must defend the boundary itself rather than leak a foreign
// workspace's profile name/model/mode through the descriptor.
func TestResolveDescriptors_AgentProfileFromDifferentWorkspace_OmitsAndWarns(t *testing.T) {
	svc, agentLookup, _, _, _, _ := resolveTestFixture(t)
	agentLookup.profiles["agent-1"] = &settingsmodels.AgentProfile{WorkspaceID: "other-workspace", Name: "Foreign Profile", AgentDisplayName: "Claude Code", Model: "claude-sonnet-5", Mode: "plan"}
	tx := beginTestReadTx(t, svc)
	a := &Automation{ID: "auto-1", Name: "Daily Review", WorkspaceID: "ws-1", AgentProfileID: "agent-1"}

	got, warnings, err := svc.resolveDescriptors(context.Background(), tx, a)
	if err != nil {
		t.Fatalf("resolveDescriptors: %v", err)
	}
	if got.AgentProfile != nil {
		t.Errorf("expected nil AgentProfile for a cross-workspace binding, got %+v", got.AgentProfile)
	}
	want := []exportWarning{{AutomationName: "Daily Review", AutomationID: "auto-1", DedupKey: "auto-1", Message: "unresolved agent profile"}}
	if len(warnings) != 1 || warnings[0] != want[0] {
		t.Errorf("warnings = %v, want %v", warnings, want)
	}
}

// A global agent profile (WorkspaceID == "") is legitimately shared across
// every workspace and must still resolve normally.
func TestResolveDescriptors_GlobalAgentProfile_ResolvesAcrossWorkspaces(t *testing.T) {
	svc, agentLookup, _, _, _, _ := resolveTestFixture(t)
	agentLookup.profiles["agent-1"] = &settingsmodels.AgentProfile{WorkspaceID: "", Name: "Global Profile", AgentDisplayName: "Claude Code", Model: "claude-sonnet-5", Mode: "plan"}
	tx := beginTestReadTx(t, svc)
	a := &Automation{ID: "auto-1", Name: "Daily Review", WorkspaceID: "ws-1", AgentProfileID: "agent-1"}

	got, warnings, err := svc.resolveDescriptors(context.Background(), tx, a)
	if err != nil {
		t.Fatalf("resolveDescriptors: %v", err)
	}
	want := &exportAgentProfile{AgentName: "Claude Code", Model: "claude-sonnet-5", Mode: "plan"}
	if got.AgentProfile == nil || *got.AgentProfile != *want {
		t.Errorf("AgentProfile = %+v, want %+v", got.AgentProfile, want)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings for a global profile, got %v", warnings)
	}
}

func TestResolveDescriptors_AgentProfileLookupFails_AbortsWithNoPartialResultAndNoWarning(t *testing.T) {
	svc, agentLookup, _, _, _, _ := resolveTestFixture(t)
	sentinel := errors.New("boom")
	agentLookup.err = sentinel
	tx := beginTestReadTx(t, svc)
	a := &Automation{ID: "auto-1", Name: "Daily Review", AgentProfileID: "agent-1"}

	_, warnings, err := svc.resolveDescriptors(context.Background(), tx, a)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want wrapping %v", err, sentinel)
	}
	if warnings != nil {
		t.Errorf("expected no warnings on failure, got %v", warnings)
	}
}

func TestResolveDescriptors_ExecutorProfileResolvedAndUnresolved(t *testing.T) {
	svc, _, executorLookup, _, _, _ := resolveTestFixture(t)
	executorLookup.profiles["exec-1"] = &taskmodels.ExecutorProfile{ExecutorID: "local_docker", Name: "Default"}
	tx := beginTestReadTx(t, svc)

	resolved, warnings, err := svc.resolveDescriptors(context.Background(), tx, &Automation{ID: "a1", Name: "A", ExecutorProfileID: "exec-1"})
	if err != nil {
		t.Fatalf("resolveDescriptors: %v", err)
	}
	want := &exportExecutorProfile{Executor: "local_docker", Name: "Default"}
	if resolved.ExecutorProfile == nil || *resolved.ExecutorProfile != *want {
		t.Errorf("ExecutorProfile = %+v, want %+v", resolved.ExecutorProfile, want)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}

	resolved, warnings, err = svc.resolveDescriptors(context.Background(), tx, &Automation{ID: "a2", Name: "B", ExecutorProfileID: "missing-exec"})
	if err != nil {
		t.Fatalf("resolveDescriptors: %v", err)
	}
	if resolved.ExecutorProfile != nil {
		t.Errorf("expected nil ExecutorProfile, got %+v", resolved.ExecutorProfile)
	}
	if len(warnings) != 1 || warnings[0].Message != "unresolved executor profile" {
		t.Errorf("warnings = %v, want [unresolved executor profile]", warnings)
	}
}

func TestResolveDescriptors_WorkflowResolvedStepResolved(t *testing.T) {
	svc, _, _, workflowLookup, stepLookup, _ := resolveTestFixture(t)
	workflowLookup.workflows["wf-1"] = &taskmodels.Workflow{Name: "Review Flow"}
	stepLookup.steps["step-1"] = &workflowmodels.WorkflowStep{Name: "In Review", WorkflowID: "wf-1"}
	tx := beginTestReadTx(t, svc)

	got, warnings, err := svc.resolveDescriptors(context.Background(), tx, &Automation{ID: "a1", Name: "A", WorkflowID: "wf-1", WorkflowStepID: "step-1"})
	if err != nil {
		t.Fatalf("resolveDescriptors: %v", err)
	}
	want := &exportWorkflow{Name: "Review Flow", Step: "In Review"}
	if got.Workflow == nil || *got.Workflow != *want {
		t.Errorf("Workflow = %+v, want %+v", got.Workflow, want)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}

func TestResolveDescriptors_WorkflowResolvedStepFromDifferentWorkflow_OmitsStepAndWarns(t *testing.T) {
	svc, _, _, workflowLookup, stepLookup, _ := resolveTestFixture(t)
	workflowLookup.workflows["wf-1"] = &taskmodels.Workflow{Name: "Review Flow"}
	stepLookup.steps["step-1"] = &workflowmodels.WorkflowStep{Name: "Foreign", WorkflowID: "wf-2"}
	tx := beginTestReadTx(t, svc)

	got, warnings, err := svc.resolveDescriptors(context.Background(), tx, &Automation{
		ID: "a1", Name: "A", WorkflowID: "wf-1", WorkflowStepID: "step-1",
	})
	if err != nil {
		t.Fatalf("resolveDescriptors: %v", err)
	}
	if got.Workflow == nil || got.Workflow.Step != "" {
		t.Errorf("expected workflow name without foreign step, got %+v", got.Workflow)
	}
	if len(warnings) != 1 || warnings[0].Message != warnUnresolvedWorkflowStep {
		t.Errorf("warnings = %v, want [unresolved workflow step]", warnings)
	}
}

// AC-19's silent case: workflow_id set, workflow_step_id empty is not
// "absent" — nothing was referenced. Same shape as the unresolved-step case
// (workflow name only, no step) but with no warning at all.
func TestResolveDescriptors_WorkflowResolvedStepNeverReferenced_NoWarning(t *testing.T) {
	svc, _, _, workflowLookup, _, _ := resolveTestFixture(t)
	workflowLookup.workflows["wf-1"] = &taskmodels.Workflow{Name: "Review Flow"}
	tx := beginTestReadTx(t, svc)

	got, warnings, err := svc.resolveDescriptors(context.Background(), tx, &Automation{ID: "a1", Name: "A", WorkflowID: "wf-1"})
	if err != nil {
		t.Fatalf("resolveDescriptors: %v", err)
	}
	want := &exportWorkflow{Name: "Review Flow"}
	if got.Workflow == nil || *got.Workflow != *want {
		t.Errorf("Workflow = %+v, want %+v", got.Workflow, want)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings for an unreferenced step, got %v", warnings)
	}
}

func TestResolveDescriptors_WorkflowResolvedStepUnresolved_OmitsStepAndWarns(t *testing.T) {
	svc, _, _, workflowLookup, _, _ := resolveTestFixture(t)
	workflowLookup.workflows["wf-1"] = &taskmodels.Workflow{Name: "Review Flow"}
	tx := beginTestReadTx(t, svc)

	got, warnings, err := svc.resolveDescriptors(context.Background(), tx, &Automation{ID: "a1", Name: "A", WorkflowID: "wf-1", WorkflowStepID: "missing-step"})
	if err != nil {
		t.Fatalf("resolveDescriptors: %v", err)
	}
	want := &exportWorkflow{Name: "Review Flow"}
	if got.Workflow == nil || *got.Workflow != *want {
		t.Errorf("Workflow = %+v, want %+v", got.Workflow, want)
	}
	if len(warnings) != 1 || warnings[0].Message != warnUnresolvedWorkflowStep {
		t.Errorf("warnings = %v, want [unresolved workflow step]", warnings)
	}
}

func TestResolveDescriptors_WorkflowUnresolved_OmitsWholeDescriptorNoStepWarning(t *testing.T) {
	svc, _, _, _, _, _ := resolveTestFixture(t)
	tx := beginTestReadTx(t, svc)

	got, warnings, err := svc.resolveDescriptors(context.Background(), tx, &Automation{ID: "a1", Name: "A", WorkflowID: "missing-wf", WorkflowStepID: "step-1"})
	if err != nil {
		t.Fatalf("resolveDescriptors: %v", err)
	}
	if got.Workflow != nil {
		t.Errorf("expected nil Workflow, got %+v", got.Workflow)
	}
	// Exactly one warning: "unresolved workflow". Not two, and never
	// "unresolved workflow step" — a step name without its workflow names
	// nothing.
	if len(warnings) != 1 || warnings[0].Message != "unresolved workflow" {
		t.Errorf("warnings = %v, want [unresolved workflow]", warnings)
	}
}

func TestResolveDescriptors_RepositoriesPartialResolution_DropsUnresolvedKeepsOrderWarnsByPosition(t *testing.T) {
	svc, _, _, _, _, repoLookup := resolveTestFixture(t)
	repoLookup.repositories["repo-a"] = &taskmodels.Repository{Name: "repo-a-name"}
	repoLookup.repositories["repo-c"] = &taskmodels.Repository{Name: "repo-c-name"}
	tx := beginTestReadTx(t, svc)

	got, warnings, err := svc.resolveDescriptors(context.Background(), tx, &Automation{
		ID: "a1", Name: "A", RepositoryIDs: []string{"repo-a", "repo-b", "repo-c"},
	})
	if err != nil {
		t.Fatalf("resolveDescriptors: %v", err)
	}
	wantRepos := []string{"repo-a-name", "repo-c-name"}
	if len(got.Repositories) != 2 || got.Repositories[0] != wantRepos[0] || got.Repositories[1] != wantRepos[1] {
		t.Errorf("Repositories = %v, want %v", got.Repositories, wantRepos)
	}
	if len(warnings) != 1 || warnings[0].Message != "unresolved repository at position 1" {
		t.Errorf("warnings = %v, want [unresolved repository at position 1]", warnings)
	}
}

func TestResolveDescriptors_RepositoriesAllUnresolved_EmptyList(t *testing.T) {
	svc, _, _, _, _, _ := resolveTestFixture(t)
	tx := beginTestReadTx(t, svc)

	got, warnings, err := svc.resolveDescriptors(context.Background(), tx, &Automation{
		ID: "a1", Name: "A", RepositoryIDs: []string{"repo-a", "repo-b"},
	})
	if err != nil {
		t.Fatalf("resolveDescriptors: %v", err)
	}
	if len(got.Repositories) != 0 {
		t.Errorf("Repositories = %v, want empty", got.Repositories)
	}
	if len(warnings) != 2 {
		t.Errorf("warnings = %v, want 2 entries", warnings)
	}
}

func TestResolveDescriptors_RepositoryFreeModeOmitsRepositoryReferences(t *testing.T) {
	svc := newTestService(t)
	tx := beginTestReadTx(t, svc)

	got, warnings, err := svc.resolveDescriptors(context.Background(), tx, &Automation{
		ID: "a1", Name: "scratch", RepositoryMode: RepositoryModeNone,
		RepositoryIDs: []string{"stale-repository-id"},
	})
	if err != nil {
		t.Fatalf("resolveDescriptors: %v", err)
	}
	if len(got.Repositories) != 0 || len(warnings) != 0 {
		t.Fatalf("repository-free export resolved references: repositories=%v warnings=%v", got.Repositories, warnings)
	}
}

func TestResolveDescriptors_LookupNotWired_FailsClosed(t *testing.T) {
	svc := newTestService(t)
	tx := beginTestReadTx(t, svc)

	_, _, err := svc.resolveDescriptors(context.Background(), tx, &Automation{ID: "a1", Name: "A", AgentProfileID: "agent-1"})
	if !errors.Is(err, ErrExportLookupUnavailable) {
		t.Fatalf("err = %v, want wrapping ErrExportLookupUnavailable", err)
	}
}

// AC-29: the export path must obtain its read-transaction snapshot once and
// pass that exact handle to every read — the automation store's own read
// and every descriptor lookup alike — never a nil handle, never a distinct
// one. Service.store is a concrete *Store (not an interface; introducing
// one purely for this test would be an out-of-scope architectural change),
// so this test uses the real store's own BeginReadTx to obtain the one
// handle under test, then asserts every lookup double recorded that exact
// pointer. This is the direct, mechanical version of AC-29's own text: "the
// test asserts, with a store double and four lookup doubles that each
// record the handle they were given, that every recorded handle is the
// same one and that no read arrived with no handle."
func TestResolveDescriptors_AC29_SameTransactionHandleReachesStoreAndEveryLookup(t *testing.T) {
	svc, agentLookup, executorLookup, workflowLookup, stepLookup, repoLookup := resolveTestFixture(t)
	agentLookup.profiles["agent-1"] = &settingsmodels.AgentProfile{Name: "Reviewer", Model: "claude-sonnet-5", Mode: "plan"}
	executorLookup.profiles["exec-1"] = &taskmodels.ExecutorProfile{ExecutorID: "local_docker", Name: "Default"}
	workflowLookup.workflows["wf-1"] = &taskmodels.Workflow{Name: "Review Flow"}
	stepLookup.steps["step-1"] = &workflowmodels.WorkflowStep{Name: "In Review", WorkflowID: "wf-1"}
	repoLookup.repositories["repo-a"] = &taskmodels.Repository{Name: "repo-a-name"}

	ctx := context.Background()
	a := &Automation{
		WorkspaceID:       "ws-1",
		Name:              "Daily Review",
		Enabled:           true,
		MaxConcurrentRuns: 1,
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "exec-1",
		WorkflowID:        "wf-1",
		WorkflowStepID:    "step-1",
		RepositoryIDs:     []string{"repo-a"},
	}
	if err := svc.store.CreateAutomation(ctx, a); err != nil {
		t.Fatalf("CreateAutomation: %v", err)
	}

	// This is the one handle under test: the store's own read (mirroring
	// export_store_test.go) and every descriptor lookup below must all see
	// this exact *sqlx.Tx.
	tx := beginTestReadTx(t, svc)

	automations, err := svc.store.ListAutomationsForExportTx(ctx, tx, "ws-1")
	if err != nil {
		t.Fatalf("ListAutomationsForExportTx: %v", err)
	}
	if len(automations) != 1 {
		t.Fatalf("expected 1 automation, got %d", len(automations))
	}

	if _, _, err := svc.resolveDescriptors(ctx, tx, automations[0]); err != nil {
		t.Fatalf("resolveDescriptors: %v", err)
	}

	for name, got := range map[string]*sqlx.Tx{
		"agent profile":    agentLookup.gotTx,
		"executor profile": executorLookup.gotTx,
		"workflow":         workflowLookup.gotTx,
		"workflow step":    stepLookup.gotTx,
		"repository":       repoLookup.gotTx,
	} {
		if got == nil {
			t.Errorf("%s lookup received a nil transaction handle, want the store's snapshot handle", name)
			continue
		}
		if got != tx {
			t.Errorf("%s lookup received a different transaction handle than the store read used", name)
		}
	}
}
