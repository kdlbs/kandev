package streams

// ContinuationSupport identifies a tested native restoration contract.
type ContinuationSupport string

const ContinuationNativeSavedHistoryV1 ContinuationSupport = "native_saved_history_v1"

// ContinuationSafetySnapshot contains no tool inputs or provider conversation content.
// Omission means the producing transport did not attest continuation safety.
type ContinuationSafetySnapshot struct {
	Support          ContinuationSupport `json:"support"`
	PromptGeneration uint64              `json:"prompt_generation"`
	Known            bool                `json:"known"`
	Unsafe           bool                `json:"unsafe"`
	Pending          bool                `json:"pending"`
	CompletedReads   uint16              `json:"completed_reads"`
}

func (s *ContinuationSafetySnapshot) SafeFor(generation uint64) bool {
	return s != nil && s.Support == ContinuationNativeSavedHistoryV1 && s.Known && !s.Unsafe && !s.Pending && generation != 0 && s.PromptGeneration == generation
}
