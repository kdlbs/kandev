package orchestrator

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// taskTitleToken is the exact literal REQ-TWS-006 substitutes in workflow
// prompt templates.
const taskTitleToken = "{task_title}"

// lookupTaskTitleForTemplate returns the task's title when template contains
// {task_title}. ok is false when the token is absent, the task ID is empty, or
// the lookup fails; the caller then leaves the template unchanged, so a
// template that does not ask performs no lookup and a failed lookup degrades
// visibly (the token stays literal) instead of failing prompt building.
func (s *Service) lookupTaskTitleForTemplate(ctx context.Context, template, taskID string) (string, bool) {
	if !strings.Contains(template, taskTitleToken) || taskID == "" {
		return "", false
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		if s.logger != nil {
			s.logger.Warn("failed to load task title for prompt interpolation",
				zap.String("task_id", taskID),
				zap.Error(err))
		}
		return "", false
	}
	return task.Title, true
}

// interpolateTaskTitleIfPresent substitutes every occurrence of {task_title}
// in template and returns the inserted title. Callers apply it after every
// other placeholder so the title's own text is never scanned for tokens.
func (s *Service) interpolateTaskTitleIfPresent(ctx context.Context, template, taskID string) (string, string) {
	title, ok := s.lookupTaskTitleForTemplate(ctx, template, taskID)
	if !ok {
		return template, ""
	}
	return strings.ReplaceAll(template, taskTitleToken, title), title
}

// reserveTaskTitleInStepTemplate replaces {task_title} in a step template with
// a per-build sentinel and returns the inserted title plus a finalizer that
// swaps the sentinel for the title. The step template still passes through
// {task_id} and {{task_prompt}} substitution in between. The sentinel keeps
// the title out of that pass and cannot occur in the base prompt, so only
// positions authored in the step template receive the title. When no title is
// substituted the template is returned unchanged with an identity finalizer.
func (s *Service) reserveTaskTitleInStepTemplate(ctx context.Context, template, taskID string) (string, string, func(string) string) {
	title, ok := s.lookupTaskTitleForTemplate(ctx, template, taskID)
	if !ok {
		return template, "", func(body string) string { return body }
	}
	sentinel := "\x00kandev-task-title-" + uuid.NewString() + "\x00"
	reserved := strings.ReplaceAll(template, taskTitleToken, sentinel)
	return reserved, title, func(body string) string {
		return strings.ReplaceAll(body, sentinel, title)
	}
}
