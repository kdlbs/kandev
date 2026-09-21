---
created: 2026-09-21
status: complete
requirements:
  - REQ-UI-SETTINGS-COMPOSITION-001
  - REQ-UI-SETTINGS-COMPOSITION-002
  - REQ-UI-SETTINGS-COMPOSITION-003
  - REQ-UI-SETTINGS-COMPOSITION-004
system_design:
  - ../../specs/ui/system-design/settings-composition.md
legacy_specs: []
---

# Implementation Plan: Settings Composition

## Overview

Keep Task behavior on one page and use consistent composition across first-party settings.
Deliver shared groups with Task behavior first, then migrate bounded route families.
Use the [surface inventory](surface-inventory.md) to prevent partial coverage.
Each work order includes its own rendered and behavioral checks.

## Scope

### In scope

- Four Task behavior groups, with runtime initially collapsed and summarized.
- Shared card padding, row spacing, descriptions, fields, actions, and disclosure treatment.
- All first-party settings route families and the forms they expose.
- Localization, discovery compatibility, dirty-state preservation, phone composition, and relevant documentation.

### Out of scope

- New settings pages, navigation entries, or tabs.
- Backend changes, new permissions, altered defaults, or auto-start switch inversion.
- Global UI Card styling and third-party plugin content.
- Specialized editor, table, terminal, and pipeline redesigns.

## Inputs and decisions

The user accepted the single-page four-group proposal and requested consistent settings cards, descriptions, and layouts.
The UI system owns this independent reusable presentation contract.
Existing typography, sizing, header tabs, search, and save coordination remain authoritative dependencies.
The [typography plan](../settings-typography/plan.md) has unfinished historical migration records.
This package owns grouping and spacing without marking those records complete or replacing their evidence.
No material product question blocked implementation. The five work orders were executed sequentially in the primary session;
no delegation was used.

- [Requirements](../../specs/ui/requirements/settings-composition.md)
- [System design](../../specs/ui/system-design/settings-composition.md)
- [Save coordinator ADR](../../decisions/0046-settings-route-save-coordinator.md)

## Technical approach

Task 01 introduces small settings-local group/row compositions using existing shared roles.
It integrates them into Task behavior, preserves domain draft owners, and opens native disclosures before target focus.
Tasks 02 through 05 migrate callers by route family and record retained exceptions in the inventory.
Each task updates its own translations and runs its own targeted tests.
No global CSS sweep, schema-driven renderer, or duplicated settings state is needed.

Use `SettingsSaveDirtyScope` to aggregate group state, preserving field markers and existing save contributors.
Reuse `SettingsPageHeader`, `SettingsCardHeader`, `SettingsField`, and settings sizing helpers.
Keep header tabs and immediate commands unchanged.
Update `apps/web/AGENTS.md` with the shipped composition rule in Task 01.
Update public descriptions of Task behavior during Task 01, not during this design turn.

## ASCII UI preview

### UI-01: Task behavior, ordinary entry

Entry: `/settings/preferences/task-behavior`. Runtime is closed and the page is clean.
Current source order is Task Actions, Transcript Navigation, Todo List Panel, Message Queue, then Session capacity.
Task Actions currently includes unrelated unread and sleep controls.

```text
Task behavior
Choose how tasks open and how their content appears.

Creating and opening tasks
+--------------------------------------------------------+
| Open new tasks automatically     [switch]               |
| Open a new task after you create it.                    |
|--------------------------------------------------------|
| Agent-generated titles           [switch]               |
| Profile for agent-created tasks  [profile selector]     |
| Prevent auto-start on open       [switch]               |
+--------------------------------------------------------+
Conversation and panels
+--------------------------------------------------------+
| Unread divider                   [switch]               |
| Transcript navigation            [existing controls]    |
| Todo checklist                   [existing controls]    |
+--------------------------------------------------------+
Archiving
+--------------------------------------------------------+
| Archive confirmation             [existing controls]    |
+--------------------------------------------------------+
> Runtime and limits                       [Everyone]
  Automatic sessions: 5 | Queued messages: unlimited
```

### UI-02: Runtime, expanded with unsaved or failed edits

```text
v Runtime and limits                 [Everyone] [Unsaved]
  Effective: automatic sessions 5 | queued messages 20
+--------------------------------------------------------+
| Session capacity                                       |
| Enable automatic-session limit               [switch]  |
| Maximum automatic sessions                   [  8  ]   |
| Manual starts can exceed this limit.                   |
|--------------------------------------------------------|
| Message queue                                          |
| Maximum messages per session                 [ 20  ]   |
| Enable queued message merging                [switch]  |
| Automatically merge consecutive messages     [switch]  |
| > Details                                              |
|--------------------------------------------------------|
| Prevent host computer sleep                  [switch]  |
| Applies to the computer running Kandev.                |
| [Error or managed-value explanation when applicable]   |
+--------------------------------------------------------+
          Unsaved changes      [Reset] [Save changes]
```

Summary numbers are illustrative effective values. Edited values do not silently replace them.
Loading and unavailable states replace only the affected summary value.
Search opens the group before focus. Errors open the group and retain the existing retry action.

### UI-03: Shared phone composition

Entry: Settings index or app navigation, then the same settings route.

```text
< Settings           Task behavior
Creating and opening tasks
+--------------------------------+
| Open new tasks          [on]   |
| automatically                  |
| Explanation wraps here.        |
|--------------------------------|
| Profile for agent-created tasks|
| Explanation wraps here.        |
| [profile selector            v]|
+--------------------------------+
Conversation and panels
[Grouped rows]
Archiving
[Grouped rows]
> Runtime and limits
  Applies to everyone
  Automatic sessions: 5
  Queued messages: unlimited

 Unsaved changes
 [Reset]       [Save changes]
```

The settings shell owns vertical scrolling. The existing Save surface remains fixed only while dirty.
Phone selectors stack below descriptions. Touch actions have at least 44px targets.
Safe-area padding and content clearance keep the last setting reachable.
No separate phone tabs, drawers, or nested scrollers are introduced for groups.

### UI-04: Shared form and resource patterns

```text
Page title                           [existing tabs/action]
One page description.

Form group
+--------------------------------------------------------+
| Field label                                            |
| [value input]                                          |
| Helper explaining effect and required constraints.      |
| [Inline error when applicable]                         |
+--------------------------------------------------------+

Resource group                              [Add resource]
+--------------------------------------------------------+
| Existing resource list or table                        |
| Name / status                         [existing actions]|
+--------------------------------------------------------+
```

On phones, actions and fields stack below their labels. Resource bodies retain their established mobile composition.
Empty states keep their existing create/connect action. Loading, disabled, and failed states keep domain behavior.

Required structure: group order, one group frame, no repeated row heading, visible essential help, and the existing Save action.
Exact wording and ASCII spacing are illustrative. All rendered copy must be translated.
UI-01/02 map to AC-UI-SETTINGS-COMPOSITION-002.1 through .6 and -003.1 through .6.
UI-03 maps to -004.1 through .5. UI-04 maps to -001.1 through .5.

## Tests

| Criteria | Evidence |
| --- | --- |
| -001.1 through .5 | Existing typography tests plus family-labelled rendered matrix tests in `settings-composition.spec.ts` and `mobile-settings-composition.spec.ts` |
| -002.1 through .6 | New `task-behavior-settings.test.tsx`: group order and runtime summary loading, effective, unlimited, disabled, and failed states |
| -003.1/.2, -004.3 | New group/row component tests: accessible labels, details behavior, keyboard focus, and essential notices |
| -003.3/.5/.6 | Task behavior component tests: collapse while dirty, invalid draft, save error, partial success, Reset, and in-flight edit |
| -003.4 | Existing `lib/settings-discovery/target.test.ts`: nested closed details, disabled target fallback, repeated requests, reduced motion |
| -004.1/.2/.4/.5 | Family-labelled mobile matrix with persisted edits, geometry, translations, and navigation checks |

New tests start red before implementation. Markup-only migrations use rendered evidence rather than implementation-mirroring unit tests.
Run existing component tests for state owners that change. Do not remove behavioral assertions to accommodate new markup.

## E2E tests

Add `apps/web/e2e/tests/settings/settings-composition.spec.ts` and `mobile-settings-composition.spec.ts`.
Use existing isolated fixtures and seed representative resources for each family.
Name tests with the work-order tags: `task behavior`, `preferences`, `agent executor`, `workspace integration`, and `system account`.
Use the default desktop project and `mobile-chrome` for the mobile file.

Task behavior scenarios cover all four groups, expand/collapse/save/reload, failed save, invalid input, locked/non-admin state, and search into closed runtime.
Test direct fragments and repeated search after manual collapse. Keep effective summaries separate from unsaved drafts.
Each family covers a preference/form/resource example where applicable, empty and populated lists, and visible errors.
Measure shared spacing and equivalent control geometry rather than only checking CSS classes.
Use Pixel 5, 767px and 768px boundary checks, and a coarse-pointer tablet case for the shared primitives.
Include long translated/pseudo-locale labels and light/dark smoke captures. Record rendered comparison against UI-01 through UI-04.
Reuse and extend `settings-typography.spec.ts` and `mobile-settings-typography.spec.ts` for representative cross-family coverage.
Do not require real external credentials or containers for layout evidence.

## Work orders

- [x] [Task 01: Shared composition and Task behavior](task-01-task-behavior.md)
- [x] [Task 02: Remaining preferences](task-02-preferences.md)
- [x] [Task 03: Agents and execution settings](task-03-agent-executor.md)
- [x] [Task 04: Workspace and integration settings](task-04-workspace-integration.md)
- [x] [Task 05: System, account, and plugin settings](task-05-system-account.md)

Execute sequentially: 01 -> 02 -> 03 -> 04 -> 05. The sequence does not authorize subagents.

## Verification results

Implementation checks on 2026-09-21:

- `(cd apps/web && pnpm exec vitest run components/settings lib/settings-discovery/target.test.ts src/settings-routes.test.ts)`: passed, 207 files and 1,400 tests.
- `(cd apps/web && pnpm e2e:run e2e/tests/settings/settings-composition.spec.ts)`: passed, 7 desktop tests.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/settings/mobile-settings-composition.spec.ts)`: passed, 6 mobile tests.
- `(cd apps/web && pnpm e2e:run e2e/tests/system/message-queue-settings.spec.ts e2e/tests/system/session-capacity-settings.spec.ts e2e/tests/settings/settings-manual-save.spec.ts)`: passed, 14 desktop tests.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/system/mobile-message-queue-settings.spec.ts e2e/tests/settings/mobile-general-settings.spec.ts e2e/tests/system/mobile-session-capacity-settings.spec.ts)`: passed, 9 mobile tests.
- `(cd apps/web && pnpm e2e:run e2e/tests/settings/notifications-type-scale.spec.ts e2e/tests/integrations/github-workspace-settings.spec.ts -- --grep 'renders the card body|repository scope is saved per workspace')`: passed, 2 focused desktop tests.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/settings/mobile-notifications-type-scale.spec.ts e2e/tests/settings/mobile-settings-typography.spec.ts e2e/tests/settings/mobile-agent-profile-layout.spec.ts e2e/tests/settings/mobile-executor-profile-spacing.spec.ts e2e/tests/integrations/mobile-github-workspace-settings.spec.ts e2e/tests/settings/mobile-workspace-settings-tabs.spec.ts)`: passed, 10 mobile tests.
- `pnpm --filter @kandev/web build:vite` from `apps`: passed. Vite emitted the repository's existing chunk-size, deprecated-option, and ineffective-dynamic-import warnings.
- Typecheck, localization checks, the new-copy ratchet, scoped ESLint, scoped Prettier, and `git diff --check`: passed.
- `python3 scripts/list-docs.py validate`: passed (294 decisions, 1,067 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
Design validation on 2026-09-21:

- `python3 scripts/list-docs.py validate`: passed (294 decisions, 1067 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/list-docs.py specs --system ui --format paths`: both new artifacts discovered.
- Requirement/acceptance references, relative document links, and existing test paths: checked.
- `git diff --check -- docs/specs docs/plans`: passed.
- `git status --short -- docs/specs docs/plans`: new package files and the related typography-plan link identified.

## Review remediation results

The implementation review identified five composition gaps after the original
work-order checks. They are resolved by the current implementation and covered
by focused evidence:

- Runtime load failures, invalid drafts, and asynchronous save failures reveal
  the mounted Runtime and limits disclosure once for queue, session, and sleep
  owners. A later manual collapse remains effective until a new attention
  transition occurs.
- Queue and session draft contributors now pass their aggregate dirty state to
  the runtime group. The shared green, localized status badge is visible for
  queue-only, session-only, and descendant sleep changes, and Reset and Save
  clear it through the existing coordinator.
- Appearance simple preference cards are rows, while Notifications and
  Terminal and Editors use frameless page content with group-owned borders.
  Structural desktop checks cover all three pages and retain specialized
  editor/resource interiors.
- `SettingsRow` generates a stable description ID when callers omit one and
  preserves caller-provided `aria-describedby` values. Production-shaped,
  custom MCP radio, and wrapped Appearance Select rows cover this association.
- Switch rows use a real label hit area with mobile/coarse-pointer dimensions,
  and `SettingsDetails` summaries have the same touch sizing. The mobile
  composition test taps near a switch target edge.

Fixup remediation on 2026-09-21 addressed the automated review findings:

- Runtime attention now uses owner-specific transition keys, so a queue,
  session, or sleep failure that arrives while another failure remains active
  reopens the mounted disclosure after a manual collapse. Discovery also sends
  an explicit synchronous disclosure-open event to React-controlled groups.
- Plugin integration routes opt out of the native group frame, preserving the
  frameless boundary for plugin-owned settings content.
- Sleep attention now has a load-failure and successful-retry callback test.

The focused fixup suite passed 4 files and 28 tests:
`(cd apps/web && pnpm exec vitest run components/settings/task-behavior-settings.test.tsx components/settings/sleep-inhibition-settings.test.tsx src/plugin-integration-settings-route.test.tsx lib/settings-discovery/target.test.ts)`.

Remediation checks run after these changes:

- Full settings component matrix: 207 files and 1,400 tests passed.
- `settings-composition.spec.ts`: 7 desktop tests passed, including the
  Appearance, Notifications, and Terminal and Editors one-frame checks.
- `mobile-settings-composition.spec.ts`: 6 mobile tests passed, including the
  coarse-pointer switch-edge activation check.
- Desktop typography/type-scale checks: 3 tests passed. Mobile
  typography/type-scale checks: 3 tests passed.
- Desktop runtime/manual-save checks: 14 tests passed. Mobile runtime/settings
  checks: 9 tests passed.
- Existing auto-focus desktop and mobile flows were updated to target the
  migrated row surface and passed with retries disabled (1 test each).

Use the exact commands in each work order. Install dependencies once with `(cd apps && pnpm install --frozen-lockfile)` before the first package command.
The managed E2E runner rebuilds the frontend and backend and enforces its resource limits.
Do not overlap these E2E commands or use worker overrides.

## Risks

- Collapsing content can hide invalid controls or break search focus unless disclosure revelation precedes focus.
- Replacing nested cards can lose dirty borders, target registrations, or domain hook identity.
- Summary values can accidentally expose drafts as effective limits or duplicate runtime requests.
- Shared spacing can affect specialized editors. Retain and document their inner geometry.
- Brief descriptions can omit decision-critical exclusions. Keep those visible and move only secondary detail.
- Existing E2E selectors reference card structure. Preserve semantic assertions and migrate affected selectors together.
