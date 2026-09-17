package backendapp

import (
	"context"
	"fmt"

	orchestrationruntime "github.com/kandev/kandev/internal/orchestration/runtime"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/dispatchcontext"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

func assistantDispatchGuard(s *orchestrationruntime.Service) executor.DispatchGuard {
	return func(ctx context.Context, task *models.Task, _ *models.TaskSession, profile string) error {
		baseline, _ := task.Metadata[dispatchcontext.MetadataKey].(string)
		ref, explicit := dispatchcontext.Reference(ctx)
		if baseline == "" && ref == "" {
			return nil
		}
		if !explicit {
			ref = baseline
		}
		if s == nil || ref == "" {
			return dispatchcontext.ErrStale
		}
		// Launch/resume may also resend the task description. Do not allow a
		// fresh follow-up to launder a stale initial handoff.
		if baseline != "" && baseline != ref {
			if err := s.ValidateDispatchContext(ctx, baseline, task, profile); err != nil {
				return fmt.Errorf("%w: %s", dispatchcontext.ErrStale, err)
			}
		}
		if err := s.ValidateDispatchContext(ctx, ref, task, profile); err != nil {
			return fmt.Errorf("%w: %s", dispatchcontext.ErrStale, err)
		}
		return nil
	}
}

// Wire even with orchestration disabled: disabling a feature is not permission
// to execute previously delegated work without its context policy.
func wireAssistantDispatch(orch *orchestrator.Service, s *orchestrationruntime.Service, tasks *taskservice.Service) {
	orch.SetDispatchGuard(assistantDispatchGuard(s), func(ctx context.Context, id string) (string, error) {
		task, err := tasks.GetTask(ctx, id)
		if err != nil {
			return "", err
		}
		ref, _ := task.Metadata[dispatchcontext.MetadataKey].(string)
		return ref, nil
	})
}
