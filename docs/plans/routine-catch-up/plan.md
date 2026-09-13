---
created: 2026-09-09
status: done
requirements:
  - REQ-OFFICE-ROUTINE-CATCHUP-001
  - REQ-OFFICE-ROUTINE-CATCHUP-002
  - REQ-OFFICE-ROUTINE-CATCHUP-003
system_design:
  - ../../specs/office/system-design/routine-catch-up-01.md
  - ../../specs/office/system-design/routine-catch-up-02.md
---

# Implementation plan: Office routine catch-up semantics (gap 24)

## Overview

Four surfaces disagreed about what happens when a routine's cron trigger comes
due while the backend is down: the policy name `enqueue_missed_with_cap`, the
`catch_up_max` field, the scheduler design docs, and the web UI label all said
missed ticks are enqueued up to a cap, while the code counted elapsed ticks and
then dispatched **exactly one** run regardless of the count. This plan makes
the code's behavior the deliberate, reviewed contract: one resume produces one
wake, `catch_up_max` bounds only how many elapsed ticks are counted, and the
crossed gap (missed-tick count, first-missed timestamp, truncated flag) is
measured, stored durably on the routine run, and — for a lightweight routine
under the summarizing policy — rendered into the woken agent's prompt. The
policy value is renamed `enqueue_missed_with_cap` -> `summarize_missed`, with
the old value accepted forever as a deprecated alias on write and normalized
away on read.

## Scope

In scope: the tick/claim/gap measurement and its durable storage (REQ-001), the
gap's delivery to the agent for a lightweight routine under the summarizing
policy (REQ-002), and renaming the misdescribing policy value across every
surface that names it — the Go enum, six creation paths, a SQLite table
rebuild, two frontend files, five locale catalogs, and both design documents
(REQ-003).

Out of scope: enqueueing N missed runs, renaming `catch_up_max` itself, a
trigger-update operation, downgrade compatibility, a paused routine still
firing, tick-level cron correctness (a separate gap), WIP/budget enforcement
during catch-up, and retention of gap summaries.

## Technical approach

`routines/service.go`'s `computeCatchUp` replaces the previous
`computeRoutineMissed`: it walks cron ticks capped at
`models.NormaliseCatchUpMax(routine.CatchUpMax)`, but the tick dispatches
exactly one run either way — the cap bounds counting, never dispatch.
`buildGapSummary` turns that result into a durable `catch_up_missed_ticks` /
`catch_up_first_missed_at` / `catch_up_truncated` record on the created
`RoutineRun`, written once at creation so a run later marked skipped,
coalesced or failed still carries the gap measured for its tick. For a
lightweight routine under `summarize_missed`, the wakeup payload and prompt
builder surface the same three fields. `models/catchup.go` owns the
policy rename (with the deprecated alias normalized on read and write) and the
`catch_up_max` clamp, funneled through `insertRoutine`/`UpdateRoutine` so every
write path and the one-time upgrade migration agree.

## Implementation waves and parallel candidates

- [x] [Task 01: Tick, claim, and gap durability](task-01-tick-and-gap.md) (`done`) — REQ-001, REQ-002
- [x] [Task 02: Rename the catch-up policy across every surface](task-02-policy-rename.md) (`done`) — REQ-003

Sequential: task 02's rename touches the same `Routine`/`RoutineTrigger`
plumbing task 01 introduces, and both share the frontend catch-up policy
control.

## Verification results

Full `internal/office/...` suite green (`go test ./internal/office/...`),
`go vet`, `golangci-lint run --new-from-rev=<PR base> ./...` 0 issues,
`go run ./cmd/sqlguard ./internal` clean, `pnpm run i18n:check` /
`i18n:ratchet` clean (5 locales + pseudo), `pnpm vitest run
app/office/routines` green, `python3 scripts/lint-spec-files.py --all` clean.
E2E: `routine-catch-up-policy-ui.spec.ts` and the mobile counterpart, plus the
`routines-ui`/`routines`/`routine-fire` regression trio, all green against a
rebuilt frontend/backend E2E bundle. A real PostgreSQL run proved the
default-policy migration's dialect-gated `ALTER TABLE ... SET DEFAULT` path
(no table rebuild needed on PG).

## Risks and out of scope

- `catch_up_max` reaches SQL at exactly two statements (`insertRoutine`,
  `UpdateRoutine`) — the single normalization funnel every write path shares.
- Nothing is discarded past the cap: no run was ever going to exist for a
  missed tick, so truncation bounds the count, never the gap's start
  timestamp.
- A pre-existing, unrelated mismatch between the Routines create-dialog's
  camelCase request fields and the backend's snake_case JSON tags (also
  affecting `concurrencyPolicy`) is tracked separately, not fixed here.
