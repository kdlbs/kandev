package repoerrors

import "errors"

var (
	// ErrAgentDeliverySubmissionConflict means an existing submission ID was
	// reused with a different immutable payload hash.
	ErrAgentDeliverySubmissionConflict = errors.New("agent delivery submission conflict")
	ErrAgentDeliverySubmissionNotFound = errors.New("agent delivery submission not found")
	ErrAgentDeliveryEventConflict      = errors.New("agent delivery event conflict")
	ErrAgentDeliveryEffectConflict     = errors.New("agent delivery effect conflict")
	ErrAgentDeliveryEffectNotFound     = errors.New("agent delivery effect not found")
)
