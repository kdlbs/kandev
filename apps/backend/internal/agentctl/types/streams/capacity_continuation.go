package streams

// CapacityContinuationSupport identifies a tested live-conversation contract.
type CapacityContinuationSupport string

const (
	CapacityContinuationCodexLiveSessionV1 CapacityContinuationSupport = "codex_live_session_v1"
	CapacityContinuationMockLiveSessionV1  CapacityContinuationSupport = "mock_live_session_v1"
)

// CapacityContinuationSnapshot contains only bounded outcome evidence for one prompt.
// It never carries tool arguments, results, paths, prompts, or credentials.
type CapacityContinuationSnapshot struct {
	Support               CapacityContinuationSupport `json:"support"`
	PromptGeneration      uint64                      `json:"prompt_generation"`
	EvidenceComplete      bool                        `json:"evidence_complete"`
	PendingTools          bool                        `json:"pending_tools"`
	FailedTools           bool                        `json:"failed_tools"`
	UnknownOutcomes       bool                        `json:"unknown_outcomes"`
	PermissionPending     bool                        `json:"permission_pending"`
	UnaccountedBackground bool                        `json:"unaccounted_background"`
	CompletedTools        uint16                      `json:"completed_tools"`
}

// SafeFor reports whether this exact prompt has only provider-confirmed completed work.
func (s *CapacityContinuationSnapshot) SafeFor(generation uint64) bool {
	if s == nil || generation == 0 || s.PromptGeneration != generation || !s.EvidenceComplete ||
		s.PendingTools || s.FailedTools || s.UnknownOutcomes || s.PermissionPending || s.UnaccountedBackground {
		return false
	}
	switch s.Support {
	case CapacityContinuationCodexLiveSessionV1, CapacityContinuationMockLiveSessionV1:
		return true
	default:
		return false
	}
}
