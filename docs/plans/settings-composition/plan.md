---
created: 2026-09-21
status: implemented
requirements:
  - REQ-UI-SETTINGS-COMPOSITION-001
  - REQ-UI-SETTINGS-COMPOSITION-002
  - REQ-UI-SETTINGS-COMPOSITION-003
  - REQ-UI-SETTINGS-COMPOSITION-004
  - REQ-UI-SETTINGS-COMPOSITION-005
system_design:
  - ../../specs/ui/system-design/settings-composition.md
legacy_specs: []
---

# Implementation Plan: Settings Composition

## Overview

Revision on 2026-09-22: the user rejected collapsed Runtime and excessive inline text.
Task 06 adds Tasks, Conversation, and Runtime header tabs with expanded sections and concise help.
The completed Tasks 01 through 05 and their results remain historical delivery evidence.
Task 06 supersedes their collapsed-section and long-description presentation.


Keep Task behavior on one page and use consistent composition across first-party settings.
Deliver shared groups with Task behavior first, then migrate bounded route families.
Use the [surface inventory](surface-inventory.md) to prevent partial coverage.
Each work order includes its own rendered and behavioral checks.

## Scope

### In scope

- Three Task behavior header tabs, expanded sections, and short descriptions with optional info.
- Shared card padding, row spacing, descriptions, fields, actions, and disclosure treatment.
- All first-party settings route families and the forms they expose.
- Localization, discovery compatibility, dirty-state preservation, phone composition, and relevant documentation.

### Out of scope

- New settings pages or sidebar entries. Tabs are limited to Task behavior.
- Backend changes, new permissions, altered defaults, or auto-start switch inversion.
- Global UI Card styling and third-party plugin content.
- Specialized editor, table, terminal, and pipeline redesigns.

## Inputs and decisions

The user retained one page and requested less visible text, no collapsed settings sections, and evaluation of the Data & Logs tab pattern.
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

Task 06 reuses the existing header tabs and adds shared info presentation.
It removes Runtime collapse and redundant descriptions without changing domain settings behavior.
Its copy table, target mapping, mobile preview, and exact checks are in the work order.

The following describes the original delivery:


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

### UI-05: Concise Task behavior, desktop

Entry: `/settings/preferences/task-behavior`, default Tasks tab.

```text
Task behavior                 [Tasks] [Conversation] [Runtime]
-------------------------------------------------------------
Creating and opening tasks
+-----------------------------------------------------------+
| Open new tasks automatically (i)                  [switch] |
| Open a task after you create it.                           |
|-----------------------------------------------------------|
| Agent-generated titles (i)                        [switch] |
| Let the agent name new tasks from their prompt.            |
|-----------------------------------------------------------|
| Profile for agent-created tasks (i)                       |
| Choose the default profile for tasks created by agents.   |
| (o) Creating session: reuse the creating session's profile.|
| ( ) Workspace default: use the workspace's default profile.|
|-----------------------------------------------------------|
| Prevent auto-start on open (i)                    [switch] |
| Keep agents stopped when reopening tasks after a restart  |
| or from the final workflow step.                          |
+-----------------------------------------------------------+
Archiving
[Archive confirmation row with short help and (i)]
```

Tasks contains creation/opening and archiving. Conversation contains transcript and panel preferences.
Info appears beside the setting label. There is no repeated section-description paragraph.
Hover/focus on `(i)` reveals technical details without changing the setting.

### UI-06: Runtime, desktop

Entry: the visible Runtime tab or an existing runtime search fragment.

```text
Task behavior                 [Tasks] [Conversation] [Runtime*]
-------------------------------------------------------------
Applies to all workspaces.
Session capacity
[Enable automatic-session limit (i)              switch]
[Maximum automatic sessions (i)                     5  ]
 Manual starts can exceed this limit.
Message queue
[Maximum queued messages (i)                       10  ]
 0 means unlimited.
[Allow queued message merging (i)                switch]
[Merge consecutive messages automatically (i)    switch]
Power
[Keep host awake (i)                             switch]
 Prevent the computer running Kandev from sleeping during tasks.

[Inline managed-value notice or error only when applicable]
           Unsaved changes       [Reset] [Save changes]
```

Every group is expanded. No chevrons, expansion summaries, or Details accordions precede the settings.
The floating Save surface appears only while dirty and applies across tabs.
A new save/validation error in an inactive tab reveals that tab once. Load failures mark the tab without interrupting active edits.

### UI-07: Phone tabs and info

```text
< Settings
Task behavior
[Tasks] [Conversation] [Runtime]
--------------------------------
Agent-generated titles (i) [on]
Let the agent name new tasks
from their prompt.

Tap (i):
+------------------------------+
| Agent-generated titles  [X]  |
| Prompt requirements, naming  |
| fallback, and manual editing |
| details.                     |
+------------------------------+
```

Phone tabs occupy their own header row. Settings keep one vertical scroll owner.
Info opens an inset drawer with safe-area clearance and 44px targets. Long details scroll inside the drawer.
Dismissal returns focus to the info button. It never changes the draft.
The desktop tooltip and phone drawer use identical content.
Required structure: three visible tabs, expanded sections, concise row descriptions, and optional accessible information.
Copy and ASCII spacing are illustrative. The work-order copy table is the English starting point.
Maps to AC-UI-SETTINGS-COMPOSITION-002.1 through .6, -003.1 through .6, -004.1 through .5, and -005.1 through .7.

## Tests and E2E

Task 06 owns the exact commands and scenarios for the refinement.
Its component tests cover info accessibility, cross-tab draft lifetime, error routing, and discovery ordering.
New desktop/mobile concise-help suites prove visible runtime controls, short descriptions, info interactions, and tab navigation.
Existing composition, save, queue, and capacity suites retain their behavioral assertions with updated navigation.
The work order also requires every modified existing E2E consumer to run and record results.

## Work orders

- [x] [Task 01: Shared composition and Task behavior](task-01-task-behavior.md)
- [x] [Task 02: Remaining preferences](task-02-preferences.md)
- [x] [Task 03: Agents and execution settings](task-03-agent-executor.md)
- [x] [Task 04: Workspace and integration settings](task-04-workspace-integration.md)
- [x] [Task 05: System, account, and plugin settings](task-05-system-account.md)

- [x] [Task 06: Concise help and header tabs](task-06-concise-help-and-tabs.md)

Tasks 01 through 05 are complete. Task 06 follows them sequentially. No delegation is authorized.

## Verification results

Revision design checks on 2026-09-22: specification catalog validation, full specification lint, relative links, acceptance references, and whitespace checks passed.
No application builds or tests ran during this design turn.


Task 06 refinement is implemented. Its work order records current validation; results below describe the original delivery.

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

- Switching tabs can hide invalid controls or break search focus unless panel activation precedes focus.
- Replacing nested cards can lose dirty borders, target registrations, or domain hook identity.
- Effective values must remain distinct from drafts without duplicate runtime requests.
- Shared spacing can affect specialized editors. Retain and document their inner geometry.
- Brief descriptions can omit decision-critical exclusions. Keep those visible and move only secondary detail.
- Existing E2E selectors reference card structure. Preserve semantic assertions and migrate affected selectors together.
