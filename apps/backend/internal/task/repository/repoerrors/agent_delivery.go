package repoerrors

import "errors"

var (
	// ErrAgentDeliverySubmissionConflict means an existing submission ID was
	// reused with a different immutable payload hash.
	ErrAgentDeliverySubmissionConflict     = errors.New("agent delivery submission conflict")
	ErrAgentDeliverySubmissionNotFound     = errors.New("agent delivery submission not found")
	ErrAgentDeliveryEventConflict          = errors.New("agent delivery event conflict")
	ErrAgentDeliveryEffectConflict         = errors.New("agent delivery effect conflict")
	ErrAgentDeliveryEffectNotFound         = errors.New("agent delivery effect not found")
	ErrAgentDeliveryReplayExpired          = errors.New("agent delivery replay expired")
	ErrAgentDeliveryCheckpointMissing      = errors.New("agent delivery checkpoint missing")
	ErrAgentDeliveryIdentityMismatch       = errors.New("agent delivery identity mismatch")
	ErrAgentDeliveryReceiptCorrupt         = errors.New("agent delivery receipt corrupt")
	ErrAgentDeliveryHashVersion            = errors.New("agent delivery hash version unsupported")
	ErrAgentDeliveryPayloadPruned          = errors.New("agent delivery submission payload pruned")
	ErrAgentDeliveryIdentityRetiring       = errors.New("agent delivery identity is retiring")
	ErrAgentDeliveryReconstructionConflict = errors.New("agent delivery reconstruction conflicts with current state")
	ErrAgentDeliveryReconstructionStale    = errors.New("agent delivery reconstruction owner changed")
)
