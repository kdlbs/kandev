package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/models"
	"go.uber.org/zap"
)

const (
	ImportProfileReasonMissingSelection = "missing_selection"
	ImportProfileReasonUnavailable      = "unavailable_profile"
	ImportProfileReasonChanged          = "changed_profile"
)

// ImportProfileCatalog supplies the profiles that the browser import flow may
// use. The agent settings domain owns eligibility; workflow import only reads
// this narrow catalog and never reimplements profile health or discovery.
type ImportProfileCatalog interface {
	ListEligibleProfiles(ctx context.Context) ([]ImportProfileCandidate, error)
	GetEligibleProfile(ctx context.Context, id string) (*ImportProfileCandidate, error)
}

type workflowImportDeletionProvider interface {
	DeleteWorkflow(ctx context.Context, id string) error
}

// ImportProfileCandidate is an enabled, global agent profile that can be used
// as a replacement for a portable profile descriptor.
type ImportProfileCandidate struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	AgentName string    `json:"agent_name"`
	Model     string    `json:"model"`
	Mode      string    `json:"mode"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ImportProfileMatch struct {
	ID        string    `json:"id"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ImportProfileStep struct {
	WorkflowIndex    int                         `json:"workflow_index"`
	WorkflowName     string                      `json:"workflow_name"`
	StepPosition     int                         `json:"step_position"`
	StepName         string                      `json:"step_name"`
	RequestedProfile models.AgentProfilePortable `json:"requested_profile"`
	MatchedProfile   *ImportProfileMatch         `json:"matched_profile,omitempty"`
}

type ImportProfilePreview struct {
	Skipped  []string                 `json:"skipped"`
	Profiles []ImportProfileCandidate `json:"profiles"`
	Steps    []ImportProfileStep      `json:"steps"`
}

type ImportProfileBinding struct {
	WorkflowIndex    int                          `json:"workflow_index"`
	StepPosition     int                          `json:"step_position"`
	RequestedProfile *models.AgentProfilePortable `json:"requested_profile"`
	ProfileID        string                       `json:"profile_id"`
	ProfileUpdatedAt time.Time                    `json:"profile_updated_at"`
}

type ImportProfileConflict struct {
	WorkflowIndex int    `json:"workflow_index"`
	StepPosition  int    `json:"step_position"`
	WorkflowName  string `json:"workflow_name"`
	StepName      string `json:"step_name"`
	Reason        string `json:"reason"`
}

// ImportProfileResolutionError means the document is valid but the user must
// review one or more direct step profile bindings before persistence.
type ImportProfileResolutionError struct {
	Conflicts []ImportProfileConflict
}

func (e *ImportProfileResolutionError) Error() string {
	return "workflow import requires agent profile selections"
}

// InvalidImportProfileBindingsError means the browser submitted a binding
// envelope that does not describe the supplied portable document.
type InvalidImportProfileBindingsError struct {
	Reason string
}

func (e *InvalidImportProfileBindingsError) Error() string {
	if e.Reason == "" {
		return "invalid workflow import profile bindings"
	}
	return "invalid workflow import profile bindings: " + e.Reason
}

// ErrImportProfileNotFound is returned by a catalog when a selected profile
// is no longer eligible or visible to the importing user.
var (
	ErrImportProfileNotFound           = errors.New("import profile not found")
	ErrImportProfileCatalogUnavailable = errors.New("import profile catalog is unavailable")
)

func samePortableProfile(a, b *models.AgentProfilePortable) bool {
	return a != nil && b != nil && a.AgentName == b.AgentName && a.Model == b.Model && a.Mode == b.Mode
}

func importProfileKey(workflowIndex, stepPosition int) string {
	return fmt.Sprintf("%d:%d", workflowIndex, stepPosition)
}

func sortedImportProfileCandidates(profiles []ImportProfileCandidate) []ImportProfileCandidate {
	sorted := append([]ImportProfileCandidate{}, profiles...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left, right := sorted[i], sorted[j]
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		if left.AgentName != right.AgentName {
			return left.AgentName < right.AgentName
		}
		if left.Model != right.Model {
			return left.Model < right.Model
		}
		if left.Mode != right.Mode {
			return left.Mode < right.Mode
		}
		return left.ID < right.ID
	})
	return sorted
}

func validateImportExport(export *models.WorkflowExport) error {
	if export == nil {
		return fmt.Errorf("invalid export data: export is required")
	}
	if err := export.NormalizeCompletionPolicy(); err != nil {
		return fmt.Errorf("invalid export data: %w", err)
	}
	if err := export.Validate(); err != nil {
		return fmt.Errorf("invalid export data: %w", err)
	}
	return nil
}

func collectImportProfileSteps(
	export *models.WorkflowExport,
	existingNames map[string]bool,
) ([]string, []ImportProfileStep) {
	skipped := make([]string, 0)
	steps := make([]ImportProfileStep, 0)
	for workflowIndex, workflow := range export.Workflows {
		if existingNames[workflow.Name] {
			skipped = append(skipped, workflow.Name)
			continue
		}
		for _, step := range workflow.Steps {
			if step.AgentProfile == nil {
				continue
			}
			steps = append(steps, ImportProfileStep{
				WorkflowIndex:    workflowIndex,
				WorkflowName:     workflow.Name,
				StepPosition:     step.Position,
				StepName:         step.Name,
				RequestedProfile: *step.AgentProfile,
			})
		}
	}
	return skipped, steps
}

func (s *Service) importProfileCandidates(ctx context.Context) ([]ImportProfileCandidate, error) {
	if s.importProfileCatalog == nil {
		return nil, ErrImportProfileCatalogUnavailable
	}
	profiles, err := s.importProfileCatalog.ListEligibleProfiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: list import profile candidates: %w", ErrImportProfileCatalogUnavailable, err)
	}
	return sortedImportProfileCandidates(profiles), nil
}

func (s *Service) matchImportProfileSteps(preview *ImportProfilePreview) {
	if s.matchProfile == nil {
		return
	}
	byID := make(map[string]ImportProfileCandidate, len(preview.Profiles))
	for _, profile := range preview.Profiles {
		byID[profile.ID] = profile
	}
	for index := range preview.Steps {
		requested := preview.Steps[index].RequestedProfile
		id := s.matchProfile(requested.AgentName, requested.Model, requested.Mode, "")
		profile, ok := byID[id]
		if !ok {
			continue
		}
		preview.Steps[index].MatchedProfile = &ImportProfileMatch{
			ID:        profile.ID,
			UpdatedAt: profile.UpdatedAt,
		}
	}
}

func (s *Service) SetImportProfileCatalog(catalog ImportProfileCatalog) {
	s.importProfileCatalog = catalog
}

func (s *Service) importExistingWorkflowNames(ctx context.Context, workspaceID string) (map[string]bool, error) {
	existing, err := s.workflowProvider.ListWorkflows(ctx, workspaceID, true)
	if err != nil {
		return nil, fmt.Errorf("failed to list existing workflows: %w", err)
	}
	names := make(map[string]bool, len(existing))
	for _, workflow := range existing {
		names[workflow.Name] = true
	}
	return names, nil
}

type preparedImportedWorkflow struct {
	portable          models.WorkflowPortable
	steps             []*models.WorkflowStep
	workflowProfileID string
}

func (s *Service) prepareImportedWorkflow(
	ctx context.Context,
	pw models.WorkflowPortable,
	directProfileIDs map[int]string,
) (preparedImportedWorkflow, error) {
	posToID := make(map[int]string, len(pw.Steps))
	for _, sp := range pw.Steps {
		posToID[sp.Position] = uuid.New().String()
	}

	steps := make([]*models.WorkflowStep, 0, len(pw.Steps))
	for _, sp := range pw.Steps {
		step := s.stepFromPortableWithMatcherOptions(
			"pending-workflow", sp, posToID, s.matchProfile, "", false,
		)
		if profileID := directProfileIDs[sp.Position]; profileID != "" {
			step.AgentProfileID = profileID
		}
		if err := models.ValidateWorkflowStep(step); err != nil {
			return preparedImportedWorkflow{}, fmt.Errorf("validate step %q: %w", sp.Name, err)
		}
		if err := s.validateImportedStepReferences(ctx, step, sp.Name); err != nil {
			return preparedImportedWorkflow{}, err
		}
		steps = append(steps, step)
	}
	if err := validateWorkflowSessionTargets(steps); err != nil {
		return preparedImportedWorkflow{}, fmt.Errorf("validate workflow session targets: %w", err)
	}

	workflowProfileID := ""
	if pw.AgentProfile != nil && s.matchProfile != nil {
		workflowProfileID = s.matchProfile(
			pw.AgentProfile.AgentName,
			pw.AgentProfile.Model,
			pw.AgentProfile.Mode,
			"",
		)
	}
	return preparedImportedWorkflow{
		portable:          pw,
		steps:             steps,
		workflowProfileID: workflowProfileID,
	}, nil
}

func (s *Service) persistImportedWorkflow(
	ctx context.Context,
	workspaceID string,
	prepared preparedImportedWorkflow,
) (*taskmodels.Workflow, error) {
	pw := prepared.portable
	wf, err := s.workflowProvider.CreateWorkflow(ctx, workspaceID, pw.Name, pw.Description)
	if err != nil {
		return nil, fmt.Errorf("create workflow: %w", err)
	}
	createdSteps := make([]*models.WorkflowStep, 0, len(prepared.steps))
	rollback := func(cause error) (*taskmodels.Workflow, error) {
		if cleanupErr := s.rollbackImportedWorkflow(ctx, wf, createdSteps); cleanupErr != nil {
			return nil, errors.Join(cause, fmt.Errorf("cleanup imported workflow: %w", cleanupErr))
		}
		return nil, cause
	}

	needsUpdate := false
	if prepared.workflowProfileID != "" {
		wf.AgentProfileID = prepared.workflowProfileID
		needsUpdate = true
	}
	if pw.Prompt != "" {
		wf.Prompt = pw.Prompt
		needsUpdate = true
	}
	if needsUpdate {
		if err := s.workflowProvider.UpdateWorkflow(ctx, wf); err != nil {
			return rollback(fmt.Errorf("set workflow fields: %w", err))
		}
	}
	for _, step := range prepared.steps {
		step.WorkflowID = wf.ID
		if err := s.repo.CreateStep(ctx, step); err != nil {
			return rollback(fmt.Errorf("create step %q: %w", step.Name, err))
		}
		createdSteps = append(createdSteps, step)
	}
	return wf, nil
}

func (s *Service) rollbackImportedWorkflow(
	ctx context.Context,
	wf *taskmodels.Workflow,
	createdSteps []*models.WorkflowStep,
) error {
	var cleanupErr error
	for index := len(createdSteps) - 1; index >= 0; index-- {
		if err := s.repo.DeleteStep(ctx, createdSteps[index].ID); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("delete step %q: %w", createdSteps[index].Name, err))
		}
	}
	deleter, ok := s.workflowProvider.(workflowImportDeletionProvider)
	if !ok {
		return errors.Join(cleanupErr, errors.New("workflow provider cannot delete imported workflows"))
	}
	if err := deleter.DeleteWorkflow(ctx, wf.ID); err != nil {
		cleanupErr = errors.Join(cleanupErr, fmt.Errorf("delete workflow %q: %w", wf.Name, err))
	}
	return cleanupErr
}

func (s *Service) importProfilePreviewData(
	ctx context.Context,
	export *models.WorkflowExport,
	workspaceID string,
) (*ImportProfilePreview, error) {
	if err := validateImportExport(export); err != nil {
		return nil, err
	}
	existingNames, err := s.importExistingWorkflowNames(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	skipped, steps := collectImportProfileSteps(export, existingNames)
	preview := &ImportProfilePreview{
		Skipped:  skipped,
		Profiles: make([]ImportProfileCandidate, 0),
		Steps:    steps,
	}
	if len(preview.Steps) == 0 {
		return preview, nil
	}
	profiles, err := s.importProfileCandidates(ctx)
	if err != nil {
		return nil, err
	}
	preview.Profiles = profiles
	s.matchImportProfileSteps(preview)
	return preview, nil
}

func (s *Service) PreviewImportWorkflows(
	ctx context.Context,
	workspaceID string,
	export *models.WorkflowExport,
) (*ImportProfilePreview, error) {
	if err := s.AuthorizeWorkspace(ctx, workspaceID); err != nil {
		return nil, err
	}
	return s.importProfilePreviewData(ctx, export, workspaceID)
}

func expectedImportProfileSteps(export *models.WorkflowExport) map[string]models.StepPortable {
	expected := make(map[string]models.StepPortable)
	for workflowIndex, workflow := range export.Workflows {
		for _, step := range workflow.Steps {
			if step.AgentProfile == nil {
				continue
			}
			expected[importProfileKey(workflowIndex, step.Position)] = step
		}
	}
	return expected
}

func validateImportProfileBindings(
	bindings []ImportProfileBinding,
	expected map[string]models.StepPortable,
) (map[string]ImportProfileBinding, error) {
	byKey := make(map[string]ImportProfileBinding, len(bindings))
	for _, binding := range bindings {
		key := importProfileKey(binding.WorkflowIndex, binding.StepPosition)
		expectedStep, ok := expected[key]
		if !ok {
			return nil, &InvalidImportProfileBindingsError{Reason: fmt.Sprintf("unknown step %s", key)}
		}
		if _, duplicate := byKey[key]; duplicate {
			return nil, &InvalidImportProfileBindingsError{Reason: fmt.Sprintf("duplicate step %s", key)}
		}
		if binding.RequestedProfile == nil || !samePortableProfile(binding.RequestedProfile, expectedStep.AgentProfile) {
			return nil, &InvalidImportProfileBindingsError{Reason: fmt.Sprintf("profile descriptor does not match step %s", key)}
		}
		byKey[key] = binding
	}
	return byKey, nil
}

func importProfileConflict(
	workflowIndex, stepPosition int,
	workflowName, stepName, reason string,
) ImportProfileConflict {
	return ImportProfileConflict{
		WorkflowIndex: workflowIndex,
		StepPosition:  stepPosition,
		WorkflowName:  workflowName,
		StepName:      stepName,
		Reason:        reason,
	}
}

func (s *Service) resolveSelectedImportProfile(
	ctx context.Context,
	binding ImportProfileBinding,
	key string,
) (*ImportProfileCandidate, string, error) {
	if binding.ProfileUpdatedAt.IsZero() {
		return nil, "", &InvalidImportProfileBindingsError{Reason: fmt.Sprintf("missing profile revision for step %s", key)}
	}
	if s.importProfileCatalog == nil {
		return nil, "", ErrImportProfileCatalogUnavailable
	}
	profile, err := s.importProfileCatalog.GetEligibleProfile(ctx, binding.ProfileID)
	if err != nil || profile == nil {
		if err != nil && !errors.Is(err, ErrImportProfileNotFound) {
			return nil, "", fmt.Errorf("%w: load selected import profile: %w", ErrImportProfileCatalogUnavailable, err)
		}
		return nil, ImportProfileReasonUnavailable, nil
	}
	if !profile.UpdatedAt.Equal(binding.ProfileUpdatedAt) {
		return nil, ImportProfileReasonChanged, nil
	}
	return profile, "", nil
}

func (s *Service) resolveImportProfileBindings(
	ctx context.Context,
	export *models.WorkflowExport,
	existingNames map[string]bool,
	byKey map[string]ImportProfileBinding,
) (map[int]map[int]string, []ImportProfileConflict, error) {
	conflicts := make([]ImportProfileConflict, 0)
	directProfileIDs := make(map[int]map[int]string)
	for workflowIndex, workflow := range export.Workflows {
		if existingNames[workflow.Name] {
			continue
		}
		for _, step := range workflow.Steps {
			if step.AgentProfile == nil {
				continue
			}
			key := importProfileKey(workflowIndex, step.Position)
			binding, ok := byKey[key]
			if !ok || binding.ProfileID == "" {
				conflicts = append(conflicts, importProfileConflict(
					workflowIndex, step.Position, workflow.Name, step.Name, ImportProfileReasonMissingSelection,
				))
				continue
			}
			profile, reason, err := s.resolveSelectedImportProfile(ctx, binding, key)
			if err != nil {
				return nil, nil, err
			}
			if reason != "" {
				conflicts = append(conflicts, importProfileConflict(
					workflowIndex, step.Position, workflow.Name, step.Name, reason,
				))
				continue
			}
			workflowProfiles := directProfileIDs[workflowIndex]
			if workflowProfiles == nil {
				workflowProfiles = make(map[int]string)
				directProfileIDs[workflowIndex] = workflowProfiles
			}
			workflowProfiles[step.Position] = profile.ID
		}
	}
	return directProfileIDs, conflicts, nil
}

func (s *Service) prepareImportedWorkflows(
	ctx context.Context,
	export *models.WorkflowExport,
	existingNames map[string]bool,
	directProfileIDs map[int]map[int]string,
) ([]preparedImportedWorkflow, *ImportResult, error) {
	prepared := make([]preparedImportedWorkflow, 0, len(export.Workflows))
	result := &ImportResult{}
	for index, workflow := range export.Workflows {
		if existingNames[workflow.Name] {
			result.Skipped = append(result.Skipped, workflow.Name)
			continue
		}
		item, err := s.prepareImportedWorkflow(ctx, workflow, directProfileIDs[index])
		if err != nil {
			return nil, nil, fmt.Errorf("failed to import workflow %q: %w", workflow.Name, err)
		}
		prepared = append(prepared, item)
		result.Created = append(result.Created, workflow.Name)
	}
	return prepared, result, nil
}

func (s *Service) persistImportedWorkflows(
	ctx context.Context,
	workspaceID string,
	prepared []preparedImportedWorkflow,
) error {
	for _, item := range prepared {
		if _, err := s.persistImportedWorkflow(ctx, workspaceID, item); err != nil {
			return fmt.Errorf("failed to import workflow %q: %w", item.portable.Name, err)
		}
	}
	return nil
}

func (s *Service) ImportWorkflowsWithBindings(
	ctx context.Context,
	workspaceID string,
	export *models.WorkflowExport,
	bindings []ImportProfileBinding,
) (*ImportResult, error) {
	if err := s.AuthorizeWorkspace(ctx, workspaceID); err != nil {
		return nil, err
	}
	if err := validateImportExport(export); err != nil {
		return nil, err
	}
	existingNames, err := s.importExistingWorkflowNames(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	expected := expectedImportProfileSteps(export)
	byKey, err := validateImportProfileBindings(bindings, expected)
	if err != nil {
		return nil, err
	}
	directProfileIDs, conflicts, err := s.resolveImportProfileBindings(ctx, export, existingNames, byKey)
	if err != nil {
		return nil, err
	}
	if len(conflicts) > 0 {
		return nil, &ImportProfileResolutionError{Conflicts: conflicts}
	}

	prepared, result, err := s.prepareImportedWorkflows(ctx, export, existingNames, directProfileIDs)
	if err != nil {
		return nil, err
	}
	if err := s.persistImportedWorkflows(ctx, workspaceID, prepared); err != nil {
		return nil, err
	}
	s.logger.Info("imported workflows",
		zap.String("workspace_id", workspaceID),
		zap.Int("created", len(result.Created)),
		zap.Int("skipped", len(result.Skipped)))
	return result, nil
}
