package models

// NativeResumeDeliveryEvidence is a pinned authenticated snapshot after native
// restoration and journal retirement. SQL verifies it under admission locks.
type NativeResumeDeliveryEvidence struct {
	SessionID         string
	IncarnationID     string
	HarnessGeneration int64
	ExecutionID       string
	NativeSessionID   string
	StreamID          string
	Cursor            *AgentDeliveryCursor
	Submissions       []NativeResumeSubmissionEvidence
}

type NativeResumeSubmissionEvidence struct {
	// Empty state denotes legacy interrupted-unknown evidence.
	State             DeliverySubmissionState
	Outcome           string
	StreamID          string
	ID                string
	HarnessGeneration int64
	PayloadHash       string
	TerminalStreamID  string
	TerminalSequence  int64
}
