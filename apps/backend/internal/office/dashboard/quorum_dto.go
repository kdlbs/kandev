package dashboard

import "github.com/kandev/kandev/internal/workflow/engine"

// GuardStateDTO is the direct JSON projection of a single AC-57d guard
// entry (`engine.QuorumGuardState`), shared verbatim by the AC-24b HTTP
// quorum endpoint and the AC-64 agent decision tool response so the two
// surfaces can never report different arithmetic for the same step.
type GuardStateDTO struct {
	TargetStepID  string `json:"target_step_id"`
	Role          string `json:"role"`
	Threshold     string `json:"threshold"`
	RequiredCount int    `json:"required_count"`
	ReceivedCount int    `json:"received_count"`
	Satisfied     bool   `json:"satisfied"`
	// Reason is present if and only if Satisfied is false (AC-24b).
	Reason string `json:"reason,omitempty"`
	// Error is populated only for the evaluation_error reason.
	Error string `json:"error,omitempty"`
}

// QuorumResponseDTO is the AC-24b diagnostic response body for
// GET /workspaces/:wsId/tasks/:taskId/quorum.
type QuorumResponseDTO struct {
	Guards              []GuardStateDTO `json:"guards"`
	ReevaluationBlocked bool            `json:"reevaluation_blocked"`
}

// guardStateDTOsFromSnapshot projects an AC-57d snapshot's Guards into the
// AC-24b/AC-64 wire shape, preserving order (AC-57d/AC-61/AC-17). Always
// returns a non-nil slice so callers marshal an empty list, never null.
func guardStateDTOsFromSnapshot(guards []engine.QuorumGuardState) []GuardStateDTO {
	out := make([]GuardStateDTO, 0, len(guards))
	for _, g := range guards {
		dto := GuardStateDTO{
			TargetStepID:  g.TargetStepID,
			Role:          g.Role,
			Threshold:     g.Threshold,
			RequiredCount: g.RequiredCount,
			ReceivedCount: g.ReceivedCount,
			Satisfied:     g.Satisfied,
			Reason:        g.Reason,
		}
		if g.Error != nil {
			dto.Error = g.Error.Error()
		}
		out = append(out, dto)
	}
	return out
}
