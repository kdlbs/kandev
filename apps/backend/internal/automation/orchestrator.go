package automation

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/db"
)

// OrchestratorTarget is injected by composition; scheduling remains core infrastructure.
// Send returns the existing conversation after durably accepting the prompt.
type OrchestratorTarget interface {
	Validate(context.Context, string, string) error
	Send(context.Context, string, string, string, string) (string, error)
}

func (s *Service) SetOrchestratorTarget(target OrchestratorTarget) { s.orchestratorTarget = target }
func (s *Service) validateOrchestratorTarget(ctx context.Context, a *Automation) error {
	if a.OrchestratorID == "" {
		return nil
	}
	if s.orchestratorTarget == nil {
		return fmt.Errorf("workspace orchestration is disabled")
	}
	if hasOrchestratorTaskOverrides(a) {
		return fmt.Errorf("orchestrator automations inherit their workspace assignment; clear task execution overrides")
	}
	if a.Prompt == "" || len(a.Prompt) > 24000 {
		return fmt.Errorf("orchestrator prompt must contain 1-24000 bytes")
	}
	return s.orchestratorTarget.Validate(ctx, a.WorkspaceID, a.OrchestratorID)
}
func (s *Service) validateUpdatedTarget(ctx context.Context, id string, req *UpdateAutomationRequest) error {
	a, err := s.store.GetAutomation(ctx, id)
	if err != nil {
		return err
	}
	if a == nil {
		return fmt.Errorf("automation not found")
	}
	applyAutomationUpdate(a, req)
	return s.validateOrchestratorTarget(ctx, a)
}
func (s *Service) dispatchOrchestrator(ctx context.Context, a *Automation, event *AutomationTriggeredEvent) (FireResult, error) {
	defer s.automationRunLock(a.ID)()
	// FireTrigger has already admitted this firing under the automation lock.
	// Use that exact row so v0.94.0's deduplication and capacity checks remain
	// authoritative and there is only one delivery audit per firing.
	run, err := s.store.GetRun(ctx, event.RunID)
	if err != nil {
		return FireResult{}, err
	}
	if run == nil || run.AutomationID != a.ID || run.Status != RunStatusTriggered {
		return FireResult{}, fmt.Errorf("orchestrator delivery is no longer admitted")
	}
	err = s.validateOrchestratorTarget(ctx, a)
	conversation := ""
	if err == nil {
		prompt := fmt.Sprintf("Scheduled automation: %s\n\n%s", a.Name, InterpolatePrompt(a.Prompt, event.TriggerType, event.TriggerData))
		conversation, err = s.orchestratorTarget.Send(ctx, a.WorkspaceID, a.OrchestratorID, run.ID, prompt)
	}
	status, message := RunStatusDispatched, ""
	if err != nil {
		status, message = RunStatusFailed, err.Error()
	}
	_, saveErr := s.store.db.ExecContext(ctx, s.store.db.Rebind(`UPDATE automation_runs SET status=?,error_message=?,conversation_task_id=? WHERE id=? AND status=?`), status, message, conversation, run.ID, RunStatusTriggered)
	if saveErr != nil {
		return FireResult{}, saveErr
	}
	return FireResult{RunID: run.ID}, err
}
func (s *Store) migrateOrchestratorTargets() error {
	migrate := db.NewRequiredMigrateLogger(s.db, nil)
	for _, item := range [][2]string{{"automations", "orchestrator_id"}, {"automation_runs", "conversation_task_id"}} {
		if err := migrate.Apply(item[0]+"."+item[1], fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s TEXT NOT NULL DEFAULT ''", item[0], item[1])); err != nil {
			return err
		}
	}
	_, err := s.db.Exec(`UPDATE automation_runs SET status='failed',error_message='Delivery interrupted by restart. Inspect the orchestrator chat before retrying.' WHERE status='triggered' AND automation_id IN (SELECT id FROM automations WHERE orchestrator_id<>'')`)
	return err
}

func hasOrchestratorTaskOverrides(a *Automation) bool {
	return a.AgentProfileID != "" || a.ExecutorProfileID != "" || len(a.RepositoryIDs) > 0 || len(a.Repositories) > 0 || a.WorkflowID != "" || a.WorkflowStepID != "" || (a.TaskMode != "" && a.TaskMode != TaskModeAutomationRun) || (a.RepositoryMode != "" && a.RepositoryMode != RepositoryModeNone) || (a.ContinuationPolicy != "" && a.ContinuationPolicy != ContinuationPolicyNewTask)
}
