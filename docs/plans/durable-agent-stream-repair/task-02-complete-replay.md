---
id: "02-complete-replay"
title: "Replay every retained page before joining live delivery"
status: done
wave: 2
depends_on: ["01-legacy-streaming"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-004
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.3
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
---

# Task 02: Replay every retained page before joining live delivery

## Summary

Replay every retained page before joining live delivery. Preserve the existing durable identity and admission boundaries.

## In scope

Replace the one-page loadAgentStreamReplay contract with bounded page delivery through a captured high-water mark.
Do not accumulate the complete history into a slice. Preserve authenticated stream ownership and cursor validation on every page.
Bridge concurrent publication using journal sequences, including events committed after the snapshot but before live attachment.
The live channel cannot be the sole source of that interval. Detect a missing sequence and catch up from the journal.
Preserve duplicate suppression, connection cancellation, permission traffic, and takeover fencing.

## Out of scope

Other work orders, automatic prompt resend, native conversation replacement, and unrelated cleanup.

## Acceptance

- A real WebSocket reconnect delivers at least 2501 retained events and a concurrent live suffix without gaps or duplicate effects.
- Empty or inconsistent pages fail visibly; memory remains bounded by page and queue limits.

## Regression evidence

Extend durable_delivery_stream_test.go: TestAgentStreamReplayMultiplePages and TestAgentStreamReplayLiveHandoff. Coordinate commits before snapshot, during final page, and before live selection with barriers.

## Verification

Run from the repository root after implementation.

```bash
(cd apps/backend && go test -race ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle -count=1)
make -C apps/backend lint
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/agentctl/server/api/agent.go`
- `apps/backend/internal/agentctl/server/api/durable_delivery.go`
- `apps/backend/internal/agentctl/server/api/durable_delivery_stream_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_stream.go`

Add named regression files beside the relevant production package.

## Dependencies

Task 01: Restore legacy streaming through the real repository.

## Risks

Partial failure must preserve ownership, original submission identity, and replay evidence. Do not clear uncertainty solely because transport reconnects.

## Parallelism

`sequential`

## Inputs

- [Plan and source evidence](plan.md).
- [Delivery requirements](../../specs/platform/requirements/durable-agent-delivery.md).
- [Delivery design](../../specs/platform/system-design/durable-agent-delivery.md).
- Existing tests beside the listed files.

## Results

Implemented bounded multi-page replay with a refreshed high-water handoff before
live delivery. Added coverage for 2,501 retained events and live-tail handoff.

Validation passed:

- `(cd apps/backend && go test -race ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle -count=1)`
- `make -C apps/backend lint`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`
