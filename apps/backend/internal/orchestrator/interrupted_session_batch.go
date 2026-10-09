package orchestrator

import (
	"context"
	"errors"
)

const MaxInterruptedRecoveryBatch = 20

type InterruptedSessionBatchItem struct {
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
	InterruptedSessionResumeRequest
}

type InterruptedSessionBatchResponse struct {
	Results   []*SessionDeliveryRecoveryResponse `json:"results"`
	Completed int                                `json:"completed"`
}

func (s *Service) ResumeInterruptedSessions(ctx context.Context, items []InterruptedSessionBatchItem) (*InterruptedSessionBatchResponse, error) {
	if len(items) == 0 || len(items) > MaxInterruptedRecoveryBatch {
		return nil, errors.New("invalid interrupted recovery batch size")
	}
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if item.TaskID == "" || item.SessionID == "" || seen[item.SessionID] {
			return nil, errors.New("invalid interrupted recovery batch selection")
		}
		seen[item.SessionID] = true
	}
	response := &InterruptedSessionBatchResponse{Results: make([]*SessionDeliveryRecoveryResponse, 0, len(items))}
	for _, item := range items {
		result, err := s.ResumeInterruptedSession(ctx, item.TaskID, item.SessionID, item.InterruptedSessionResumeRequest)
		if err != nil {
			result = sessionDeliveryRecoveryResponse(item.TaskID, item.SessionID, SessionDeliveryRecoveryBlocked, "recovery_unavailable", item.RecoveryRevision, &item.RecoveryIdentity)
		}
		response.Results = append(response.Results, result)
		response.Completed++
	}
	return response, nil
}
