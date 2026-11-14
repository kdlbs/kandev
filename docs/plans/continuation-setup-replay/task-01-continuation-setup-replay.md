---
id: "01-continuation-setup-replay"
title: "Replay successor setup events before the generation commit"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-002
acceptance_criteria:
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-002.3
system_design:
  - ../../specs/agents/system-design/harness-session-continuity.md
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
---

# Task 01: Replay successor setup events before the generation commit

## Summary

Let an authorized context continuation (and an explicit fresh start) replay
setup events retained on its successor stream before SQL commits the successor
generation, and make event processing wait for that commit instead of failing
the owner check.

## In scope

- `agent_delivery_retention.go`: when the stream has no SQL cursor, accept a
  replay from zero through the new successor setup exception.
- `delivery_stream_preflight.go`: `canReplaySuccessorSetupEvents` requires a
  predecessor-replacing execution, a fully retained unacknowledged stream, no
  unresolved or retained submissions, and an adjacent pending successor in SQL.
- `durable_silent_restore_generation.go`: the bounded successor wait covers
  context continuation as well as fresh start.

## Out of scope

Orchestrator commit ordering, changes to ordinary fresh generation-one
bootstrap behavior, and UI. The legacy follow-up below reuses the existing
first-generation wait for an explicitly authorized context continuation.

## Acceptance

1. A continuation stream with retained setup events and no SQL cursor returns a
   zero replay cursor and leaves the cursor absent.
2. Unauthorized, submission-bearing, pruned, or already committed streams keep
   their existing rejection.
3. A continuation setup event waits for the successor commit, creates no
   receipt before it, and is received exactly once after it.

## Verification

- `go test ./internal/agent/runtime/lifecycle/ -run 'TestDeliveryPreflightReplaysContinuationSetupBeforeGenerationCommit|TestContinuationSetupEventWaitsForGenerationCommit' -count=1`
  fails on the base revision (`agent delivery checkpoint missing`, then
  `durable adoption owner identity mismatch`) and passes with the fix.
- `go test ./internal/agent/runtime/lifecycle/ -count=1`.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/agent_delivery_retention.go`
- `apps/backend/internal/agent/runtime/lifecycle/delivery_stream_preflight.go`
- `apps/backend/internal/agent/runtime/lifecycle/durable_silent_restore_generation.go`
- `apps/backend/internal/agent/runtime/lifecycle/continuation_setup_replay_test.go` (new)
- `docs/specs/agents/requirements/harness-session-continuity.md`
- `docs/specs/agents/system-design/harness-session-continuity.md`
- `docs/specs/platform/system-design/durable-agent-stream-processing.md`
- `docs/plans/continuation-setup-replay/plan.md`
- `docs/plans/continuation-setup-replay/task-01-continuation-setup-replay.md`

## Dependencies

None. Builds on the fresh-start successor wait from the fresh-start delivery
generation package.

## Inputs

- [Owning requirement](../../specs/agents/requirements/harness-session-continuity.md)
- [Continuation design](../../specs/agents/system-design/harness-session-continuity.md)
- [Owning design](../../specs/platform/system-design/durable-agent-stream-processing.md)
- [Plan](plan.md)

## Risks

A too-broad zero-cursor exception could hide lost history; a missing commit
could hang startup. The guards and the existing bounded wait address both.

## Parallelism

`sequential`


## Legacy first-generation completion

The original successor results above remain historical evidence. This bounded
follow-up composes a typed-absence generation-zero branch into the existing
successor preflight and adds the optional read-only
HasPreparedLegacyContinuationBootstrap query. The existing producer already
stores an authorized blocked unfinished attempt with expected generation zero
and a prepared snapshot targeting one; no producer or waiter change is needed.

The original public source rejected native-bound and resumed candidates at the
real initialDeliveryReplayCursor entry point (0.720s). The candidate passed
these cases and existing fresh/successor/commit/cancellation controls (5.681s).
Final existing-file cases additionally cover fourteen contradictory SQL states,
ordinary/non-bootstrap/journal/query-error refusals, and first-generation commit
versus cancellation with no early receipt; the selected race suite passed
(11.460s). No installed-provider or browser execution is claimed.

SQLguard completed successfully. Full store conformance on this Windows host
fails the existing storage_temp_artifacts absolute-path fixture, including its
previous-stable upgrade use. An exact published-source overlay reproduces the
four storage conformance failures (0.282s); the new optional query is hidden in
that baseline. No schema or storage-validation change was made to bypass it.
Full conformance success is not claimed. PostgreSQL was not executed because
KANDEV_TEST_POSTGRES_DSN is not configured.

Normal hooks, current-head documentation coverage and hosted CI are separate
publication results. Existing native proof/claims, unknown delivery rows,
original native history, predecessor replay and operator authorization remain
authoritative. No automatic recovery or historical-ancestry closure is added.
