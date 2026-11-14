---
id: "03-abandon-orphan-successor-streams"
title: "Abandon orphaned successor streams"
status: done
wave: 2
depends_on: ["02-continuation-successor-wait"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.24
system_design:
  - ../../specs/platform/system-design/interrupted-prompt-recovery.md
---

# Task 03: Abandon orphaned successor streams

## Summary

A failed successor start can leave a journal stream with unacknowledged events
but no committed SQL generation and no SQL cursor. That stream must not keep the
session in unresolved-work state, and native Resume of the committed generation
must still clear the recovery block.

## Regression test (write first)

- Lifecycle/agentctl test with a journal containing stream `I:g1` fully
  acknowledged, an orphan `I:g2` (`high_water > acknowledged`, no SQL generation
  2, no SQL cursor), and an open `unresolved_durable_work` block with an
  `interrupted_unknown` submission: native Resume of generation 1 must clear the
  block and leave `HasUnresolvedWork` false.
- Record whether this test fails on current code. **If it already passes, keep
  it as coverage, skip step 2 below, and note that in the Result section.**
- A failed context-continuation bootstrap (agent stops with "agent bootstrap
  failed" before generation 2 commits) leaves no `I:g2` stream behind.
- After abandoning `I:g2`, a retry on generation 2 can append to the same
  deterministic stream ID and commit its SQL generation.
- Status, proof, and automatic boot-ready reads do not inventory or abandon
  successor streams; explicit Resume performs cleanup once.
- Mixed shell-output and segmented message events can be abandoned without a
  journal byte-accounting error.
- Negative: a stream with a committed SQL generation, or with an SQL cursor, is
  never abandoned, even with unacknowledged events.
- Negative: rollover-sealed and retirement-sealed streams remain non-reopenable.

## Implementation

1. agentctl process owner: add an "abandon stream" operation that removes the
   stream record and retained events only when called by the trusted owner,
   under dispatch serialization, for a stream generation greater than the
   owner's configured generation. The exclusive journal owner can only append
   to its configured stream, so no live owner can append to the newer stream.
2. Lifecycle: at the start of explicit `AcknowledgeNativeResumeDelivery`,
   abandon journal streams whose generation has no committed SQL generation
   row and no SQL cursor. Status, proof, and automatic boot-ready reads remain
   read-only. On failed successor bootstrap, remove the successor stream only
   after the agent process has stopped and before instance teardown.

**Stop and report** if abandonment would need to touch a stream with a
committed generation, an SQL cursor, or a live owner.

## Out of scope

- Startup sweeps; repairs stay inside Resume and the failing bootstrap.

## Acceptance

- Tests pass with `-race`; negative cases keep data intact.

## Commands

```bash
cd apps/backend
go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle/ -run 'OrphanStream|NativeResume|Continuation' -count=1
go test -trimpath -tags fts5 ./internal/agentctl/journal/ ./internal/agentctl/server/process/ ./internal/agentctl/server/api/ -count=1
make -C apps/backend build
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Result

- Added an authenticated agentctl successor-stream inventory and abandonment
  operation. It serializes with prompt dispatch, validates the exact stream
  snapshot, rejects unsettled submissions, and removes retained event payloads,
  matching event-identity receipts, and stream metadata in one transaction. The
  deterministic stream ID can then be reused from sequence 1 by a later start.
- Explicit native Resume performs orphan discovery once before status and
  unresolved-work evaluation. Read-only status, proof, and automatic
  boot-ready paths do not inspect successor streams. Generation or native
  identity mismatches skip cleanup and remain subject to the existing refusal
  checks. Failed successor bootstrap cleanup runs after agent termination and
  before runtime teardown. SQL generations, cursors, active dispatches, and
  running bootstrap owners preserve their streams.
- Added lifecycle regressions for Resume block resolution, retrying the same
  generation and deterministic stream ID, failed bootstrap cleanup, read-only
  status/proof requests, and mismatch handling. Journal tests cover mixed
  shell-output/segmented payload accounting and keep rollover/retirement seals
  non-reopenable.
- Result: the failed-bootstrap retry reproduces the previous sealed-stream
  failure before the fix. The initial Resume regression was already in place
  from implementation; the new retry and cleanup-boundary regressions pass.
- Passed:
  - `go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle/ -run 'OrphanStream|NativeResume|Continuation' -count=1`
  - `go test -trimpath -tags fts5 ./internal/agentctl/journal/ ./internal/agentctl/server/process/ ./internal/agentctl/server/api/ -count=1`
  - `bash scripts/lint-go-changed` on Go files changed from local `HEAD` (0 issues)
  - `make -C apps/backend build`
  - `python3 scripts/list-docs.py validate`
  - `python3 scripts/lint-spec-files.py --all`
