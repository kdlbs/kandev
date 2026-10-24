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

Orchestrator commit ordering, the generation-one bootstrap exception, UI.

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
