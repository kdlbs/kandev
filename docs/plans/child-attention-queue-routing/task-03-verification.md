---
id: "03-verification"
title: "Narrow verification"
status: done
wave: 3
depends_on:
  - "02-producer-reconciliation"
plan: "plan.md"
requirements:
  - REQ-TASKS-CHILD-STALL-004
acceptance_criteria:
  - AC-TASKS-CHILD-STALL-004.1
  - AC-TASKS-CHILD-STALL-004.2
  - AC-TASKS-CHILD-STALL-004.3
  - AC-TASKS-CHILD-STALL-004.4
  - AC-TASKS-CHILD-STALL-004.5
system_design:
  - ../../specs/tasks/system-design/child-turn-stalled-signal.md
---

# Task 03: Narrow Verification

## Summary

Prove the reconcile paths with focused companion unit tests and the scoped Go
checks; no broad suite, E2E, or CI change.

## In scope

- Companion tests beside the source covering: re-home to the current promptable
  primary; withdraw-and-hold when no promptable primary; withdraw-and-resolve
  when the parent is archived/terminal or the source resolved; same-child
  supersede/coalesce; isolation (user/question/approval/non-alert entries are
  never touched or reordered); read-failure retry; idempotence across passes.
- Scoped `go test`, `go build`, `go vet`, and changed-code lint.

## Out of scope

- New permanent test harness, E2E, CI, or dashboard assertions.

## Acceptance

1. The focused tests fail before the change and pass after it.
2. Only child-attention entries are affected; other senders and other children
   are preserved and unreordered.
3. `go build`, `go vet`, and changed-code lint are clean.

## Verification

From `apps/backend`:

```bash
go test -tags fts5 ./internal/orchestrator/... -run 'ChildStall|Notice' -count=1
go build ./...
go vet ./internal/orchestrator/...
```

Skip the six known upstream Windows listener tests when a package requires it.
Record command, exit code, and summary in the plan.

## Files likely touched

- `apps/backend/internal/orchestrator/child_stall_reconcile_test.go` (new)

## Dependencies

Task 02.

## Risks

Tests that assert on log strings instead of the queue state would not prove the
behavior; assert on the actual queue snapshot and source-turn marker.

## Parallelism

`sequential`

## Inputs

- `apps/backend/AGENTS.md` (testing conventions)
- `.agents/skills/tdd/references/backend-tests.md`

## Results

Added focused reconcile tests in `child_stall_reconcile_test.go`: direct
re-home/hold/withdraw paths, a `runPass`-level test proving the delivered branch
is reached from the producer pass, user-entry preservation, and same-child
supersede. Scoped `go test`, `go build`, `go vet`, and changed-code lint (0
issues) plus doc validation are recorded in the plan's verification results.
