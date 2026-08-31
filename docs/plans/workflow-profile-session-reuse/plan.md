---
created: 2026-08-31
status: implemented
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
legacy_specs: []
---

# Implementation Plan: Workflow Profile Session Reuse

## Overview

Add a backward-compatible workflow policy that controls whether fixed-profile
handoffs complete, park and reuse, or park and replace task sessions. Delivery
starts with the portable workflow contract, then implements execution-safe
parking, exposes the workflow setting, and proves desktop/mobile behavior.

## Scope

### In scope

- Workflow persistence, API projection, export/import, and sync round-trip.
- Profile-switch routing for `complete`, `park_reuse`, and `park_new`.
- Execution-stamped suppression of completion events caused by parked switches.
- Workflow editor draft/save/read-only behavior and localized explanations.
- Desktop runtime E2E, mobile configuration E2E, and public workflow guidance.

### Out of scope

- Per-step lifecycle policies.
- Office and automation thread behavior.
- A new task-session state or automatic parked-session retention cleanup.
- Reuse across different agent profiles or revival of terminal sessions.

## Technical approach

### Portable workflow policy

Add `WorkflowProfileSessionPolicy` and `profile_session_policy` to the workflow
domain model. Persist it in `workflows`, normalize empty/unknown values to
`complete`, and carry it through workflow update DTOs, workflow metadata,
portable export/import, and sync. Existing workflows and imports remain
`complete` without a backfill that changes behavior.

### Profile-switch lifecycle

Extend `WorkflowMeta` so fixed-profile routing reads policy with the already
cached profile/prompt query. Parameterize session selection and source cleanup:
`park_reuse` uses the existing conditional nonterminal promotion,
`park_new` always prepares a destination, and `complete` retains current
behavior.

For parked cleanup, write an execution-stamped metadata intent before setting
`WAITING_FOR_INPUT` and stopping the old execution. Agent completion/stopped
handlers consume the matching intent and skip workflow advancement. Preserve
the existing terminal-candidate and primary-promotion race guards.

### Workflow editor

Add the field to `Workflow`, draft merge, dirty tracking, save payloads, and
duplication/import-visible state. Render one self-explaining select in Workflow
details and disable it for synced workflows. Add all copy to five locale
catalogs. Generate the Traditional Chinese pair with the existing script.

### Public documentation

Update the how-to page `docs/public/tasks-and-workflows.md` with the three
policies, the default, the runtime-stop guarantee, and guidance for choosing
continuity versus a fresh conversation.

### Mobile design contract

Use the existing one-column Workflow details composition and responsive Radix
select. The phone control uses touch-sized triggers and rows. It retains the
document as the single scroll owner and wraps explanatory copy. It relies on
the existing safe-area-aware floating Save action. No new drawer, route, or
fixed control is needed.

## Tests

- Map AC 001.1, 001.2, 001.3, 001.4, 001.5, and 001.9 to focused orchestrator
  profile-switch and delayed-callback tests in
  `event_handlers_workflow_profile_test.go` and adjacent completion tests.
- Map AC 001.6 to repository, workflow metadata, export/import, and sync tests.
- Map AC 001.7 and 001.8 to frontend draft/dirty/component tests, including
  read-only behavior and localized labels.

## E2E tests

- Extend `apps/web/e2e/tests/workflow/workflow-agent-switch.spec.ts` with a
  workflow setting save/reload and `A -> B -> A` runtime scenario for
  `park_reuse` and `park_new`.
- Extend `apps/web/e2e/tests/workflow/mobile-workflow-settings.spec.ts` with a
  touch-driven policy change, save/reload, 44px hit-area checks, and no document
  horizontal overflow.

## Work orders

- [x] [Task 01: Add portable workflow policy](task-01-add-portable-workflow-policy.md)
- [x] [Task 02: Implement safe session parking](task-02-implement-safe-session-parking.md)
- [x] [Task 03: Expose workflow policy](task-03-expose-workflow-policy.md)
- [x] [Task 04: Prove reuse flows and document behavior](task-04-prove-and-document-reuse-flows.md)

## Verification results

- Task 01: `go test -tags fts5 ./internal/task/repository/sqlite ./internal/workflow/models ./internal/workflow/service ./internal/workflow/handlers ./internal/workflowsync -run 'ProfileSessionPolicy|WorkflowMeta' -count=1` passed.
- Task 02: `go test -tags fts5 ./internal/orchestrator -run 'Test(SwitchSessionForStep|HandleAgentCompleted|HandleAgentStopped).*ProfileSessionPolicy' -count=1 -v` passed (8 tests).
- Task 03: the focused web tests passed (32 tests), `pnpm run typecheck` passed, `pnpm run i18n:check` passed, and `pnpm run i18n:ratchet` passed.
- Task 04: the desktop profile-session E2E passed (2 tests), the mobile profile-session E2E passed (1 test), and public docs validation passed (61 tests and 41 pages).
- Changed backend packages: `go test -tags fts5 -count=1 ./internal/backendapp ./internal/orchestrator ./internal/task/... ./internal/workflow/...` passed (7,212 tests across 27 packages).
- Changed Go files are gofmt-clean, and `git diff --check` passed.

## Risks

- A delayed completion from the stopped source execution can advance the new
  step unless suppression is bound to the exact execution identity.
- Clearing a generic metadata flag can race with a later resumed execution.
  Compare-and-remove must use the stored stamp.
- `park_new` intentionally permits multiple nonterminal sessions with one
  profile and can grow task session counts.
- Export/import and workflow sync can silently reset the policy if one portable
  mapping path is omitted.
- Frontend draft reconciliation can overwrite an unsaved policy unless it is
  added to both displayed and saved merge logic.
