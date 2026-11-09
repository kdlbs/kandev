package service

import (
	"context"
	"fmt"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/models"
)

type syncProfileBindings struct {
	workflow string
	steps    map[int]string
	reviews  map[int]map[int]string
}

type syncProfileCheck func(models.ProfileMatch, string) (string, string)

func (b *syncProfileBindings) applyStep(step *models.WorkflowStep, portable models.StepPortable) {
	if portable.AgentProfile != nil {
		step.AgentProfileID = b.steps[portable.Position]
	}
	for index, id := range b.reviews[portable.Position] {
		if step.Events.OnEnter[index].Config == nil {
			step.Events.OnEnter[index].Config = make(map[string]interface{})
		}
		step.Events.OnEnter[index].Config[models.ReviewAgentProfileConfigKey] = id
	}
}

func (s *Service) preflightSyncProfiles(ctx context.Context, path string, wf *taskmodels.Workflow, pw models.WorkflowPortable) (*syncProfileBindings, string) {
	if s.lookupProfile == nil {
		return nil, ""
	}
	bound := &syncProfileBindings{steps: make(map[int]string), reviews: make(map[int]map[int]string)}
	existing := make(map[string]*models.WorkflowStep)
	if wf != nil {
		steps, err := s.repo.ListStepsByWorkflow(ctx, wf.ID)
		if err != nil {
			return nil, fmt.Sprintf("%s: workflow %q: unable to read existing steps", path, pw.Name)
		}
		for _, step := range steps {
			existing[step.Name] = step
		}
	}
	check := func(result models.ProfileMatch, location string) (string, string) {
		if result.Reason != "" || result.ID == "" {
			reason := result.Reason
			if reason == "" {
				reason = "no matching profile"
			}
			return "", fmt.Sprintf("%s: workflow %q %s: %s; correct the source descriptor or local profiles", path, pw.Name, location, reason)
		}
		return result.ID, ""
	}
	if pw.AgentProfile != nil {
		current := ""
		if wf != nil {
			current = wf.AgentProfileID
		}
		id, warning := check(s.lookupProfile(pw.AgentProfile, current), "default profile")
		if warning != "" {
			return nil, warning
		}
		bound.workflow = id
	}
	for _, step := range pw.Steps {
		if warning := s.preflightSyncStep(step, existing[step.Name], bound, check); warning != "" {
			return nil, warning
		}
	}
	return bound, ""
}

func (s *Service) preflightSyncStep(step models.StepPortable, old *models.WorkflowStep, bound *syncProfileBindings, check syncProfileCheck) string {
	if step.AgentProfile != nil {
		current := ""
		if old != nil {
			current = old.AgentProfileID
		}
		id, warning := check(s.lookupProfile(step.AgentProfile, current), fmt.Sprintf("step %q profile", step.Name))
		if warning != "" {
			return warning
		}
		bound.steps[step.Position] = id
	}
	used := make(map[int]bool)
	for index, action := range step.Events.OnEnter {
		if action.Type != models.OnEnterRunCodeReview {
			continue
		}
		value, ok := action.Config[models.ReviewAgentProfilePortableKey]
		if !ok {
			continue
		}
		descriptor, err := models.ParsePortableProfile(value)
		if err != nil {
			_, warning := check(models.ProfileMatch{Reason: "invalid profile descriptor"}, fmt.Sprintf("step %q review profile", step.Name))
			return warning
		}
		id, warning := check(s.matchingReviewBinding(old, descriptor, used), fmt.Sprintf("step %q review profile", step.Name))
		if warning != "" {
			return warning
		}
		if bound.reviews[step.Position] == nil {
			bound.reviews[step.Position] = make(map[int]string)
		}
		bound.reviews[step.Position][index] = id
	}
	return ""
}

func (s *Service) matchingReviewBinding(old *models.WorkflowStep, wanted *models.AgentProfilePortable, used map[int]bool) models.ProfileMatch {
	if old == nil {
		return s.lookupProfile(wanted, "")
	}
	for index, action := range old.Events.OnEnter {
		if action.Type != models.OnEnterRunCodeReview {
			continue
		}
		id, ok := action.Config[models.ReviewAgentProfileConfigKey].(string)
		if !ok || used[index] {
			continue
		}
		match := s.lookupProfile(wanted, id)
		if match.Reason == "profile lookup failed" {
			return match
		}
		if match.ID == id {
			used[index] = true
			return match
		}
	}
	return s.lookupProfile(wanted, "")
}
