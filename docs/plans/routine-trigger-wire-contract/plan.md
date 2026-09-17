---
created: 2026-09-10
status: done
requirements:
  - REQ-OFFICE-TRIGGER-WIRE-001
  - REQ-OFFICE-TRIGGER-WIRE-002
  - REQ-OFFICE-TRIGGER-WIRE-003
  - REQ-OFFICE-TRIGGER-WIRE-004
system_design:
  - ../../specs/office/system-design/routine-trigger-wire-contract-01.md
legacy_specs: []
---

# Implementation Plan: Office Routine Trigger Wire Contract

## Overview

Office's backend routine-trigger API is snake_case; the web app's routine
surfaces read camelCase domain fields. Before this change nothing translated
between them, so a fetched trigger's cron expression, countdown, and status
fields were `undefined` on every routine row and detail page, and a create
request serialized camelCase keys the backend never binds, so arming a cron
schedule from the UI always failed silently. Add one explicit wire adapter for
both directions, a single-source-of-truth primary-trigger selector so the row
and the detail page can never describe two different triggers, and a create
error path that reports a failed trigger sync distinctly from a failed routine
create without leaving a phantom schedule.

## Scope

### In scope

- A `normalizeRoutineTrigger`/`normalizeRoutineTriggerList` wire adapter that
  maps every field the UI reads from the snake_case API to the camelCase
  domain shape, explicitly allow-listed so an unrecognized wire key (in
  particular `secret`) never reaches domain state.
- A purpose-built `CreateRoutineTriggerInput` type and serializer that emits
  only declared, supplied optional wire fields under their snake_case keys.
- `selectPrimaryCronTrigger`, the one place that resolves "the" cron trigger a
  routine's row, detail card, detail editable fields, and trigger sync
  describe, so they cannot disagree.
- Error handling for a routine create whose trigger-arm step fails: report the
  routine-created-without-schedule message independently of the list refresh
  outcome, close the dialog, and never retry the routine create itself.

### Out of scope

- Re-tagging the Go DTOs to camelCase (Office stays uniformly snake_case).
- Trigger scheduling/firing logic (`shared.NextCronTime`) and routine status
  gating, both consumed but not owned here.
- Unifying the list row's and detail card's two distinct firing-suppression
  inputs (`routine.status` vs. the live unsaved draft status).

## Technical approach

### Wire adapter

`apps/web/lib/api/domains/office-routine-normalize.ts` builds each domain
trigger field-by-field from its snake_case source, never via spread, so a
wire key the domain type does not declare (including `secret`) cannot leak
through. Applied inside `office-api.ts`'s list/create/get functions so every
caller receives the normalized shape without its own key mapping.

### Primary trigger selection

`apps/web/app/office/lib/routine-trigger-selection.ts` filters to enabled
`cron` triggers whose `nextRunAt` `parseTurnTimestamp` resolves, sorts by that
value ascending with `id` ascending as tiebreak, and falls back to the
lowest-`id` cron trigger when no primary exists. `routine-row.tsx`,
`routine-detail-view.tsx`'s last-fired/next-fire card, and its editable
cron/timezone seeding all call this selector instead of indexing the trigger
array by position.

### Create-time error handling

`routines-content.tsx`'s `handleCreate` creates the routine first, then
attempts the trigger arm as an independent step: a failed trigger create
reports the distinct "routine created without a schedule" message, closes the
dialog, and refreshes the routine list — guarding that refresh so a failed
refetch cannot mask the already-reported trigger error or leave the dialog
open.

## Tests

- `AC-OFFICE-TRIGGER-WIRE-001.1` through `.13`:
  `apps/web/lib/api/domains/office-routine-normalize.test.ts` covers every
  field mapping, the camelCase-first/snake_case-fallback rule, idempotence,
  the two timestamp fields' pass-through-or-`undefined` rule, `enabled`
  coercion, `secret` exclusion, list/create-path filtering of empty-`id`
  results, and `kind` widening.
- `AC-OFFICE-TRIGGER-WIRE-002.1` through `.8`: the same file covers
  `serializeRoutineTriggerInput`'s snake_case-only emission, omission of
  unsupplied fields vs. explicit empty-string emission, webhook field names,
  and `CreateRoutineTriggerInput`'s server-owned-field exclusion; the
  create-error path is covered in
  `apps/web/app/office/routines/routines-content.test.tsx`.
- `AC-OFFICE-TRIGGER-WIRE-003.1` through `.7`:
  `apps/web/app/office/lib/routine-trigger-selection.test.ts` covers primary
  selection, ordering/tiebreak, and fallback; `routine-row.test.tsx` and the
  detail view's tests cover countdown/expression rendering, no-primary
  fallback display, absent-trigger display, firing suppression, and editable
  field seeding.
- `AC-OFFICE-TRIGGER-WIRE-004.1` through `.12`: routine detail sync tests
  cover the delete-then-create sequencing, the two distinct error messages,
  the `===` no-op comparison, delete-failure short-circuit, refetch-on-empty-
  body handling and its own failure message, the no-prior-trigger arm path,
  the cron-kind/non-empty-expression gate, and default timezone resolution.

## Work orders

- [x] [Task 01: Wire the routine trigger snake_case/camelCase contract](task-01-wire-routine-trigger-contract.md)

## Verification results

- Backend: unaffected (no Go changes); pre-existing backend/script test
  failures were confirmed byte-identical-at-merge-base and unrelated to this
  branch.
- Frontend: `pnpm --filter @kandev/web run test` passed.
- `make fmt`, `make typecheck`, `make lint` passed.

## Risks

- The wire adapter is the only place allowed to read `secret` off the wire; a
  future field addition must extend it explicitly rather than switching to a
  spread, or a new sensitive field could leak into domain state silently.
- `selectPrimaryCronTrigger` is now the single source of truth for "the"
  trigger; a new surface reading `triggers[0]` directly would silently
  reintroduce the row/detail disagreement this change fixes.
