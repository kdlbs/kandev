---
id: "01-preserve-storage-evidence"
title: "Preserve storage evidence"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001
  - REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002
  - REQ-PLATFORM-DIAGNOSTIC-SIGNAL-001
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001.1
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001.3
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001.6
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001.7
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.1
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.5
  - AC-PLATFORM-DIAGNOSTIC-SIGNAL-001.5
system_design:
  - ../../specs/platform/system-design/delivery-storage-diagnostics.md
  - ../../specs/platform/system-design/runtime-diagnostic-signal.md
---

# Preserve storage evidence

## Summary

A later untyped write can replace the original typed failure. The launcher
accepts byte counts but drops the storage reason code.

## Scope

- Use the prompt generation's tracked submission on host error events.
- Keep the typed cause and revision in the existing `DeliveryFailureSnapshot`
  when an untyped follow-up belongs to the same unresolved prompt and owner.
- Log the cause and known byte counts from that snapshot. Let the relay
  accept the closed reason codes through its existing field validators.

## Acceptance

1. An untyped follow-up for the same owner and prompt keeps the typed cause,
   measured bytes, and revision. A different verified owner gets its own
   evidence. Host error snapshots retain the tracked submission identity.
2. A relayed child error retains the known reason and byte fields. An unknown
   reason, duplicate field, raw error text, or invalid measurement is omitted.

## Verification

Extend the existing process-failure and child relay suites. The current
runtime projection test covers a typed snapshot received from the matching
owner. The pre-fix cases failed as `TestIssue5513SecondaryErrorDropsStorageReason`,
`TestIssue5513HostErrorWriteLosesSubmissionBinding`, and
`TestIssue5513StorageReasonIsDroppedByRelay`.

```bash
cd apps/backend
go test -trimpath ./internal/agentctl/server/process ./internal/agent/runtime/agentctl/launcher ./internal/agent/runtime/lifecycle -run 'Test(DeliveryStorage|DeliveryFailure|PipeOutput)' -count=1
```

## Likely files

- `apps/backend/internal/agentctl/server/process/delivery_failure.go` and `delivery_failure_storage_test.go`
- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agent/runtime/agentctl/launcher/child_log_context.go` and `child_log_context_test.go`

## Dependencies

None. Complete this work order before changing the monitor's recovery trigger.

## Risks

Cause retention requires a matching submission, prompt generation, and
writer owner. The existing journal reservations and ACK path own waiting
writes.

## Parallelism

Sequential.

## Results

RED: `TestDeliveryStorageSecondaryErrorPreservesSnapshot` failed when
`ErrJournalClosed` replaced the prior typed storage cause and measurements.
The relay test failed because `event_too_large` was rejected.

GREEN: The snapshot keeps prior typed evidence and its revision when the next
write has no typed storage evidence and the process, session, incarnation,
harness, stream, submission, and prompt owner still matches. Host errors use
their tracked generation binding. Backend logs include the closed cause and
known retained/limit bytes. A newer typed corruption cause replaces the saved
storage evidence and advances the revision. The child relay accepts the closed
delivery and storage reasons, including `event_too_large`, in `reason`; the
existing `error_class` vocabulary is unchanged. Unknown, duplicate, raw, and
invalid fields remain omitted.

Verification passed:

```bash
cd apps/backend
go test -trimpath ./internal/agentctl/server/process ./internal/agent/runtime/agentctl/launcher ./internal/agent/runtime/lifecycle -run 'Test(DeliveryStorage|DeliveryFailure|PipeOutput)' -count=1
go test -trimpath ./internal/agentctl/server/process -run 'TestDelivery(StorageSecondaryErrorPreservesSnapshot|StorageFailureLogIncludesCauseAndMeasurements)$' -count=1
go test -trimpath ./internal/agent/runtime/agentctl/launcher -run 'TestPipeOutput(Forwards|Rejects).*DeliveryFailureContext' -count=1
```

The two focused regression commands passed after the review fixes.
