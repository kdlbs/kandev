package service

import (
	"context"
	"errors"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// BackgroundWorkloadUsageDTO carries usage statistics attributed to a workload.
type BackgroundWorkloadUsageDTO struct {
	WorkID          string `json:"work_id"`
	TokensIn        int64  `json:"tokens_in"`
	TokensOut       int64  `json:"tokens_out"`
	TokensTotal     int64  `json:"tokens_total"`
	CostSubcents    int64  `json:"cost_subcents"`
	TokenProvenance string `json:"token_provenance,omitempty"`
	CostProvenance  string `json:"cost_provenance,omitempty"`
	Provenance      string `json:"provenance"` // "reported" | "estimated" | "unavailable"
}

func matchSubagentContext(workload *models.BackgroundWorkload, sc *models.SubagentContext) bool {
	if sc == nil {
		return false
	}
	if sc.ToolCallID != "" && (sc.ToolCallID == workload.SourceCallID || sc.ToolCallID == workload.ID) {
		return true
	}
	if sc.AgentID != nil && *sc.AgentID != "" && (*sc.AgentID == workload.ID || *sc.AgentID == workload.SourceCallID) {
		return true
	}
	if sc.ChildSessionID != nil && *sc.ChildSessionID != "" && *sc.ChildSessionID == workload.ID {
		return true
	}
	return false
}

// GetBackgroundWorkloadUsage returns the token usage and cost attributed to a workload.
func (s *Service) GetBackgroundWorkloadUsage(ctx context.Context, sessionID, workID string) (*BackgroundWorkloadUsageDTO, error) {
	sessionID = strings.TrimSpace(sessionID)
	workID = strings.TrimSpace(workID)
	if sessionID == "" || workID == "" {
		return nil, errors.New("session ID and work ID are required")
	}
	if err := s.AuthorizeSessionAccess(ctx, sessionID); err != nil {
		return nil, err
	}
	if s.backgroundWork == nil {
		return nil, errors.New("background work repository not configured")
	}
	workload, err := s.backgroundWork.GetBackgroundWorkload(ctx, sessionID, workID)
	if err != nil || workload == nil {
		return nil, repoerrors.ErrTaskNotFound
	}
	if workload.SessionID != sessionID {
		return nil, repoerrors.ErrTaskNotFound
	}

	result := &BackgroundWorkloadUsageDTO{
		WorkID:          workID,
		TokenProvenance: "unavailable",
		CostProvenance:  "unavailable",
		Provenance:      "unavailable",
	}

	// Check for attributable child subagent usage from subagent contexts
	if s.subagentContexts != nil && workload.Kind == "subagent" {
		contexts, err := s.subagentContexts.ListSubagentContextsBySession(ctx, sessionID)
		if err == nil {
			for _, sc := range contexts {
				if matchSubagentContext(workload, sc) && sc.TotalTokens != nil && *sc.TotalTokens > 0 {
					result.TokensTotal = *sc.TotalTokens
					result.TokenProvenance = "reported"
					result.Provenance = "reported"
					break
				}
			}
		}
	}

	return result, nil
}
