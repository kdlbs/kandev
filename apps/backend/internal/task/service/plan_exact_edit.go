package service

import (
	"context"
	"strings"
)

// ExactPlanEditRequest describes one agent edit of one exact text span.
// NewText may be empty when the caller intends to delete the unique match.
type ExactPlanEditRequest struct {
	TaskID          string
	ExpectedVersion string
	OldText         string
	NewText         string
	AllowTruncation bool
}

// EditPlan replaces one unique exact occurrence in the current plan. The
// complete read, match check, version guard, and write stay inside the task's
// plan lock so a successful edit applies to the snapshot it inspected.
func (s *PlanService) EditPlan(ctx context.Context, req ExactPlanEditRequest) (PlanWriteResult, error) {
	if req.TaskID == "" {
		return PlanWriteResult{}, ErrTaskIDRequired
	}
	if err := s.authorize(ctx, req.TaskID); err != nil {
		return PlanWriteResult{}, err
	}
	if req.OldText == "" {
		return PlanWriteResult{}, newPlanSafetyError(
			PlanErrorEditTextRequired, ErrPlanEditTextRequired, req.TaskID,
			"Plan was not changed because old_text must not be empty.",
			"Read the current plan and retry with one non-empty exact fragment.",
		)
	}

	release := s.locks.acquire(req.TaskID)
	defer release()
	readCtx := context.WithoutCancel(ctx)
	head, state, headErr := s.readPlanHead(readCtx, req.TaskID)
	if state == planHeadUnknown {
		return PlanWriteResult{}, s.headUnavailableError(req.TaskID, headErr)
	}
	if state == planHeadAbsent {
		return PlanWriteResult{}, ErrTaskPlanNotFound
	}
	if err := s.requireCurrentVersion(CreatePlanRequest{
		TaskID: req.TaskID, ExpectedVersion: req.ExpectedVersion,
	}, head); err != nil {
		return PlanWriteResult{}, err
	}

	count, offset := countOverlappingOccurrences(head.Content, req.OldText)
	if count == 0 {
		err := newPlanSafetyError(
			PlanErrorEditNotFound, ErrPlanEditNotFound, req.TaskID,
			"Plan was not changed because old_text was not found in the current plan.",
			"Read the current plan and retry with an exact fragment from that snapshot.",
		)
		err.CurrentVersion = head.WriteVersion
		return PlanWriteResult{}, err
	}
	if count > 1 {
		err := newPlanSafetyError(
			PlanErrorEditAmbiguous, ErrPlanEditAmbiguous, req.TaskID,
			"Plan was not changed because old_text occurs more than once in the current plan.",
			"Read the current plan and retry with a longer exact fragment that occurs once.",
		)
		err.CurrentVersion = head.WriteVersion
		return PlanWriteResult{}, err
	}

	content := head.Content[:offset] + req.NewText + head.Content[offset+len(req.OldText):]
	if content == "" {
		return PlanWriteResult{}, s.emptyAgentPlanError(req.TaskID)
	}
	if err := checkPlanContentSize(content); err != nil {
		return PlanWriteResult{}, err
	}

	// upsertPlan performs the same read and version check again while the lock
	// is held. The expected version makes the exact span check fail closed if a
	// writer outside this service instance changed the row in between reads.
	result, err := s.upsertPlan(ctx, CreatePlanRequest{
		TaskID:             req.TaskID,
		Title:              head.Title,
		Content:            content,
		CreatedBy:          createdByAgent,
		AuthorKind:         createdByAgent,
		AgentWrite:         true,
		ExpectedVersion:    req.ExpectedVersion,
		AllowTruncation:    req.AllowTruncation,
		EvaluateTruncation: true,
		Mode:               PlanWriteModeReplace,
	}, true)
	if err != nil {
		return PlanWriteResult{}, err
	}

	release()
	s.publishPlanEvent(ctx, result.eventType, result.result.Plan)
	s.publishRevisionEvent(ctx, result.rev, result.coalesced)
	return result.result, nil
}

// countOverlappingOccurrences counts literal byte spans. Advancing one byte
// after a match is intentional: strings such as "aaa" in "aaaa" have two
// overlapping matches and are not safe to edit as one unique fragment.
func countOverlappingOccurrences(content, fragment string) (count, firstOffset int) {
	firstOffset = -1
	for start := 0; start <= len(content)-len(fragment); {
		offset := strings.Index(content[start:], fragment)
		if offset < 0 {
			break
		}
		offset += start
		if firstOffset < 0 {
			firstOffset = offset
		}
		count++
		start = offset + 1
	}
	return count, firstOffset
}
