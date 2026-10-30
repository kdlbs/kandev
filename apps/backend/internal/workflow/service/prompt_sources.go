package service

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/models"
	"sort"
)

func (s *Service) PreviewPromptSources(ctx context.Context, request models.PromptSourceImport) (*models.PromptSourceResult, error) {
	plans, err := s.planPromptSources(ctx, request, true)
	if err != nil {
		return nil, err
	}
	return sourcePreviewResult(plans), nil
}
func (s *Service) ImportPromptSources(ctx context.Context, request models.PromptSourceImport) (*models.PromptSourceResult, error) {
	plans, err := s.planPromptSources(ctx, request, true)
	if err != nil {
		return nil, err
	}
	if err := s.repo.ApplyPromptSources(ctx, plans); err != nil {
		return nil, err
	}
	s.publishPromptSourceChanges(ctx, plans)
	return sourcePlanResult(plans), nil
}
func sourcePlanResult(plans []models.PromptSourcePlan) *models.PromptSourceResult {
	out := &models.PromptSourceResult{Instances: make([]models.PromptSourceChange, 0, len(plans))}
	for _, plan := range plans {
		fields := plan.ChangedFields
		if fields == nil {
			fields = []string{}
		}
		out.Instances = append(out.Instances, models.PromptSourceChange{PromptSourceRecord: plan.Record, DesiredRevision: plan.Template.Revision, DesiredSourceLocked: plan.Record.SourceLocked, ChangedFields: fields})
		if len(fields) > 0 {
			out.Changed++
		}
	}
	return out
}
func (s *Service) planPromptSources(ctx context.Context, request models.PromptSourceImport, validateApply bool) ([]models.PromptSourcePlan, error) {
	templates, err := validateSourceEnvelope(request)
	if err != nil {
		return nil, err
	}
	if s.workflowProvider == nil || s.syncOps == nil {
		return nil, fmt.Errorf("prompt source workflow provider is unavailable")
	}
	plans := make([]models.PromptSourcePlan, 0, len(request.Instances))
	for _, instance := range request.Instances {
		if err := s.AuthorizeWorkspace(ctx, instance.WorkspaceID); err != nil {
			return nil, err
		}
		if err := s.authorizePromptSourceWrite(ctx, instance.WorkspaceID); err != nil {
			return nil, err
		}
		plan, err := s.planPromptSource(ctx, templates[instance.TemplateID], instance, validateApply)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, nil
}
func (s *Service) planPromptSource(ctx context.Context, template models.PromptSourceTemplate, instance models.PromptSourceInstance, validateApply bool) (models.PromptSourcePlan, error) {
	plan := models.PromptSourcePlan{Template: template}
	rendered, err := s.prepareSourceInput(ctx, template, instance)
	if err != nil {
		return plan, err
	}
	record, err := s.repo.GetPromptSourceInstance(ctx, instance.WorkspaceID, instance.TemplateID)
	if err != nil {
		return plan, err
	}
	workflow, err := s.sourceWorkflow(ctx, instance.WorkspaceID, template.Name, record)
	if err != nil {
		return plan, err
	}
	// Import and its write-authorized preview own the source definition lock.
	// Workspace write authorization ran above; preserve executor authority
	// without applying the ordinary-edit lock to reconciliation itself.
	// Diff passes validateApply=false and retains ordinary read visibility.
	if workflow != nil && validateApply && s.executorMutationChecker != nil {
		if err := s.executorMutationChecker(ctx, workflow.ID); err != nil {
			return plan, err
		}
	}
	plan = newPromptSourcePlan(template, instance, rendered, record, workflow)
	workflow = plan.Workflow
	if plan.ExpectedWorkflow != nil {
		plan.ExistingSteps, err = s.repo.ListStepsByWorkflow(ctx, workflow.ID)
		if err != nil {
			return plan, err
		}
	}
	byName, removed, warning := matchStepChanges(workflow, rendered.portable, plan.ExistingSteps)
	if warning != "" {
		return plan, fmt.Errorf("%s", warning)
	}
	plan.Removed = removed
	if validateApply {
		for _, step := range removed {
			if err := s.repo.CheckPromptSourceStepRemoval(ctx, step.ID); err != nil {
				return plan, err
			}
		}
	}
	plan.Steps, err = s.sourceDesiredSteps(ctx, workflow.ID, rendered, byName, validateApply)
	if err != nil {
		return plan, err
	}
	plan.ChangedFields = append(plan.ChangedFields, sourceChangedSteps(plan, byName)...)
	sort.Strings(plan.ChangedFields)
	return plan, nil
}

func (s *Service) sourceWorkflow(ctx context.Context, workspaceID, name string, record *models.PromptSourceRecord) (*taskmodels.Workflow, error) {
	existing, err := s.workflowProvider.ListWorkflows(ctx, workspaceID, true)
	if err != nil {
		return nil, err
	}
	var found *taskmodels.Workflow
	for _, workflow := range existing {
		if record != nil && workflow.ID == record.WorkflowID {
			found = workflow
			continue
		}
		if workflow.Name != name {
			continue
		}
		if record != nil || found != nil || workflow.Source == taskmodels.WorkflowSourceGitHub || workflow.Source == taskmodels.WorkflowSourcePromptSource {
			return nil, fmt.Errorf("%w: workflow name %q is already owned or ambiguous", models.ErrPromptSourceConflict, name)
		}
		owner, err := s.repo.GetPromptSourceWorkflowOwner(ctx, workflow.ID)
		if err != nil {
			return nil, err
		}
		if owner != "" {
			return nil, models.ErrPromptSourceConflict
		}
		found = workflow
	}
	if record != nil && found == nil {
		return nil, models.ErrPromptSourceConflict
	}
	return found, nil
}

func (s *Service) sourceDesiredSteps(ctx context.Context, workflowID string, rendered sourceRendered, existing map[string]*models.WorkflowStep, validateProfiles bool) ([]*models.WorkflowStep, error) {
	positions := make(map[int]string)
	for _, step := range rendered.portable.Steps {
		positions[step.Position] = uuid.NewString()
		if old := existing[step.Name]; old != nil {
			positions[step.Position] = old.ID
		}
	}
	result := make([]*models.WorkflowStep, 0, len(rendered.portable.Steps))
	for _, portable := range rendered.portable.Steps {
		step := s.stepFromPortableForSync(workflowID, portable, positions, existing[portable.Name])
		extra := rendered.extras[portable.Position]
		if err := s.validateSourceDirectProfile(ctx, extra.AgentProfileID, validateProfiles); err != nil {
			return nil, err
		}
		if extra.AgentProfileID != "" {
			step.AgentProfileID = extra.AgentProfileID
		}
		if portable.AgentProfile != nil && step.AgentProfileID == "" {
			return nil, fmt.Errorf("portable source profile is unavailable")
		}
		step.StageType = extra.StageType
		if step.StageType == "" {
			step.StageType = models.StageTypeCustom
		}
		if err := models.ValidateWorkflowStep(step); err != nil {
			return nil, err
		}
		if err := s.validateImportedStepReferences(ctx, step, step.Name); err != nil {
			return nil, err
		}
		result = append(result, step)
	}
	return result, validateWorkflowSessionTargets(result)
}

func (s *Service) DiffPromptSources(ctx context.Context) (*models.PromptSourceResult, error) {
	records, err := s.visibleSourceRecords(ctx)
	if err != nil {
		return nil, err
	}
	var plans []models.PromptSourcePlan
	for _, record := range records {
		if err := s.AuthorizeWorkspace(ctx, record.WorkspaceID); err != nil {
			if s.authorizationApplies(ctx) {
				continue
			}
			return nil, err
		}
		template, err := s.repo.GetPromptSourceTemplate(ctx, record.WorkspaceID, record.TemplateID, record.AppliedRevision)
		if err != nil {
			return nil, err
		}
		locked := record.SourceLocked
		instance := models.PromptSourceInstance{TemplateID: record.TemplateID, WorkspaceID: record.WorkspaceID, SourceLocked: &locked, Variables: record.Variables}
		plan, err := s.planPromptSource(ctx, template, instance, false)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return sourcePlanResult(plans), nil
}

type sourceWorkspaceLister interface {
	ListWorkspaces(context.Context) ([]*taskmodels.Workspace, error)
}

func (s *Service) visibleSourceRecords(ctx context.Context) ([]models.PromptSourceRecord, error) {
	if !s.authorizationApplies(ctx) {
		return s.repo.ListPromptSourceInstances(ctx)
	}
	provider, ok := s.workspaceProvider.(sourceWorkspaceLister)
	if !ok {
		return nil, ErrNotVisible
	}
	workspaces, err := provider.ListWorkspaces(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(workspaces))
	for _, workspace := range workspaces {
		ids = append(ids, workspace.ID)
	}
	return s.repo.ListPromptSourceInstancesByWorkspace(ctx, ids)
}

func (s *Service) prepareSourceInput(ctx context.Context, template models.PromptSourceTemplate, instance models.PromptSourceInstance) (sourceRendered, error) {
	if s.workspaceProvider != nil {
		workspace, err := s.workspaceProvider.GetWorkspace(ctx, instance.WorkspaceID)
		if err != nil {
			return sourceRendered{}, err
		}
		if workspace == nil {
			return sourceRendered{}, ErrNotVisible
		}
		if workspace.IsImproveKandev() {
			return sourceRendered{}, ErrWorkflowWorkspaceReadOnly
		}
	}
	if err := s.repo.CheckPromptSourceTemplate(ctx, instance.WorkspaceID, template); err != nil {
		return sourceRendered{}, err
	}
	return renderSourceTemplate(template, instance.Variables)
}

func (s *Service) validateSourceDirectProfile(ctx context.Context, id string, validate bool) error {
	if id == "" || !validate {
		return nil
	}
	if s.importProfileCatalog == nil {
		return fmt.Errorf("prompt source profile validation is unavailable")
	}
	candidate, err := s.importProfileCatalog.GetEligibleProfile(ctx, id)
	if err != nil {
		return err
	}
	if candidate == nil {
		return fmt.Errorf("source profile is unavailable")
	}
	return nil
}

// A preview must not report a proposed revision or generated ID as applied.
func sourcePreviewResult(plans []models.PromptSourcePlan) *models.PromptSourceResult {
	result := sourcePlanResult(plans)
	result.Preview = true
	for i, plan := range plans {
		if plan.ExpectedRecord != nil {
			result.Instances[i].PromptSourceRecord = *plan.ExpectedRecord
			continue
		}
		current := models.PromptSourceRecord{TemplateID: plan.Record.TemplateID, WorkspaceID: plan.Record.WorkspaceID}
		if plan.ExpectedWorkflow != nil {
			current.WorkflowID = plan.ExpectedWorkflow.ID
		}
		result.Instances[i].PromptSourceRecord = current
	}
	return result
}

func newPromptSourcePlan(template models.PromptSourceTemplate, instance models.PromptSourceInstance, rendered sourceRendered, record *models.PromptSourceRecord, workflow *taskmodels.Workflow) models.PromptSourcePlan {
	plan := models.PromptSourcePlan{Template: template}
	if workflow != nil {
		copy := *workflow
		plan.ExpectedWorkflow = workflow
		workflow = &copy
	} else {
		workflow = &taskmodels.Workflow{ID: uuid.NewString(), WorkspaceID: instance.WorkspaceID, Style: taskmodels.WorkflowStyleKanban}
		plan.ChangedFields = append(plan.ChangedFields, "workflow")
	}
	plan.Workflow = workflow
	plan.ExpectedRecord = record
	plan.Record = models.PromptSourceRecord{ID: uuid.NewString(), TemplateID: template.ID, WorkspaceID: instance.WorkspaceID, WorkflowID: workflow.ID, AppliedRevision: template.Revision, SourceLocked: *instance.SourceLocked, Variables: instance.Variables}
	if record != nil {
		plan.Record.ID = record.ID
		plan.Record.UpdatedAt = record.UpdatedAt
	}
	if sourceInstanceChanged(record, template, instance) {
		plan.ChangedFields = append(plan.ChangedFields, "instance")
	}
	if sourceWorkflowDefinitionChanged(workflow, template, rendered.portable.Prompt) {
		plan.ChangedFields = append(plan.ChangedFields, "workflow.definition")
	}
	workflow.Name, workflow.Description, workflow.Prompt = template.Name, template.Description, rendered.portable.Prompt
	workflow.Source = taskmodels.WorkflowSourceManual
	if *instance.SourceLocked {
		workflow.Source = taskmodels.WorkflowSourcePromptSource
	}
	workflow.SourcePath = template.ID
	if plan.ExpectedWorkflow != nil && (plan.ExpectedWorkflow.Source != workflow.Source || plan.ExpectedWorkflow.SourcePath != workflow.SourcePath) {
		plan.ChangedFields = append(plan.ChangedFields, "workflow.source")
	}
	return plan
}
func sourceInstanceChanged(record *models.PromptSourceRecord, template models.PromptSourceTemplate, instance models.PromptSourceInstance) bool {
	return record == nil || record.AppliedRevision != template.Revision || record.SourceLocked != *instance.SourceLocked || !sourceVariablesEqual(record.Variables, instance.Variables)
}
func sourceWorkflowDefinitionChanged(workflow *taskmodels.Workflow, template models.PromptSourceTemplate, prompt string) bool {
	return workflow.Name != template.Name || workflow.Description != template.Description || workflow.Prompt != prompt
}
func sourceChangedSteps(plan models.PromptSourcePlan, byName map[string]*models.WorkflowStep) []string {
	var fields []string
	for _, step := range plan.Steps {
		old := byName[step.Name]
		if old == nil || !stepMatchesDefinition(old, step) || old.StageType != step.StageType {
			fields = append(fields, "steps."+step.Name)
		}
	}
	for _, step := range plan.Removed {
		fields = append(fields, "steps."+step.Name)
	}
	return fields
}

type sourceWorkspaceWriter interface {
	AuthorizePromptSourceWrite(context.Context, string) error
}

// Direct reconciliation must retain the task domain's definition-write policy.
func (s *Service) authorizePromptSourceWrite(ctx context.Context, workspaceID string) error {
	if !s.authorizationApplies(ctx) {
		return nil
	}
	writer, ok := s.workspaceProvider.(sourceWorkspaceWriter)
	if !ok {
		return ErrNotVisible
	}
	return normalizeAccessError(writer.AuthorizePromptSourceWrite(ctx, workspaceID))
}
