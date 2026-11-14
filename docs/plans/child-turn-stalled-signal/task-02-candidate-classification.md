---
id: "02-candidate-classification"
title: "Classify at producer"
status: done
wave: 2
depends_on:
  - "01-live-settlement"
plan: "plan.md"
requirements:
  - REQ-TASKS-CHILD-STALL-001
  - REQ-TASKS-CHILD-STALL-002
  - REQ-TASKS-CHILD-STALL-003
acceptance_criteria:
  - AC-TASKS-CHILD-STALL-001.1
  - AC-TASKS-CHILD-STALL-001.2
  - AC-TASKS-CHILD-STALL-001.3
  - AC-TASKS-CHILD-STALL-002.3
  - AC-TASKS-CHILD-STALL-002.4
  - AC-TASKS-CHILD-STALL-003.2
  - AC-TASKS-CHILD-STALL-003.3
system_design:
  - ../../specs/tasks/system-design/child-turn-stalled-signal.md
---

# Task 02: Classify at Producer

## Summary and scope

Implement the producer-side classifier that qualifies each live settlement
using structured evidence already available in the task system. The
classification result is persisted alongside the candidate in the completion
transaction. No external monitor, paid classifier, or unstructured transcript
interpretation is used. A pending parent question holds the candidate and links
it to the question; the producer promotes it once a parent turn that started
after the question settles without answering it (AC-TASKS-CHILD-STALL-001.4).
Likely files: a pure classifier package under `apps/backend/internal/task/`,
an orchestrator-owned producer loop with `Start`/`Stop`, and
`apps/backend/internal/backendapp/` wiring.

## Exclusions

No paid inference, raw transcript event payload, parent prompt, workflow move,
new general event platform, public webhook, monitor binding, or permanent tests.

## Implementation acceptance

1. All listed causes qualify only with structured current evidence and the
   complete settlement predicate; ordinary success and optional signals suppress.
2. Failed reads and lost events recover through durable classification state;
   producer restart cannot duplicate classification.
3. Retry exhaustion remains visible and retryable under the original identity;
   resolved retention prevents stale replay from recreating a candidate.

## Verification

Temporary cases named `TestChildTurnClassification` and `TestChildTurnReceipt`
use the Task 01 temporary overlay convention. From `apps/backend`:

```text
go test -p 2 -tags fts5 -overlay "$env:TEMP\kandev-child-turn-stalled-signal\overlay.json" ./internal/task/service ./internal/task/repository/sqlite -run '^TestChildTurn(Classification|Receipt)'
```

Cover all causes, required/optional signal, unknown evidence, ordinary success,
active execution, reparenting/workspace mismatch, archive/terminal state,
same-step re-entry, pending/committed deferred transition, read failure, duplicate
and reordered event, restart, capped retry, retention replay, held parent
question promotion after an unanswering parent turn, and answered-question
suppression.
Assert no workflow mutation and no parent-slot acquisition during classification.
Record exact executed case counts and cleanup owned temporary files.

## Dependencies, risks, results

Depends on Task 01. The producer classifier must use
only structured evidence available at settlement time; if a cause cannot be
classified without unstructured interpretation, return an explicit unresolved
diagnostic for owner decision rather than adding inference costs.

Results (2026-10-04): the pure classifier lives in `internal/task/childstall`.
Temporary overlay `TestChildTurnClassification` passed 28 cases covering every
cause, required versus optional signal, unknown entry, abandonment,
cancellation, settle grace, deletion, archive, terminal state, reparenting,
workspace change, resumed execution, same-step re-entry, pending and
unprocessed signal, and held parent-question promotion.
