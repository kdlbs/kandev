---
id: "14-event-replay"
title: "Implement cursor-based event replay"
status: complete
wave: 14
depends_on: ["13-prompt-submissions"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-004
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.2
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
---

# Task 14: Implement cursor-based event replay

## Summary

Commit normalized events before publication and support bounded replay into a reconnecting backend.

## In scope

- Route durable events through the journal while preserving ACP notification ordering and native-load replay suppression.
- Implement sequence envelopes, atomic terminal records, replay-to-live handover, and acknowledgment validation.
- Bound event sizes and memory. Keep cancellation and permission response paths independent from blocked event consumers.
- Own agent_delivery_replayed_events_total and agent_delivery_sequence_errors_total at the stream boundary.
- Return typed cursor, identity, and retention errors without silently skipping records.

## Out of scope

Canonical backend projection and enabling production protocol advertisement.

## Acceptance

- A reconnect receives every committed event after its cursor, with no gap at the live boundary.
- Expired or mismatched cursors fail explicitly. Slow consumers cannot cause an unbounded queue.
- Transport replay cannot reissue native tools or permission decisions.

## Verification

Run from the repository root. New test names describe required evidence, not existing passing tests.
Use TDD for implementation. Record the failing assertion before the implementation result.

```bash
(cd apps/backend && rtk go test -race ./internal/agentctl/server/api ./internal/agentctl/server/process ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/journal ./internal/agent/runtime/agentctl -count=1)
rtk make -C apps/backend lint
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

Target evidence in `apps/backend/internal/agentctl/server/api/durable_stream_test.go`:

- `TestDurableStreamReplayLiveBoundary`: `AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.1`.
- `TestDurableStreamCursorAndBackpressure`: `AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.2`.

## Files likely touched

- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/api/agent.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_updates.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go`
- `apps/backend/internal/agent/runtime/agentctl/agent.go`
- `apps/backend/internal/agentctl/journal/` (new in Task 08).
- `apps/backend/internal/agentctl/server/api/durable_stream_test.go` (new tests or extensions).

## Dependencies

[Task 13](task-13-prompt-submissions.md).

## Risks

- Blocking the ACP receive path can deadlock cancellation. Tests must hold a slow consumer while cancellation completes.

## Parallelism

`sequential`

The primary session owns integration. This work order does not authorize subagents.
Preserve existing user edits and unrelated changes.

## Inputs

- [Owned system design](../../specs/platform/system-design/durable-agent-delivery.md).
- [Package manifest](plan.md), including shared regression gates and test prerequisites.
- Existing source and adjacent tests in the listed files.
- [Boundary decision](../../decisions/2026-09-10-durable-harness-session-boundaries.md).

## Results

Implemented commit-before-publish event metadata, cursor replay before live delivery, overlap fencing, bounded client intake, and lifecycle cursor reconnect. Focused API/client/lifecycle replay tests pass.
