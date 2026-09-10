---
id: "07-expose-source-step-choices"
title: "Expose source-step choices"
status: complete
wave: 7
depends_on:
  - "06-route-source-step-recipients"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.8
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.10
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.12
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.13
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.2
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.6
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.8
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.9
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.11
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.12
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 07: Expose source-step choices

## Summary

Add earlier direct-profile step choices to the working initial selector.
Let authors repair invalid references without losing their draft or lifecycle choices.

## In scope

- Full edited step list, pure earlier-source derivation, stable IDs, labels and
  distinct choices when profiles match. Inherited/default steps are not sources.
- Rename/profile updates, same-workflow earlier-position restrictions, and
  source removal/reorder validation with explicit repair.
- Choose another target, Use no profile override, and discard/undo behavior.
  Invalid references disable Save; server conflicts retain drafts.
- Safe dependent-before-source save ordering. When no safe sequence exists,
  guide separate dependent repair/save before the source change. No new batch API.
- Synced workflow inspection/errors with repair in the source file.
- Extend shared desktop/phone state, fixed lifecycle placement, accessibility
  and localized copy. Reuse Task 03's picker shell and responsive branch.

## Out of scope

- Backend binding/routing and a second mobile picker implementation.
- Changing lifecycle values as a side effect of target repair.

## Acceptance

- Earlier sources are distinct, based on edited configuration, and labelled
  with step/profile. Initial targeting and profile-only options remain intact.
- Invalid source edits have explicit repair paths; no silent retargeting,
  dropped drafts, invalid persistent intermediates, or lost lifecycle settings.
- Both viewports can choose and repair targets while read-only views expose
  errors without mutation controls.

## ASCII UI preview

These excerpts use the [plan's view labels](plan.md#ascii-ui-preview).
Grouping, control order and repair actions are required; spacing is illustrative.

### UI-01: Recipient selector, second-slice addition

```text
+-------------------------------------------+
| Session lifecycle                       > | fixed
| Search agents or workflow sessions...     | fixed
+-------------------------------------------+
| WORKFLOW SESSIONS                         | scroll
| ( ) Initial agent session                 |
| (*) Implement / 5.6 Luna                  |
| ( ) Other earlier step / 5.6 Luna         |
| AGENT PROFILES                            |
| ...                                       |
+-------------------------------------------+
```

Rows reflect actual earlier direct-profile steps; do not add fictional sources.
Selecting Implement changes UI-02's target label and reuse description to its
conversation. The existing lifecycle groups and Save changes action remain.

### UI-03: Phone recipient drawer, second-slice addition

```text
+----------------------------------+
| Review                           |
| [ Implement / 5.6 Luna         v ]|
|                                  |
|  +----------------------------+  |
|  | Agent session              |  |
|  | Session lifecycle        > |  |
|  | Search...                  |  |
|  +----------------------------+  |
|  | WORKFLOW SESSIONS          |  |
|  | ( ) Initial agent session |  |
|  | (*) Implement / 5.6 Luna   |  |
|  | AGENT PROFILES             |  |
|  | ...                       |  |
|  +----------------------------+  |
|  | Bottom safe-area spacing  |  |
|  +----------------------------+  |
+----------------------------------+
```

Reuse the first slice's fixed header/search, one scrolling list, 44 px phone
hitboxes, keyboard containment, safe area and focus return.

### UI-04: Invalid source target

Desktop inline state; phone stacks the actions within the same step card:

```text
+--------------------------------------------------+
| Review                                           |
| Agent: [ Implement / 5.6 Luna                  v ]|
| ! Implement must remain an earlier source step.   |
| [Choose another target] [Use no profile override] |
| Undo the source edit to keep this target.         |
+--------------------------------------------------+
| Save changes [disabled until references are valid]|
+--------------------------------------------------+
```

Choose another target opens UI-01/UI-03. A valid choice or explicit clearing
resolves the target error and preserves lifecycle values. Discard restores
saved references. Read-only sync errors point to the source file.

UI-01 maps to AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.2; UI-03 to 001.13/002.11,
and UI-04 to 002.9/002.11 (same AC prefix). Task 08 owns rendered evidence.

## Verification

Use TDD for source derivation and changed draft/save behavior. Dependencies
are already installed by the first slice. From `apps/web`:

```bash
rtk pnpm exec vitest run lib/workflows/session-target-options.test.ts components/settings/workflow-step-agent-profile-selector.test.tsx components/settings/workflow-dirty-state.test.ts
rtk pnpm run typecheck
rtk pnpm exec eslint components/settings/workflow-step-agent-profile-selector.tsx components/settings/workflow-pipeline-editor-panels.tsx components/settings/workflow-step-mutations.ts components/settings/workflow-dirty-state.ts lib/workflows/session-target-options.ts
rtk pnpm run i18n:zh-hant
rtk pnpm run i18n:pseudo
rtk pnpm run i18n:check
rtk pnpm run i18n:ratchet
```

Include changed save-contributor/extracted selector files in targeted tests/lint.

## Files likely touched

- `apps/web/components/settings/{workflow-step-agent-profile-selector.tsx,workflow-pipeline-editor-panels.tsx,workflow-step-mutations.ts,workflow-dirty-state.ts}`
- Workflow settings save contributor and targeted draft/save tests
- `apps/web/lib/workflows/session-target-options.ts` and its tests
- `apps/web/src/locales/*/workflows.json`

## Dependencies

Tasks 05-06 supply working source references. Task 03 owns shared shell behavior.

## Risks

Using saved steps hides draft invalidation. A reorder API can reject a draft
that looked valid before a concurrent edit. Preserve errors and drafts rather
than retrying destructive source edits or silently clearing targets.

## Inputs

- Design: Combined step agent selector; Explicit recipient contract.
- Plan UI-01 through UI-04 and Task 03's shared picker implementation.

## Parallelism

`sequential`

## Results

Implemented earlier-step choices, invalid-source repair actions, dependent-step
validation, save blocking, undo, duplication, sync, and mobile drawer reuse.
Selector, validation, duplication, dirty-state, typecheck, lint, and
translation gates passed. Desktop and mobile authoring coverage passed.
