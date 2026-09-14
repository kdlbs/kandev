---
id: "04-cancellation-failure-residual"
title: "A claim whose displaced-run cancellation fails still succeeds"
status: done
wave: 3
depends_on:
  - "03-surface-refusals-and-claim-activity"
plan: "plan.md"
spec: "../../specs/office/requirements/participant-seat-provenance-assurance.md"
---

# Task 04: A claim whose displaced-run cancellation fails still succeeds

Implements `AC-OFFICE-SEAT-ASSURANCE-002.7`, pinning the **second sentence** of
`AC-OFFICE-SEAT-PROVENANCE-006.6`.

Sequenced after task 03: both add files to the `dashboard` package and this one
introduces a variant of its shared test harness.

## Problem

`AC-OFFICE-SEAT-PROVENANCE-006.6`'s second sentence is the only place the
contract accepts a residual: the claim commits, the displaced agent's queued run
cannot be cancelled, the registration still succeeds, and one run stays runnable
for an agent no longer seated. `cancelDisplacedRun` in
`dashboard/service_tasks.go` logs and swallows the error. Nothing tests that
branch.

## Acceptance

**Mechanism: take the runs store away for the duration of the call, then give it
back.** Seed the fan-out run for the displaced agent, rename the runs table
aside, drive the registration, then restore it.

Renaming rather than dropping is what makes the fourth assertion possible: after
the restore the run is still there and still runnable, which *is* the residual
the criterion describes. A dropped table would take the evidence with it.

Four assertions, one per clause of the sentence:

1. The request **succeeds** — the registration returns no error.
2. The claimed slate is **exactly as a successful claim leaves it**: one seat,
   naming the claiming agent, provenance `manual`.
3. The failure is **recorded**.
4. The displaced agent's run is **still runnable** after the restore.

For (3), the service takes a logger at construction and the logger package can
be built from a zap logger, so an **observer core at warning level** supplies
it. The existing shared test harness hands the service a default logger, so add
a **logger-accepting variant of that harness** rather than changing every caller
of it.

Assert on the observed entry's **fields** — the task, the step, the displaced
agent profile — **not** on its message text.

## Constraints

- No production code changes. The warning this case asserts already exists and
  is already emitted; the case reads it rather than adding it, which is what
  makes the assertion evidence about shipped behaviour.
- The harness variant is test-only and must leave existing callers' behaviour
  identical.
- The table rename mutates schema **within the test's own isolated database**,
  in the same family as the existing case that drops the decision table to stand
  in for a storage error. It must reach no shared state, and must restore the
  table even if the case fails partway — use `t.Cleanup`.
- New file. `dashboard/participants_test.go` is at 766 lines against a
  800-effective-line limit.

## Verification

```bash
cd apps/backend && go test ./internal/office/dashboard/... -run 'Cancel|Displaced|Claim'
cd apps/backend && go test ./internal/office/...
make -C apps/backend lint
```

Confirm the harness change is inert: the full `./internal/office/dashboard/...`
suite passes with no existing case edited.

## Files likely touched

- `apps/backend/internal/office/dashboard/` — new cancellation-failure test file
- `apps/backend/internal/office/dashboard/` — shared test harness, logger-accepting variant
- this task file

## Inputs

- `docs/specs/office/requirements/participant-seat-provenance.md` — `-006.6`
- `docs/specs/office/system-design/participant-seat-provenance-assurance-01.md`
  ("A cancellation that fails")
- `apps/backend/internal/office/dashboard/service_tasks.go` —
  `cancelDisplacedRun`, `applyParticipantAddOutcome`
- Completed task 03.
- `/tdd`

## Output contract

Return a compact handoff capsule with intent/acceptance, base/head SHA, changed
files and entry points, risk tags, exact RED/GREEN verification commands and
results, uncertainties, and this task status set to `done`. Do not edit
`plan.md`.
