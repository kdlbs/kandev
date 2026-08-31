---
created: 2026-08-31
status: complete
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
legacy_specs: []
---

# Implementation Plan: Workflow Step Profile Session Reuse

## Overview

Move profile-session behavior from the workflow to each destination step and
present it with the step's agent profile in one robust selector. Delivery first
establishes the portable step contract, then points the existing safe
profile-switch lifecycle at that contract, replaces the workflow-level UX, and
proves mixed-policy workflows on desktop and mobile.

## Scope

### In scope

- Step persistence, API projection, templates, export/import, and sync
  round-trip.
- Removal of the unshipped workflow-level policy contract and UI.
- Destination-step routing for `complete`, `park_reuse`, and `park_new`.
- Execution-stamped suppression of completion events caused by parked switches.
- A combined searchable step agent-profile and session-behavior selector.
- Step draft/save/read-only behavior, localized explanations, desktop popover,
  and mobile inset drawer.
- Desktop runtime E2E, mobile configuration E2E, and public workflow guidance.

### Out of scope

- Workflow-wide or transition-edge lifecycle policies.
- Office and automation thread behavior.
- A new task-session state or automatic parked-session retention cleanup.
- Reuse across different agent profiles or revival of terminal sessions.
- Combining conditional original-session model configuration with profile
  session behavior.

## Technical approach

### Portable step policy

Keep the normalized `WorkflowProfileSessionPolicy` enum, but move
`profile_session_policy` from the workflow model to `WorkflowStep` and
`StepDefinition`. Persist it in `workflow_steps`, normalize empty or unknown
values to `complete`, and carry it through step create/update DTOs, templates,
duplication, portable `StepPortable`, import, and workflow sync.

Remove the workflow-level database field, workflow DTO field,
`WorkflowPortable` field, workflow metadata projection, frontend workflow
field, and Workflow details control. The workflow-level implementation is
unshipped, so implementation must leave one authoritative location rather than
support both.

### Profile-switch lifecycle

Retain the execution-safe parking implementation and source-session outcomes.
Change policy resolution to normalize `destinationStep.ProfileSessionPolicy`
directly. The existing engine still resolves the destination step; no action,
transition, or state-machine contract changes.

`park_reuse` uses the existing conditional nonterminal promotion,
`park_new` always prepares a destination, and `complete` retains current
eligible-nonterminal reuse behavior. Parked cleanup writes an
execution-stamped metadata intent before setting `WAITING_FOR_INPUT` and
stopping the old execution. Completion and stopped handlers consume the matching
intent and skip workflow advancement. Preserve the terminal-candidate,
primary-promotion, teardown-failure, and queue-rollback guards.

### Combined step agent selector

Replace `StepAgentProfileSelect` with a dedicated combined selector. The
closed trigger shows the profile and a compact session-behavior summary. The
desktop popover follows `ModelConfigSelector`: a searchable profile list,
then a separated **Session behavior** configuration row that opens a nested
three-option view with descriptions and Back navigation.

The component updates `agent_profile_id` and `profile_session_policy` in the
same step draft. Dirty tracking and coordinated save compare and persist both.
Synced workflows display both values read-only. The existing
`configure_session` incompatibility remains explicit and does not silently
discard either value.

Do not force this workflow-specific data into the model selector's types. Reuse
or extract small hierarchical picker primitives only where that keeps profile
health, workflow-default fallback, and step validation independent.

### Mobile design contract

Use the same trigger in the step card. On phone viewports, present the selector
as an inset bottom drawer rather than a compressed popover or short select. The
drawer has a fixed navigation header, one internal scroll owner, `100dvh`
constraints, safe-area padding, touch targets of at least 44 px, focus return,
keyboard-safe search, wrapped descriptions, and no page-level horizontal
overflow. Desktop and mobile share filtering, validation, draft updates, dirty
tracking, and save logic.

### Public documentation

Update `docs/public/tasks-and-workflows.md` to locate the setting on each step,
explain destination-step semantics and defaults, and show when repeated steps
should reuse context or start a fresh conversation.

## Tests

- Map AC 001.1, 001.6, and 001.10 to workflow-step repository, request,
  template, export/import, and sync tests.
- Map AC 001.1 through 001.5 and 001.9 to focused orchestrator profile-switch and
  delayed-callback tests that supply different destination-step policies in one
  workflow.
- Map AC 001.7, 001.8, and 001.11 to frontend selector, draft, dirty,
  read-only, desktop, and mobile presentation tests.

## E2E tests

- Extend `apps/web/e2e/tests/workflow/workflow-agent-switch.spec.ts` with
  per-step save/reload and `A -> B -> A` runtime scenarios for
  `park_reuse` and `park_new`.
- Extend `apps/web/e2e/tests/workflow/mobile-workflow-settings.spec.ts` with
  profile search, nested policy selection, save/reload, focus return, 44 px
  hit-area checks, safe-area containment, and no document horizontal overflow.

## Work orders

- [x] [Task 01: Move policy to workflow steps](task-01-add-portable-workflow-policy.md)
- [x] [Task 02: Route sessions from the destination step](task-02-implement-safe-session-parking.md)
- [x] [Task 03: Build the combined step agent selector](task-03-expose-workflow-policy.md)
- [x] [Task 04: Prove mixed step policies and document behavior](task-04-prove-and-document-reuse-flows.md)

## Verification results

- Task 01: Step persistence, portable export/import, template, and sync tests
  passed as part of the affected backend package suite.
- Task 02: Profile-switch, durable stop-intent, queue-rollback, and lifecycle
  race tests passed as part of the focused orchestrator suite.
- Task 03: Focused frontend tests passed (52 tests), together with lint,
  typecheck, i18n checks, and the new-code ratchet.
- Task 04: Desktop policy E2E passed (2 tests), mobile selector E2E passed (2
  targeted tests), and public docs validation passed (61 tests and 41 pages).
- Backend affected packages passed: `go test -tags fts5 -count=1
  ./internal/backendapp ./internal/orchestrator ./internal/task/... ./internal/workflow/...`
  (7,209 tests across 27 packages).
- `make build`, `make e2e-plugin-package`, and specification lint passed.

## Risks

- Leaving a workflow-level field in any contract creates two policy authorities
  with unclear precedence.
- A delayed completion from the stopped source execution can advance the new
  step unless suppression is bound to the exact execution identity.
- Clearing a generic metadata flag can race with a later resumed execution.
  Stamped consumption must leave a durable tombstone.
- `park_new` intentionally permits multiple nonterminal sessions with one
  profile and can grow task session counts.
- Portable step import and workflow sync can silently reset the policy if one
  step mapping or equality check is omitted.
- Frontend draft reconciliation can overwrite an unsaved step policy unless it
  is added to both displayed and saved merge logic.
- The combined selector can become cramped or trap focus on phones unless its
  responsive presentation has one scroll owner and explicit nested navigation.
