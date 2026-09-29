---
id: "01-detached-journal"
title: "Make detached durable output independent of the notification queue"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-004
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.4
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.3
system_design:
  - ../../specs/platform/system-design/durable-agent-reattachment.md
---

# Task 01: Make detached durable output independent of the notification queue

## Summary

Make detached durable output independent of the notification queue. Preserve original ownership and the no-resend contract.

## In scope

Replace blocking durable event publication with bounded wakeups backed by journal cursors.
Integrate all durable producers and live/replay writers. Preserve legacy queue behavior and typed journal failures.
Prove the drain/register/check ordering cannot miss the final committed tail when no later event arrives.
Keep event replay ordered while allowing MCP/control traffic and Stop to progress between bounded pages.
Retain quota and terminal reserve handling; this is not permission for unlimited offline output.

## Out of scope

Other work orders, contributor-owned executor redial, long-horizon scheduling, and detached MCP policy.

## Acceptance

- All named regressions exercise the actual owning boundary and pass.
- No stale owner, unrelated recovery cause, or automatic resend crosses the repaired path.
- Partial failure remains visible, bounded, and restart-reconcilable without deleting retained evidence.

## Tests

TestDetachedDurableProducerExceedsQueue: produce over 2501 events without a consumer, then replay exact output and one terminal.
TestDurableWriterFinalWake: force a commit between drain and wait; deliver a quiet final event without another notification.
TestDurableDetachReattachDuringCommit: race detach, commit, ACK, reconnect, and shutdown; assert contiguous sequence and bounded memory.
TestLegacyNotificationBackpressure: ensure no new legacy drop behavior.

## Verification

Run from repository root after implementation. All commands are independently rooted.

```bash
(cd apps/backend && go test -race ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agentctl/journal -count=1)
make -C apps/backend lint
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/api/agent.go`
- `apps/backend/internal/agentctl/server/api/durable_delivery.go`
- `apps/backend/internal/agentctl/journal`

Add the named regression files beside the owning source packages.

## Dependencies

None. Integrate with existing runtime-replacement changes.

## Risks

Concurrent old events and new ownership can race settlement or cleanup. Use immutable identity and compare-and-set at the mutation boundary.

## Parallelism

`sequential`

## Inputs

- [Manifest](plan.md).
- [Requirements](../../specs/platform/requirements/durable-agent-delivery.md).
- [Design](../../specs/platform/system-design/durable-agent-reattachment.md).

## Results

Completed 2026-09-27.

- Durable producers now use a one-slot coalesced wake after journal commit and do not wait on or enqueue payloads into the live notification queue. Legacy producers keep their existing bounded-queue backpressure.
- The durable stream writer drains ordered journal pages from its own cursor, refreshes high-water before waiting, and observes buffered wakes across the drain/wait boundary. It services MCP/control traffic between pages.
- The 2,502-event producer regression verifies exact retained sequence and terminal data with no consumer. Quiet-tail wake and detach/ACK/reattach regressions pass.
- Focused race checks passed: `go test -race ./internal/agentctl/server/process -run '^(TestDetachedDurableProducerExceedsQueue|TestSendUpdateBlockingParksUntilRoomFreesUp|TestForwardUpdatesUsesItsGenerationStopChannel)$' -count=1`; API and journal packages passed the task's full race command.
- `make -C apps/backend lint`, `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check` passed.
- Full task race command still fails in the existing process suite at `TestWorkspaceTracker_StopsWhenGitBroken` (goroutine shutdown timeout). An initial attempt also hit the host's full `/tmp`; rerunning in a private mount namespace removed the temp-directory failures but not that unrelated timeout. The process package's focused new and legacy regressions pass.
