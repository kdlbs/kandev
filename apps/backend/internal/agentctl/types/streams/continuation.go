package streams

// ContinuationSupport identifies a tested native restoration contract.
type ContinuationSupport string

const (
	ContinuationNativeSavedHistoryV1 ContinuationSupport = "native_saved_history_v1"
	ContinuationNativeSavedHistoryV2 ContinuationSupport = "native_saved_history_completed_tools_v2"
	ContinuationAgentManagedToolsV3  ContinuationSupport = "native_history_agent_managed_tools_v3"
)

// ContinuationSafetySnapshot contains no tool inputs or provider conversation content.
// Omission means the producing transport did not attest continuation safety.
type ContinuationSafetySnapshot struct {
	Support             ContinuationSupport `json:"support"`
	PromptGeneration    uint64              `json:"prompt_generation"`
	Known               bool                `json:"known"`
	Unsafe              bool                `json:"unsafe"`
	Pending             bool                `json:"pending"`
	PermissionsResolved bool                `json:"permissions_resolved"`
	CompletedReads      uint16              `json:"completed_reads"`
	CompletedTools      uint16              `json:"completed_tools,omitempty"`
}

func (s *ContinuationSafetySnapshot) SafeFor(generation uint64) bool {
	if s == nil || !s.Known || s.Unsafe || generation == 0 || s.PromptGeneration != generation {
		return false
	}
	switch s.Support {
	case ContinuationNativeSavedHistoryV1, ContinuationNativeSavedHistoryV2:
		return !s.Pending
	case ContinuationAgentManagedToolsV3:
		return s.PermissionsResolved
	default:
		return false
	}
}
