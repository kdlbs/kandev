package automation

import (
	"context"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// ErrExportLookupUnavailable is returned when an automation references an
// agent profile, executor profile, workflow, workflow step, or repository but
// the matching Export*Lookup was never wired via the Service's setters.
// Fails closed rather than treating the reference as absent: AC-19's "absent"
// path is for a reference that was actually looked up and not found, and an
// unwired lookup has looked up nothing, so reporting "absent" here would
// silently drop real references the moment a startup wire is missing.
var ErrExportLookupUnavailable = errors.New("automation: export descriptor lookup is not available")

// warnUnresolvedWorkflowStep is emitted when a workflow resolves but its
// referenced step does not (AC-19's non-silent case).
const warnUnresolvedWorkflowStep = "unresolved workflow step"

// exportWarning is one unformatted warning captured during export: an
// automation name/ID pair plus a message (already carrying any AC-42-escaped
// interpolation, such as a trigger type, that a trigger-scoped message
// embeds) and a DedupKey scoping AC-42's de-duplication rule — the
// automation ID for an automation-scoped message, the trigger ID for a
// trigger-scoped one. Ordering (AC-21) and the final "<name>: <message>"
// rendering (AC-42) are buildWarningsList's job, not this one's.
type exportWarning struct {
	AutomationName string
	AutomationID   string
	DedupKey       string
	Message        string
}

// resolvedDescriptors is one automation's resolved agent profile, executor
// profile, workflow/step, and repository names — everything resolveDescriptors
// produces from an Automation's *ID fields. A nil/empty field means the
// reference was never made or did not resolve; see resolveDescriptors.
type resolvedDescriptors struct {
	AgentProfile    *exportAgentProfile
	ExecutorProfile *exportExecutorProfile
	Workflow        *exportWorkflow
	Repositories    []string
}

// resolveDescriptors resolves a.AgentProfileID, a.ExecutorProfileID,
// a.WorkflowID/a.WorkflowStepID, and a.RepositoryIDs into their portable
// descriptors within tx, applying AC-19's partial-resolution rules. A
// resolution failure — as opposed to an absent reference — aborts the whole
// export: the caller must not fall back to a partial document or a warning
// for an error returned here (AC-19, AC-45).
func (s *Service) resolveDescriptors(ctx context.Context, tx *sqlx.Tx, a *Automation) (resolvedDescriptors, []exportWarning, error) {
	agentProfile, agentWarning, err := s.resolveAgentProfile(ctx, tx, a)
	if err != nil {
		return resolvedDescriptors{}, nil, err
	}
	executorProfile, executorWarning, err := s.resolveExecutorProfile(ctx, tx, a)
	if err != nil {
		return resolvedDescriptors{}, nil, err
	}
	workflow, workflowWarning, err := s.resolveWorkflow(ctx, tx, a)
	if err != nil {
		return resolvedDescriptors{}, nil, err
	}
	repositories, repoWarnings, err := s.resolveRepositories(ctx, tx, a)
	if err != nil {
		return resolvedDescriptors{}, nil, err
	}

	var warnings []exportWarning
	addWarning := func(msg string) {
		if msg == "" {
			return
		}
		warnings = append(warnings, exportWarning{AutomationName: a.Name, AutomationID: a.ID, DedupKey: a.ID, Message: msg})
	}
	addWarning(agentWarning)
	addWarning(executorWarning)
	addWarning(workflowWarning)
	for _, msg := range repoWarnings {
		addWarning(msg)
	}

	return resolvedDescriptors{
		AgentProfile:    agentProfile,
		ExecutorProfile: executorProfile,
		Workflow:        workflow,
		Repositories:    repositories,
	}, warnings, nil
}

// resolveAgentProfile resolves a.AgentProfileID. An empty ID references
// nothing, so it returns no descriptor and no warning.
//
// validateAgentProfileID (service.go) only checks that the profile row
// exists, not that it belongs to the automation's own workspace — unlike
// RepositoryLookup, which fails closed on a cross-workspace reference at
// create/update time. A workspace-scoped profile (non-empty WorkspaceID)
// that does not match a.WorkspaceID is therefore treated as unresolved here,
// the same as a missing row: the export must not leak another workspace's
// profile name/model/mode just because a stale or crafted ID happens to
// resolve. A global profile (WorkspaceID == "") is unaffected.
func (s *Service) resolveAgentProfile(ctx context.Context, tx *sqlx.Tx, a *Automation) (*exportAgentProfile, string, error) {
	if a.AgentProfileID == "" {
		return nil, "", nil
	}
	if s.exportAgentProfileLookup == nil {
		return nil, "", fmt.Errorf("%w: agent profile", ErrExportLookupUnavailable)
	}
	profile, found, err := s.exportAgentProfileLookup.GetAgentProfileTx(ctx, tx, a.AgentProfileID)
	if err != nil {
		return nil, "", fmt.Errorf("resolve agent profile %q: %w", a.AgentProfileID, err)
	}
	if !found || (profile.WorkspaceID != "" && profile.WorkspaceID != a.WorkspaceID) {
		return nil, "unresolved agent profile", nil
	}
	return &exportAgentProfile{AgentName: profile.AgentDisplayName, Model: profile.Model, Mode: profile.Mode}, "", nil
}

// resolveExecutorProfile resolves a.ExecutorProfileID. An empty ID
// references nothing, so it returns no descriptor and no warning.
func (s *Service) resolveExecutorProfile(ctx context.Context, tx *sqlx.Tx, a *Automation) (*exportExecutorProfile, string, error) {
	if a.ExecutorProfileID == "" {
		return nil, "", nil
	}
	if s.exportExecutorProfileLookup == nil {
		return nil, "", fmt.Errorf("%w: executor profile", ErrExportLookupUnavailable)
	}
	profile, found, err := s.exportExecutorProfileLookup.GetExecutorProfileTx(ctx, tx, a.ExecutorProfileID)
	if err != nil {
		return nil, "", fmt.Errorf("resolve executor profile %q: %w", a.ExecutorProfileID, err)
	}
	if !found {
		return nil, "unresolved executor profile", nil
	}
	return &exportExecutorProfile{Executor: profile.ExecutorID, Name: profile.Name}, "", nil
}

// resolveWorkflow resolves a.WorkflowID and, if present, a.WorkflowStepID.
// An empty WorkflowID references nothing. A resolved workflow with an empty
// WorkflowStepID is AC-19's silent case: the step was never referenced, so
// the workflow descriptor carries only name and no warning is produced.
func (s *Service) resolveWorkflow(ctx context.Context, tx *sqlx.Tx, a *Automation) (*exportWorkflow, string, error) {
	if a.WorkflowID == "" {
		return nil, "", nil
	}
	if s.exportWorkflowLookup == nil {
		return nil, "", fmt.Errorf("%w: workflow", ErrExportLookupUnavailable)
	}
	wf, found, err := s.exportWorkflowLookup.GetWorkflowTx(ctx, tx, a.WorkflowID)
	if err != nil {
		return nil, "", fmt.Errorf("resolve workflow %q: %w", a.WorkflowID, err)
	}
	if !found {
		// The whole descriptor is omitted, including any step: a step name
		// without its workflow names nothing, and this AC's "step does not
		// resolve" warning covers only a workflow that did resolve.
		return nil, "unresolved workflow", nil
	}
	result := &exportWorkflow{Name: wf.Name}
	if a.WorkflowStepID == "" {
		return result, "", nil
	}
	if s.exportWorkflowStepLookup == nil {
		return nil, "", fmt.Errorf("%w: workflow step", ErrExportLookupUnavailable)
	}
	step, stepFound, err := s.exportWorkflowStepLookup.GetStepTx(ctx, tx, a.WorkflowStepID)
	if err != nil {
		return nil, "", fmt.Errorf("resolve workflow step %q: %w", a.WorkflowStepID, err)
	}
	if !stepFound || step == nil || step.WorkflowID != a.WorkflowID {
		return result, warnUnresolvedWorkflowStep, nil
	}
	result.Step = step.Name
	return result, "", nil
}

// resolveRepositories resolves a.RepositoryIDs, dropping unresolved members
// and keeping resolved ones in their stored (AC-8) order. Each unresolved
// member warns by its 0-based position, since the export carries no UUID to
// name it by (AC-18).
func (s *Service) resolveRepositories(ctx context.Context, tx *sqlx.Tx, a *Automation) ([]string, []string, error) {
	if a.RepositoryMode == RepositoryModeNone || len(a.RepositoryIDs) == 0 {
		return nil, nil, nil
	}
	if s.exportRepositoryLookup == nil {
		return nil, nil, fmt.Errorf("%w: repository", ErrExportLookupUnavailable)
	}
	var names []string
	var warnings []string
	for position, id := range a.RepositoryIDs {
		repo, found, err := s.exportRepositoryLookup.GetRepositoryTx(ctx, tx, id)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve repository %q: %w", id, err)
		}
		if !found {
			warnings = append(warnings, fmt.Sprintf("unresolved repository at position %d", position))
			continue
		}
		names = append(names, repo.Name)
	}
	return names, warnings, nil
}
