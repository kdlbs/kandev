package orchestrator

import "context"

// A delegated task retains its account across manual starts and workflow steps.
func (s *Service) orchestrationProfile(ctx context.Context, taskID string) string {
	if s.repo == nil {
		return ""
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return ""
	}
	if managed, _ := task.Metadata["orchestration_managed"].(bool); managed {
		return ""
	}
	profile, _ := task.Metadata["orchestration_execution_profile_id"].(string)
	return profile
}
