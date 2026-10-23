---
id: "04-legacy-v1-serialization"
title: "Preserve legacy v1 event serialization"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DELIVERY-RETENTION-002
  - REQ-PLATFORM-DELIVERY-RETENTION-006
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-RETENTION-002.1
  - AC-PLATFORM-DELIVERY-RETENTION-006.1
system_design:
  - ../../specs/platform/system-design/backend-agent-delivery-retention-payloads.md
  - ../../specs/platform/system-design/backend-agent-delivery-retention.md
---

# Task 04: Preserve legacy v1 event serialization

## Outcome and owned change

Retained events can contain `tool_output_bytes` from an older producer. The v1
recipe decodes a typed `streams.AgentEvent` and re-encodes it. Dropping this
formerly serialized field changes otherwise valid legacy normalization.

Restore only the legacy read-compatibility `*uint64` field in
`apps/backend/internal/agentctl/types/streams/agent.go`, after `ToolStatus` and
before `NormalizedPayload`. Current producers do not set it. Keep the existing
normalization recipe, receipt validator, stored rows and lifecycle-field stripping.

## Acceptance and exclusions

1. Absent/nil keeps current serialized bytes unchanged. Present zero and nonzero
   retain the historical key, unsigned value and relative serialization order.
2. Decode/encode round trips preserve the known legacy field alongside the existing
   normalized payload; malformed field types remain normal decoding errors.
3. No progress/stall producer, format migration, permission, new setting, database
   write, prompt replay, verification bypass or permanent test is added.

## Narrow confirmation and current results

One-time fixtures live outside regular test collection and use a Go overlay in
the existing `internal/agentctl/types/streams` package. They compare JSON bytes,
field presence/order and round trips only, never compute or compare hashes.
The prepared invocation, once the shared Go owner permits it, is:

```sh
go test -overlay <temporary-overlay.json> ./internal/agentctl/types/streams -run '^TestLegacyReceiptSerialization' -count=1
```

The unchanged-base overlay reproduced real JSON-contract failures: explicit zero
and nonzero were dropped, and an invalid unsigned field was silently ignored.
The absent-field control passed. The candidate passed all three serialization
cases, malformed-field controls and the five existing normalized-payload round-trip
subcases (0.024s package time). Formatting passed. No receipt hash was computed
or compared, and no permanent test or runtime action was added.

This completes the local serialization change only. Preserve the original receipt
failure as unresolved until the corrected binary's existing validation actually
returns a result in the separately owned copy/maintenance work. Local JSON evidence
is not database repair or compaction; publication and operating application remain
with their existing owners.
