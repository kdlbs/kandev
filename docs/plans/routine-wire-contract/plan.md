---
created: 2026-09-17
status: done
requirements:
  - REQ-OFFICE-ROUTINE-WIRE-001
  - REQ-OFFICE-ROUTINE-WIRE-002
  - REQ-OFFICE-ROUTINE-WIRE-003
  - REQ-OFFICE-ROUTINE-WIRE-004
system_design:
  - ../../specs/office/system-design/routine-wire-contract.md
---

# Implementation plan: Office routine wire contract (camelCase/snake_case adapter)

## Overview

The Office routine HTTP contract is snake_case in both directions
(`apps/backend/internal/office/routines/dto.go`), while the web client sent
and read camelCase. Go's `encoding/json` matches case-insensitively but does
not fold underscores, so every multi-word key was silently discarded on
write, a cron trigger could not be armed at all, and reads rendered blanks,
placeholders, or the characters of a JSON-encoded string one per line. This
plan closes that gap with a single adapter at the client's API boundary that
translates in both directions, so no HTTP contract, Go struct tag, database
column, or scheduler behavior changes.

## Scope

In scope: a create/update/trigger request builder that emits the exact
snake_case keys the backend binds, JSON-encoding `task_template` and
`variables` as strings (REQ-001); a create-routine and detail-view control
flow that arms, replaces, and reports on a cron trigger correctly (REQ-002);
a response adapter that unwraps envelopes, decodes JSON-encoded string
fields, and produces camelCase models with no snake_case fallback casts and
no `as unknown as` (REQ-003); and the same normalization applied to routine
runs, including the previously-missing `dispatchFingerprint`, `linkedTaskId`
and `createdAt` (REQ-004).

Out of scope: any HTTP contract, Go struct tag, database column, or
scheduler behavior change; clearing an armed cron schedule; enqueueing more
than one run per resumed tick; and the pre-existing `AgentRunSummary` wire
shape consumed by `app/office/agents/[id]/`, which this capability does not
own (system design, [Out of scope](../../specs/office/system-design/routine-wire-contract.md#out-of-scope)).

## Technical approach

`apps/web/lib/api/domains/office-routine-api.ts` builds every create/update/
trigger request body from typed inputs, emitting only the contract's
snake_case keys and JSON-encoding `task_template`/`variables` before they
leave the client. `apps/web/lib/api/domains/office-routine-normalize.ts` is
the single response-side adapter: it unwraps the envelope, decodes the two
JSON-encoded string fields back into objects (falling back to `{}` for
anything that isn't a JSON object per the requirement's definition), and
maps every wire field to its camelCase model field with no
`as`/`as unknown as` cast. `apps/web/lib/api/domains/office-api.ts` now calls
through these two modules instead of shipping ad hoc request/response
shapes. `apps/web/app/office/routines/[id]/cron-reconcile.ts` centralizes the
create-before-delete replacement logic for a changed cron schedule (REQ-002:
AC-002.4 through AC-002.11), including the two-cron-trigger and
failed-relist error messages. `routine-row.tsx` and `routine-detail-view.tsx`
were changed to consume model-shape fields exclusively; `routine-row.tsx`
also gained a defensive `templateText` guard (a non-string `task_template`
title/description, reachable only because this branch is the first time that
field's content reaches JSX, would otherwise crash React's renderer).
`create-routine-dialog.tsx`'s `onSubmit` now returns `Promise<boolean>` so a
rejected `createRoutine` leaves the dialog open with the user's input intact
instead of silently resetting the form. `routine-wire-scan.test.ts` is the
static scan required by AC-003.10 and AC-003.18: it asserts no snake_case
wire key and no `as unknown as` appear under
`apps/web/app/office/routines/**` plus
`apps/web/src/office-routine-client-routes.tsx`, and that the set of files
outside that scope reading `s.office.routines` is exactly the two enumerated
in the system design's Testing section.

## Implementation waves and parallel candidates

- [x] [Task 01: camelCase/snake_case adapter and cron-reconcile control flow](task-01-wire-adapter.md) (`done`) — REQ-001, REQ-002, REQ-003, REQ-004

Single wave: the request builder, response adapter, and the UI components
that consume them all change together behind one contract, so there is no
useful task split.

## Verification results

`pnpm --filter @kandev/web run typecheck`, `pnpm --filter @kandev/web run lint`
and `pnpm --filter @kandev/web run test` (vitest, full `app/office/routines`
and `lib/api/domains/office-routine-*` suites) all green. `pnpm run
i18n:ratchet` and `pnpm run i18n:check` clean across English, `pt-pt`,
`zh-cn`, `zh-hk`, `zh-tw` and pseudo. `pnpm run build` (Vite production
build) succeeds. `apps/web/e2e` Playwright run green, including
`routines-ui.spec.ts` (create dialog persists assignee/policies/task
template and arms a cron trigger; detail view save persists a changed
schedule; list/detail render persisted assignee, policy and declared
variables; an unrecognized persisted status hides the next-fire countdown; a
post-create refetch failure reports its own toast and still closes the
dialog), `routine-catch-up-policy-ui.spec.ts`, `routine-fire.spec.ts`,
`agentctl-cli.spec.ts`'s routines coverage, and the i18n pseudo-locale
coverage check for the routines pages. Four independent review rounds
(code-reviewer, security-reviewer, test-supervisor, plus a cross-vendor
outside voice) converged with no residual defect; the final round's one
verified finding (a React crash on a non-string `task_template` field) and
one pre-existing regression (the create dialog silently discarding form
input on a rejected create) were both fixed and covered by regression tests
in the same round.

## Risks and out of scope

- The client cannot validate that the server actually stored what it sent;
  it can only prove it sent the contract's exact keys and shapes. Any future
  DTO change on the backend needs a matching adapter change, not the reverse.
- `app/office/agents/[id]/` reads `s.office.routines` for `AgentRunSummary`
  data this capability does not own; those two files are deliberately
  excluded from the AC-003.10/AC-003.18 static scan rather than migrated.
- Clearing an already-armed cron schedule from the UI remains unbuilt
  (system design, Out of scope); AC-002.13 only covers leaving it untouched
  when the submitted expression is empty after trimming.
