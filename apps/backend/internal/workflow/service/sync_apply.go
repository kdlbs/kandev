package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"go.uber.org/zap"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/models"
)

// SyncWorkflowOps is the extra task-domain surface the workflow-sync applier
// needs beyond WorkflowProvider. Satisfied by the task service.
type SyncWorkflowOps interface {
	DeleteWorkflow(ctx context.Context, id string) error
	CountTasksByWorkflow(ctx context.Context, workflowID string) (int, error)
	CountTasksByWorkflowStep(ctx context.Context, stepID string) (int, error)
	SetWorkflowSource(ctx context.Context, id, source, sourcePath string) error
}

// SetSyncWorkflowOps wires the task-domain operations used when applying a
// workflow sync (set during service init to break circular deps).
func (s *Service) SetSyncWorkflowOps(ops SyncWorkflowOps) {
	s.syncOps = ops
}

// SyncFileExport is one parsed, validated workflow definition file from a
// sync source (e.g. a GitHub repo). Path is the repo-relative file path. A
// nil Export marks a file that exists but could not be parsed: workflows
// previously synced from it are left untouched instead of being treated as
// removed.
type SyncFileExport struct {
	Path   string
	Export *models.WorkflowExport
}

// SyncApplyResult reports what applying a workflow sync changed. Warnings
// describe definitions that could not be applied and need user attention.
type SyncApplyResult struct {
	Created  []string `json:"created"`
	Updated  []string `json:"updated"`
	Deleted  []string `json:"deleted"`
	Warnings []string `json:"warnings"`
}

func syncKey(sourcePath, name string) string {
	return sourcePath + "\x00" + name
}

// ApplySyncedWorkflows reconciles the workspace's GitHub-sourced workflows
// with the given definition files. Workflows are matched by (source file
// path, workflow name): matches are updated in place (step identity is
// preserved by step name so tasks keep their step assignment), new
// definitions are created, and previously-synced workflows that disappeared
// from the source are deleted only when they no longer hold tasks. Manual
// workflows are never touched. Every export must already be validated.
func (s *Service) ApplySyncedWorkflows(ctx context.Context, workspaceID string, files []SyncFileExport) (*SyncApplyResult, error) {
	if s.workflowProvider == nil || s.syncOps == nil {
		return nil, fmt.Errorf("workflow sync is not wired")
	}
	existing, err := s.workflowProvider.ListWorkflows(ctx, workspaceID, true)
	if err != nil {
		return nil, fmt.Errorf("failed to list existing workflows: %w", err)
	}
	synced := make(map[string]*taskmodels.Workflow)
	for _, wf := range existing {
		if wf.Source == taskmodels.WorkflowSourceGitHub {
			synced[syncKey(wf.SourcePath, wf.Name)] = wf
		}
	}

	result := &SyncApplyResult{}
	desired := make(map[string]bool)
	frozenPaths := make(map[string]bool)
	for _, f := range files {
		if f.Export == nil {
			frozenPaths[f.Path] = true
			continue
		}
		for _, pw := range f.Export.Workflows {
			key := syncKey(f.Path, pw.Name)
			if desired[key] {
				result.Warnings = append(result.Warnings, fmt.Sprintf("%s: duplicate workflow %q ignored", f.Path, pw.Name))
				continue
			}
			desired[key] = true
			if wf, ok := synced[key]; ok {
				s.updateSyncedWorkflow(ctx, wf, pw, result)
			} else {
				s.createSyncedWorkflow(ctx, workspaceID, pw, f.Path, result)
			}
		}
	}
	s.deleteVanishedWorkflows(ctx, synced, desired, frozenPaths, result)

	sort.Strings(result.Created)
	sort.Strings(result.Updated)
	sort.Strings(result.Deleted)
	s.logger.Info("applied workflow sync",
		zap.String("workspace_id", workspaceID),
		zap.Int("created", len(result.Created)),
		zap.Int("updated", len(result.Updated)),
		zap.Int("deleted", len(result.Deleted)),
		zap.Int("warnings", len(result.Warnings)))
	return result, nil
}

func (s *Service) createSyncedWorkflow(ctx context.Context, workspaceID string, pw models.WorkflowPortable, path string, result *SyncApplyResult) {
	wf, err := s.importSingleWorkflow(ctx, workspaceID, pw)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("%s: failed to create workflow %q: %v", path, pw.Name, err))
		return
	}
	if err := s.syncOps.SetWorkflowSource(ctx, wf.ID, taskmodels.WorkflowSourceGitHub, path); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("%s: failed to mark workflow %q as synced: %v", path, pw.Name, err))
		return
	}
	result.Created = append(result.Created, pw.Name)
}

// updateSyncedWorkflow reconciles an existing synced workflow with its
// definition. Steps are matched by name so their IDs — referenced by
// tasks.workflow_step_id — survive the update, and steps that already match
// the definition are left untouched so a no-drift sync writes (and
// broadcasts) nothing. The workflow is skipped with a warning when steps
// can't be matched unambiguously or when a removed step still holds tasks.
func (s *Service) updateSyncedWorkflow(ctx context.Context, wf *taskmodels.Workflow, pw models.WorkflowPortable, result *SyncApplyResult) {
	steps, err := s.repo.ListStepsByWorkflow(ctx, wf.ID)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("workflow %q: failed to list steps: %v", wf.Name, err))
		return
	}
	existingByName, toDelete, warning := s.planStepChanges(ctx, wf, pw, steps)
	if warning != "" {
		result.Warnings = append(result.Warnings, warning)
		return
	}

	// Position → step ID: matched steps keep their ID, new steps get one.
	posToID := make(map[int]string, len(pw.Steps))
	for _, sp := range pw.Steps {
		if st, ok := existingByName[sp.Name]; ok {
			posToID[sp.Position] = st.ID
		} else {
			posToID[sp.Position] = uuid.New().String()
		}
	}

	changed, err := s.applyWorkflowFields(ctx, wf, pw)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("workflow %q: failed to update: %v", wf.Name, err))
		return
	}
	for _, sp := range pw.Steps {
		step := s.stepFromPortable(wf.ID, sp, posToID)
		if existing, ok := existingByName[sp.Name]; ok {
			step.CreatedAt = existing.CreatedAt
			step.StageType = existing.StageType // not carried by the portable format
			if stepMatchesDefinition(existing, step) {
				continue
			}
			err = s.repo.UpdateStep(ctx, step)
		} else {
			err = s.repo.CreateStep(ctx, step)
		}
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("workflow %q: failed to apply step %q: %v", wf.Name, sp.Name, err))
			return
		}
		changed = true
	}
	for _, st := range toDelete {
		if err := s.DeleteStep(ctx, st.ID); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("workflow %q: failed to remove step %q: %v", wf.Name, st.Name, err))
			return
		}
		changed = true
	}
	if changed {
		result.Updated = append(result.Updated, wf.Name)
	}
}

// stepMatchesDefinition reports whether an existing step already equals its
// desired definition (timestamps and stage type aside — stage type is copied
// from the existing step). Events are compared via their JSON encoding to
// normalize YAML/JSON numeric type differences in action configs.
func stepMatchesDefinition(existing, desired *models.WorkflowStep) bool {
	return existing.Name == desired.Name &&
		existing.Position == desired.Position &&
		existing.Color == desired.Color &&
		existing.Prompt == desired.Prompt &&
		existing.AllowManualMove == desired.AllowManualMove &&
		existing.IsStartStep == desired.IsStartStep &&
		existing.ShowInCommandPanel == desired.ShowInCommandPanel &&
		existing.AutoArchiveAfterHours == desired.AutoArchiveAfterHours &&
		existing.AgentProfileID == desired.AgentProfileID &&
		existing.WIPLimit == desired.WIPLimit &&
		existing.PullFromStepID == desired.PullFromStepID &&
		existing.AutoAdvanceRequiresSignal == desired.AutoAdvanceRequiresSignal &&
		eventsEqual(existing.Events, desired.Events)
}

func eventsEqual(a, b models.StepEvents) bool {
	aj, errA := json.Marshal(a)
	bj, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(aj, bj)
}

// planStepChanges matches existing steps to the portable definition by name
// and decides which steps would be removed. It returns a non-empty warning
// (and the update must be skipped) when names are ambiguous or a removed step
// still holds tasks.
func (s *Service) planStepChanges(ctx context.Context, wf *taskmodels.Workflow, pw models.WorkflowPortable, steps []*models.WorkflowStep) (map[string]*models.WorkflowStep, []*models.WorkflowStep, string) {
	existingByName := make(map[string]*models.WorkflowStep, len(steps))
	for _, st := range steps {
		if _, dup := existingByName[st.Name]; dup {
			return nil, nil, fmt.Sprintf("workflow %q has multiple steps named %q; rename them, then sync again", wf.Name, st.Name)
		}
		existingByName[st.Name] = st
	}
	desiredNames := make(map[string]bool, len(pw.Steps))
	for _, sp := range pw.Steps {
		if desiredNames[sp.Name] {
			return nil, nil, fmt.Sprintf("workflow %q defines multiple steps named %q; step names must be unique to sync", wf.Name, sp.Name)
		}
		desiredNames[sp.Name] = true
	}
	var toDelete []*models.WorkflowStep
	for _, st := range steps {
		if desiredNames[st.Name] {
			continue
		}
		count, err := s.syncOps.CountTasksByWorkflowStep(ctx, st.ID)
		if err != nil {
			return nil, nil, fmt.Sprintf("workflow %q: failed to count tasks in step %q: %v", wf.Name, st.Name, err)
		}
		if count > 0 {
			return nil, nil, fmt.Sprintf("workflow %q not updated: step %q was removed from the definition but still has %d task(s); move or archive them, then sync again", wf.Name, st.Name, count)
		}
		toDelete = append(toDelete, st)
	}
	return existingByName, toDelete, ""
}

func (s *Service) applyWorkflowFields(ctx context.Context, wf *taskmodels.Workflow, pw models.WorkflowPortable) (bool, error) {
	profileID := ""
	if pw.AgentProfile != nil && s.matchProfile != nil {
		profileID = s.matchProfile(pw.AgentProfile.AgentName, pw.AgentProfile.Model, pw.AgentProfile.Mode)
	}
	if wf.Description == pw.Description && wf.AgentProfileID == profileID {
		return false, nil
	}
	wf.Description = pw.Description
	wf.AgentProfileID = profileID
	return true, s.workflowProvider.UpdateWorkflow(ctx, wf)
}

// deleteVanishedWorkflows removes previously-synced workflows whose
// definition disappeared from the source, unless they still hold tasks or
// come from a file that failed to parse this round.
func (s *Service) deleteVanishedWorkflows(ctx context.Context, synced map[string]*taskmodels.Workflow, desired, frozenPaths map[string]bool, result *SyncApplyResult) {
	for key, wf := range synced {
		if desired[key] || frozenPaths[wf.SourcePath] {
			continue
		}
		count, err := s.syncOps.CountTasksByWorkflow(ctx, wf.ID)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("workflow %q: failed to count tasks: %v", wf.Name, err))
			continue
		}
		if count > 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf("workflow %q was removed from the sync source but still has %d task(s); move or archive them, then sync again", wf.Name, count))
			continue
		}
		if err := s.syncOps.DeleteWorkflow(ctx, wf.ID); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("workflow %q: failed to delete: %v", wf.Name, err))
			continue
		}
		result.Deleted = append(result.Deleted, wf.Name)
	}
}
