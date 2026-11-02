package deliverylimits

import "testing"

// @covers AC-PLATFORM-BOUNDED-TOOL-DATA-004.1
// @covers AC-PLATFORM-BOUNDED-TOOL-DATA-004.4
func TestDeliveryLimitRelationships(t *testing.T) {
	if MaxEventBytes != 2*MiB {
		t.Fatalf("max event bytes = %d, want %d", MaxEventBytes, 2*MiB)
	}
	if DeliveryEnvelopeAllowanceBytes >= MaxEventBytes {
		t.Fatalf("delivery envelope allowance %d does not fit the event cap %d", DeliveryEnvelopeAllowanceBytes, MaxEventBytes)
	}
	if DisplayBytes >= MaxEventBytes {
		t.Fatalf("display budget %d must leave room for the delivery envelope", DisplayBytes)
	}
	if DisplayBytes+DeliveryEnvelopeAllowanceBytes > MaxEventBytes {
		t.Fatalf("display budget %d plus envelope allowance %d exceeds the event cap %d", DisplayBytes, DeliveryEnvelopeAllowanceBytes, MaxEventBytes)
	}
	if MaxSubmissionBytes != PromptBytes {
		t.Fatalf("submission limit %d diverged from the prompt limit %d", MaxSubmissionBytes, PromptBytes)
	}
	if MaxSubmissionBytes >= MaxEventBytes {
		t.Fatalf("submission limit %d must stay below the event limit %d", MaxSubmissionBytes, MaxEventBytes)
	}
	if WriterQueueBytes < MaxEventBytes+DeliveryEnvelopeAllowanceBytes {
		t.Fatalf("writer queue %d cannot fit a maximum event", WriterQueueBytes)
	}
	if BackendDispatchQueueBytes < MaxEventBytes+DeliveryEnvelopeAllowanceBytes {
		t.Fatalf("backend dispatch queue %d cannot fit a maximum event", BackendDispatchQueueBytes)
	}
	// Live envelopes carry the prepared event as JSON and base64 payloads, so
	// the queues must fit the largest encoded record, not only its raw bytes.
	encodedMaximum := MaxEventBytes*4/3 + DeliveryEnvelopeAllowanceBytes
	if WriterQueueBytes < encodedMaximum || BackendDispatchQueueBytes < encodedMaximum {
		t.Fatalf("queue bounds %d/%d cannot fit an encoded maximum event %d",
			WriterQueueBytes, BackendDispatchQueueBytes, encodedMaximum)
	}
	if ReplayResponseBytes < MaxEventBytes+DeliveryEnvelopeAllowanceBytes {
		t.Fatalf("replay bytes %d cannot fit a maximum event", ReplayResponseBytes)
	}
	if ShellTailBytes >= MaxSubmissionBytes {
		t.Fatalf("shell tails %d must stay below the prompt limit", ShellTailBytes)
	}
}

func TestDeliveryLimitSubmissionAndDisplayBudgets(t *testing.T) {
	if got := SubmissionLimitFor(MaxEventBytes); got != MaxSubmissionBytes {
		t.Fatalf("submission limit for the default event cap = %d, want %d", got, MaxSubmissionBytes)
	}
	if got := SubmissionLimitFor(64 * KiB); got != 64*KiB {
		t.Fatalf("submission limit for a smaller event cap = %d, want %d", got, 64*KiB)
	}
	if got := SubmissionLimitFor(0); got != MaxSubmissionBytes {
		t.Fatalf("submission limit for an unset event cap = %d, want %d", got, MaxSubmissionBytes)
	}
	if got := EventEnvelopeBytes(MaxEventBytes); got != MaxEventBytes-DeliveryEnvelopeAllowanceBytes {
		t.Fatalf("event envelope bytes = %d, want %d", got, MaxEventBytes-DeliveryEnvelopeAllowanceBytes)
	}
	if got := DisplayBudgetFor(MaxEventBytes); got != DisplayBytes {
		t.Fatalf("display budget for the default event cap = %d, want %d", got, DisplayBytes)
	}
	small := 128 * KiB
	if got := DisplayBudgetFor(small); got != small-DeliveryEnvelopeAllowanceBytes {
		t.Fatalf("display budget for a small event cap = %d, want %d", got, small-DeliveryEnvelopeAllowanceBytes)
	}
	if got := DisplayBudgetFor(32 * KiB); got != 32*KiB {
		t.Fatalf("display budget must not go negative for a tiny event cap: %d", got)
	}
}
