---
id: "01-continuation-status-preservation"
title: "Keep the saved continuation through status transitions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.25
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-01.md
---

# Task 01: Keep the saved continuation through status transitions

## Summary

Save route continuations through the engine so its cached route state and the
durable record stay consistent; a later status transition for the same
generation no longer overwrites the handoff with an empty continuation.

## In scope

- `Conductor.persistContinuation` delegates to `Engine.saveContinuation` when an
  engine is attached.
- `Engine.saveContinuation` serializes with status transitions, rejects a stale
  generation with `ErrStaleGeneration`, saves durably, then refreshes the cache.
- The policy-failure and `CancelPending` transitions hold the engine mutex
  across their route read, durable write and cache update, like `MarkActive`.

## Verification

`go test ./internal/task/repository/sqlite/ -run TestContinuationConsistency`
covers starting, retrying, cancellation, stale generation and a failed save.
`TestContinuationSurvivesConcurrentStatusTransition` pauses a policy failure
and a cancel between their read and write while a continuation is saved.
