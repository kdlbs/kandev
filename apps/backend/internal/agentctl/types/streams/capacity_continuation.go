package streams

// CapacityContinuationSupport identifies a tested live-conversation contract.
type CapacityContinuationSupport string

const (
	CapacityContinuationCodexLiveSessionV1    CapacityContinuationSupport = "codex_live_session_v1"
	CapacityContinuationCodexLiveSessionV2    CapacityContinuationSupport = "codex_live_session_v2"
	CapacityContinuationOpenCodeLiveSessionV1 CapacityContinuationSupport = "opencode_live_session_v1"
	CapacityContinuationMockLiveSessionV1     CapacityContinuationSupport = "mock_live_session_v1"
	CapacityContinuationMockLiveSessionV2     CapacityContinuationSupport = "mock_live_session_v2"
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
	// KnownFailedShellTools counts commands that ended with authoritative
	// nonzero terminal evidence. They remain failed in history but do not
	// block V2 capacity continuation.
	KnownFailedShellTools uint16 `json:"known_failed_shell_tools,omitempty"`
}

// SupportsKnownFailedShellTools reports whether this support version accepts
// confirmed terminal command failures as capacity continuation evidence.
func (s *CapacityContinuationSnapshot) SupportsKnownFailedShellTools() bool {
	if s == nil {
		return false
	}
	switch s.Support {
	case CapacityContinuationCodexLiveSessionV2, CapacityContinuationMockLiveSessionV2:
		return true
	default:
		return false
	}
}

// SafeFor reports whether this exact prompt has only bounded, provider-confirmed
// work that the versioned contract permits.
func (s *CapacityContinuationSnapshot) SafeFor(generation uint64) bool {
	if s == nil || generation == 0 || s.PromptGeneration != generation || !s.EvidenceComplete ||
		s.PendingTools || s.FailedTools || s.UnknownOutcomes || s.PermissionPending || s.UnaccountedBackground {
		return false
	}
	switch s.Support {
	case CapacityContinuationCodexLiveSessionV1, CapacityContinuationOpenCodeLiveSessionV1,
		CapacityContinuationMockLiveSessionV1:
		return s.CompletedTools > 0 && s.KnownFailedShellTools == 0
	case CapacityContinuationCodexLiveSessionV2, CapacityContinuationMockLiveSessionV2:
		return s.CompletedTools > 0 || s.KnownFailedShellTools > 0
	default:
		return false
	}
}
