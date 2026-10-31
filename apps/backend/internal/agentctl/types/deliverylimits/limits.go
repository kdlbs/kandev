// Package deliverylimits owns the shared delivery capacity contract used by
// the agentctl journal, producer, client, and replay paths. It has no
// dependencies so every transport layer can consume the same values.
package deliverylimits

const (
	KiB int64 = 1 << 10
	MiB int64 = 1 << 20

	// MaxEventBytes is the default encoded event payload limit. The observable
	// incident event serialized to 1,060,995 bytes, which is below this cap but
	// above the former 1 MiB default.
	MaxEventBytes = 2 * MiB

	// MaxSubmissionBytes is the independent prompt submission limit. It must
	// never grow with the event limit.
	MaxSubmissionBytes = 1 * MiB

	// DeliveryEnvelopeAllowanceBytes is reserved inside the event cap for the
	// delivery fields (stream identity, sequence, ownership) that wrap a
	// prepared payload.
	DeliveryEnvelopeAllowanceBytes = 64 * KiB

	// DisplayBytes bounds aggregate tool display data, including typed output,
	// rich content, duplicate carriers, and JSON escaping. It admits the
	// observed 1,048,607-byte search result without reduction.
	DisplayBytes = 1536 * KiB

	// PreviewBytes bounds the preview carried by an existing omission marker.
	PreviewBytes = 4 * KiB

	// GenericValueBytes retains the original stricter generic JSON value policy.
	GenericValueBytes = 64 * KiB

	// WriterQueueBytes bounds queued producer requests. It fits one maximum
	// event and the complete live envelope.
	WriterQueueBytes = 4 * MiB

	// BackendDispatchQueueBytes bounds the backend dispatch queue.
	BackendDispatchQueueBytes = 4 * MiB

	// ReplayResponseBytes bounds an encoded replay response, including base64
	// expansion, event envelopes, and stream metadata.
	ReplayResponseBytes = 4 * MiB

	// PromptBytes is the supported prompt payload limit.
	PromptBytes = 1 * MiB

	// ShellTailBytes retains the existing per-stream shell output tails.
	ShellTailBytes = 256 * KiB
)

// SubmissionLimitFor returns the default internal submission limit for an
// event limit. A smaller event limit stays authoritative; a larger event limit
// never raises the prompt limit.
func SubmissionLimitFor(eventBytes int64) int64 {
	if eventBytes <= 0 || eventBytes > MaxSubmissionBytes {
		return MaxSubmissionBytes
	}
	return eventBytes
}

// EventEnvelopeBytes returns the encoded payload budget available after the
// delivery envelope allowance is reserved.
func EventEnvelopeBytes(eventBytes int64) int64 {
	if eventBytes <= 0 {
		eventBytes = MaxEventBytes
	}
	reserved := eventBytes - DeliveryEnvelopeAllowanceBytes
	if reserved < 1 {
		return eventBytes
	}
	return reserved
}

// DisplayBudgetFor returns the aggregate display budget for a configured event
// limit. A reduced event limit still reserves the delivery envelope, so
// preparation targets the smallest of the shared display budget and the
// remaining envelope capacity.
func DisplayBudgetFor(eventBytes int64) int64 {
	envelope := EventEnvelopeBytes(eventBytes)
	if envelope < DisplayBytes {
		return envelope
	}
	return DisplayBytes
}
