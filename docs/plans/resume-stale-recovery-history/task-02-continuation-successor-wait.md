---
id: "02-continuation-successor-wait"
title: "Successor wait covers context continuation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-002
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-002.1
system_design:
  - ../../specs/agents/system-design/harness-session-continuity.md
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
---

# Task 02: Successor wait covers context continuation

## Summary

Events from a context-continuation execution (`ForceContextContinuation`) on
generation N+1 must wait for the successor SQL generation to commit, the same
way an explicit fresh start does, instead of being rejected as an owner
identity mismatch against generation N.

## Regression test (write first, must fail)

Add to `apps/backend/internal/agent/runtime/lifecycle/` a new file
`durable_continuation_successor_wait_test.go`:

- `TestContinuationSuccessorEventWaitsForGenerationCommit`: execution with
  `ForceContextContinuation=true`, `DeliveryHarnessGeneration=2`, SQL current
  generation 1. Deliver an authenticated `mcp_attachment` event for `I:g2`
  through `deliveryEventIsStale`; commit generation 2 after a barrier. Before
  the fix it returns `ErrDurableAdoptionIdentityMismatch` immediately; after,
  it waits and admits the event once generation 2 commits.
- `TestContinuationSuccessorEventFailsClosedWithoutCommit`: generation 2 never
  commits; the bounded wait ends with an error, never admission.
- `TestOrdinaryResumeEventStillRejectsForeignGeneration`: an execution with
  neither flag still fails closed on a generation mismatch.

## Implementation

1. `durable_silent_restore_generation.go` `replacementGenerationPending`: use
   `execution.replacesPredecessorGeneration()` (`session.go`, already
   `ForceContextContinuation || ReplacesNativeConversation`) instead of
   `ReplacesNativeConversation` alone.
2. Verify `bindSuccessorGenerationOwner` binds for continuation launches so the
   wait is cancelled with its owner; add coverage if missing.

## Out of scope

- Orphan stream cleanup (Task 03).

## Acceptance

- New tests pass with `-race`; the first fails before the change.
- Existing silent-restore and fresh-start successor tests pass.

## Result

Successor generation waiting now covers `ForceContextContinuation` through the
same predecessor-generation predicate used by the native session transition.
The regression tests prove admission waits for generation commit, launch-owner
cancellation releases the wait, and ordinary resume still rejects a foreign
generation.

Passed:

- `go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle/ -run 'Successor|Continuation|SilentRestore|FreshStart' -count=1`

## Commands

```bash
cd apps/backend
go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle/ -run 'Successor|Continuation|SilentRestore|FreshStart' -count=1
cd ../.. && bash scripts/lint-go-changed $(git diff --name-only origin/main -- '*.go')
```
