---
created: 2026-08-31
status: draft
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
legacy_specs: []
---

# Implementation Plan: Workflow Step Session Lifecycle

## Overview

Replace the combined three-value policy with separate step-start and step-end
settings. Then update session routing, the combined agent selector, tests, and
documentation.

The change builds on the safe parking and callback handling already implemented
in PR 3225.

## Scope

### In scope

- Two per-step enums for session start and end behavior.
- Removal of the unshipped combined policy.
- Source-step end behavior and destination-step start behavior in session
  routing.
- All four start-and-end combinations.
- Clear lifecycle labels and helper messages.
- Agent logos in profile rows and the selected-profile trigger.
- Desktop popover and mobile inset drawer behavior.
- Persistence, API, templates, portable formats, sync, tests, and public docs.

### Out of scope

- Workflow-wide or transition-edge settings.
- A new task-session state.
- Office or automation session ownership.
- Automatic cleanup of parked sessions.
- Conditional original-session model settings.

## Technical approach

### Split the contract

Replace `profile_session_policy` with:

- `profile_session_start_policy`: `reuse` or `new`.
- `profile_session_end_policy`: `complete` or `park`.

Use `reuse` and `complete` as defaults. Remove the old enum and every old
mapping because the contract is not released.

### Split routing inputs

Refactor profile switching to accept a destination start setting and a source
end setting. The start setting controls lookup or creation. The end setting
controls completion or parking.

Thread the source step or normalized end setting through every transition path.
The workflow engine core does not change. Its integration boundary changes when
a path currently retains only the destination step.

Keep the current stop-intent, lifecycle guard, promotion, queue rollback, and
credential preflight protections.

### Simplify the selector

Keep profile search as the first view. Add a **Session lifecycle** row with this
compact summary:

`Reuse on start · Complete on end`

The lifecycle view has two groups:

- **When this step starts**
  - **Reuse an available session**
  - **Start a new session**
- **When this step ends**
  - **Complete the session**
  - **Park the session**

Show short descriptions from the system design. Remove the old combined labels.

Use `AgentLogo` for actual profiles in rows and the trigger. Use the generic
agent icon for the workflow-default choice.

### Mobile design contract

Keep the inset bottom drawer on phones. The lifecycle view contains both groups
in one scroll region. Rows have a 44 px active dimension. Back returns to the
profile list. Close returns focus to the trigger.

Desktop and mobile share filtering, normalization, draft updates, dirty state,
and save behavior.

## Tests

- Persistence and portable tests cover both defaults and independent round trips.
- Orchestrator tests cover the four combinations and prove source/destination
  ownership.
- Existing race and error-path tests remain.
- Frontend tests cover copy, logos, both groups, summaries, dirty state, and
  read-only behavior.

## E2E tests

- Desktop saves two steps with different lifecycle combinations and proves all
  four runtime outcomes.
- Mobile selects both settings, saves, reloads, returns focus, and remains
  viewport-contained.

## Work orders

- [ ] [Task 01: Split the step lifecycle contract](task-01-add-portable-workflow-policy.md)
- [ ] [Task 02: Route source and destination behavior](task-02-implement-safe-session-parking.md)
- [ ] [Task 03: Simplify the selector and add agent logos](task-03-expose-workflow-policy.md)
- [ ] [Task 04: Prove all lifecycle combinations](task-04-prove-and-document-reuse-flows.md)

## Risks

- A transition path can use the destination end setting by mistake.
- Direct engine entry can lose the source step after task state changes.
- Retaining the old field creates two sources of truth.
- UI text can imply that end behavior applies after every agent turn.
- The mobile lifecycle view can become too tall without one scroll owner.
