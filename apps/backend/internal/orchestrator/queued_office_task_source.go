package orchestrator

import (
	"context"
	"fmt"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Read-only task/frame resolution shares the real start preflight. No task
// admission mutex or queue writer is acquired under common capacity admission.
func (s *Service) officeTaskCapacityRequest(ctx context.Context, candidate OfficeCapacityWaiter) (admissionRequest, bool, error) {
	req := admissionRequest{taskID: candidate.TaskID, profileID: candidate.ProfileID, origin: launchOriginAutomatic, seam: "startTask", startSelection: &explicitWorkflowStartSelection{}}
	task, err := s.repo.GetTask(ctx, candidate.TaskID)
	if err != nil {
		return req, false, err
	}
	if task == nil {
		return req, false, fmt.Errorf("office task capacity owner unavailable")
	}
	if task.ArchivedAt != nil || task.State == v1.TaskStateCancelled {
		return req, false, nil
	}
	if candidate.WorkspaceID == "" || task.WorkspaceID == "" || candidate.WorkspaceID != task.WorkspaceID {
		return req, false, fmt.Errorf("office task and queue workspace do not match")
	}
	req.profileID, err = s.resolveEffectiveAgentProfile(ctx, task.ID, "", candidate.ProfileID)
	if err != nil {
		return req, false, err
	}
	req = s.sessionCeiling.resolveAdmissionGradeLocked(ctx, req)
	return req, true, nil
}

func (c *sessionCeilingController) officeCapacityRequestLocked(ctx context.Context, candidate OfficeCapacityWaiter) (admissionRequest, bool, error) {
	req := admissionRequest{grade: candidate.Grade, gradeKnown: candidate.GradeKnown, origin: launchOriginAutomatic, seam: officeRunCapacitySeam}
	if candidate.TaskID != "" {
		if c.officeTaskRequest == nil {
			return req, false, fmt.Errorf("office task capacity reader unavailable")
		}
		var eligible bool
		var err error
		req, eligible, err = c.officeTaskRequest(ctx, candidate)
		if err != nil || !eligible {
			return req, eligible, err
		}
	}
	req.sessionID = queuedOfficeCapacityKey(candidate.RunID)
	return req, true, nil
}

// Reuse of a real counted destination consumes no additional execution slot.
func officeTaskSelectionCounted(req admissionRequest, population gradedPopulation) bool {
	if req.startSelection == nil || req.startSelection.session == nil {
		return false
	}
	_, counted := population.counted[req.startSelection.session.ID]
	return counted
}
