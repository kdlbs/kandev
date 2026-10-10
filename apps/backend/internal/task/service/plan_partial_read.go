package service

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/task/contract"
	"github.com/kandev/kandev/internal/task/models"
)

const PlanErrorReadOffsetOutOfRange = "plan_read_offset_out_of_range"

var ErrPlanReadOffsetOutOfRange = errors.New("plan read offset exceeds content length")

type PlanReadRange struct {
	Partial            bool   `json:"partial"`
	TotalCharacters    int64  `json:"total_characters"`
	TotalContentBytes  int    `json:"total_content_bytes"`
	Offset             int64  `json:"offset"`
	Limit              int64  `json:"limit"`
	ReturnedCharacters int64  `json:"returned_characters"`
	ContentBytes       int    `json:"content_bytes"`
	HasMore            bool   `json:"has_more"`
	NextOffset         *int64 `json:"next_offset"`
}

type PlanReadResult struct {
	Plan  *models.TaskPlan
	Range *PlanReadRange
}

// GetPlanRead projects content and metadata from one authorized snapshot.
// A projected plan is a copy; range reads never modify the stored HEAD.
func (s *PlanService) GetPlanRead(ctx context.Context, taskID string, options contract.PlanReadOptions) (*PlanReadResult, error) {
	if taskID == "" {
		return nil, ErrTaskIDRequired
	}
	if err := options.Validate(); err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, taskID); err != nil {
		return nil, err
	}
	release := s.locks.acquire(taskID)
	defer release()
	head, err := s.repo.GetTaskPlan(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if options.ExpectedVersion != nil && (head == nil || head.WriteVersion != *options.ExpectedVersion) {
		return nil, planReadVersionConflict(taskID, head)
	}
	if head == nil {
		return nil, nil
	}
	result := &PlanReadResult{Plan: head}
	if options.Offset == nil && options.Limit == nil {
		return result, nil
	}
	content, page, err := projectPlanRange(taskID, head.Content, options)
	if err != nil {
		return nil, err
	}
	projected := *head
	projected.Content = content
	result.Plan, result.Range = &projected, page
	return result, nil
}

func planReadVersionConflict(taskID string, head *models.TaskPlan) error {
	err := newPlanSafetyError(PlanErrorVersionConflict, ErrPlanVersionConflict, taskID,
		"Plan was not read because expected_version does not match the current plan.",
		"Restart the read and reconcile current content. Do not retry stale pages or edits with a refreshed version.")
	if head != nil {
		err.CurrentVersion = head.WriteVersion
	}
	return err
}

func projectPlanRange(taskID, content string, options contract.PlanReadOptions) (string, *PlanReadRange, error) {
	offset, limit := int64(0), int64(contract.DefaultPlanReadCharacters)
	if options.Offset != nil {
		offset = *options.Offset
	}
	if options.Limit != nil {
		limit = *options.Limit
	}
	total := int64(utf8.RuneCountInString(content))
	if offset > total {
		return "", nil, newPlanSafetyError(PlanErrorReadOffsetOutOfRange, ErrPlanReadOffsetOutOfRange, taskID,
			"Plan was not read because offset exceeds the content length.",
			fmt.Sprintf("Use an offset between 0 and %d, or restart the read at offset 0.", total))
	}
	count := min(limit, total-offset)
	fragment := planCharacterSlice(content, offset, count)
	page := &PlanReadRange{
		Partial: true, TotalCharacters: total, TotalContentBytes: len(content),
		Offset: offset, Limit: limit, ReturnedCharacters: count, ContentBytes: len(fragment),
		HasMore: offset+count < total,
	}
	if page.HasMore {
		next := offset + count
		page.NextOffset = &next
	}
	return fragment, page, nil
}

func planCharacterSlice(content string, offset, count int64) string {
	start := len(content)
	var position int64
	for index := range content {
		if position == offset {
			start = index
		}
		if position == offset+count {
			return content[start:index]
		}
		position++
	}
	return content[start:]
}
