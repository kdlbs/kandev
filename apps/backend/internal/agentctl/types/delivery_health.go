package types

import (
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// Closed delivery failure reasons. They describe why the producer stopped
// accepting ordinary output; they never encode output content or provider text.
const (
	DeliveryFailureEventTooLarge      = "event_too_large"
	DeliveryFailureStorageFull        = "storage_full"
	DeliveryFailureStorageCorrupt     = "storage_corrupt"
	DeliveryFailureStorageUnavailable = "storage_unavailable"
	DeliveryFailureWriterFailed       = "writer_failed"
)

// DeliveryFailureSnapshot is the immutable, bounded evidence captured at the
// failed write boundary. It is exposed through the authenticated status
// response and grants no outcome authority.
type DeliveryFailureSnapshot struct {
	Reason            string                             `json:"reason"`
	Storage           *streams.DeliveryStorageDiagnostic `json:"storage,omitempty"`
	ProcessGeneration uint64                             `json:"process_generation,omitempty"`
	SessionID         string                             `json:"session_id,omitempty"`
	IncarnationID     string                             `json:"incarnation_id,omitempty"`
	HarnessGeneration uint64                             `json:"harness_generation,omitempty"`
	StreamID          string                             `json:"stream_id,omitempty"`
	SubmissionID      string                             `json:"submission_id,omitempty"`
	PromptGeneration  uint64                             `json:"prompt_generation,omitempty"`
	Revision          uint64                             `json:"revision"`
	EventType         string                             `json:"event_type,omitempty"`
	EventBytes        int64                              `json:"event_bytes,omitempty"`
	EventLimit        int64                              `json:"event_limit,omitempty"`
	ObservedAt        time.Time                          `json:"observed_at"`
}
