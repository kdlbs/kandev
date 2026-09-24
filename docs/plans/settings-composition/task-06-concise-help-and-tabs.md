---
id: "06-concise-help-and-tabs"
title: "Concise Task behavior help and header tabs"
status: complete
wave: 6
depends_on:
  - 05-system-account
plan: "plan.md"
requirements:
  - REQ-UI-SETTINGS-COMPOSITION-002
  - REQ-UI-SETTINGS-COMPOSITION-003
  - REQ-UI-SETTINGS-COMPOSITION-004
  - REQ-UI-SETTINGS-COMPOSITION-005
acceptance_criteria:
  - AC-UI-SETTINGS-COMPOSITION-002.1
  - AC-UI-SETTINGS-COMPOSITION-002.2
  - AC-UI-SETTINGS-COMPOSITION-002.3
  - AC-UI-SETTINGS-COMPOSITION-002.4
  - AC-UI-SETTINGS-COMPOSITION-002.5
  - AC-UI-SETTINGS-COMPOSITION-002.6
  - AC-UI-SETTINGS-COMPOSITION-003.1
  - AC-UI-SETTINGS-COMPOSITION-003.2
  - AC-UI-SETTINGS-COMPOSITION-003.3
  - AC-UI-SETTINGS-COMPOSITION-003.4
  - AC-UI-SETTINGS-COMPOSITION-003.5
  - AC-UI-SETTINGS-COMPOSITION-003.6
  - AC-UI-SETTINGS-COMPOSITION-004.1
  - AC-UI-SETTINGS-COMPOSITION-004.2
  - AC-UI-SETTINGS-COMPOSITION-004.3
  - AC-UI-SETTINGS-COMPOSITION-004.4
  - AC-UI-SETTINGS-COMPOSITION-004.5
  - AC-UI-SETTINGS-COMPOSITION-005.1
  - AC-UI-SETTINGS-COMPOSITION-005.2
  - AC-UI-SETTINGS-COMPOSITION-005.3
  - AC-UI-SETTINGS-COMPOSITION-005.4
  - AC-UI-SETTINGS-COMPOSITION-005.5
  - AC-UI-SETTINGS-COMPOSITION-005.6
  - AC-UI-SETTINGS-COMPOSITION-005.7
system_design:
  - ../../specs/ui/system-design/settings-composition.md
---

# Task 06: Concise Task behavior help and header tabs

## Summary

Replace the long Task behavior page with Tasks, Conversation, and Runtime header tabs.
Keep all sections expanded and use one short description plus an info button for each setting.
This work order supersedes the collapsed-runtime direction from the original delivery.

## In scope

- Reuse the Data & Logs tab composition and preserve settings links and cross-tab drafts.
- Add shared desktop-hover/keyboard and touch-drawer information presentation.
- Apply the copy inventory to every Task behavior setting, including compound runtime fields and profile choices.
- Remove repeated introductions, the collapsed runtime summary, and redundant inline technical paragraphs.
- Retain essential inline restrictions, current errors, and managed-value notices.
- Update all six real locales, pseudo generation, affected tests, and documentation.

## Out of scope

- New pages, sidebar entries, or backend settings behavior.
- Changing switch polarity, profile fallback logic, limits, or permissions.
- A copy rewrite of unrelated settings pages. The shared info component remains reusable there.
- Third-party plugin content or a new global tooltip system.

## Acceptance

- Tabs preserve values, Save/Reset, error recovery, and existing search fragments. Runtime needs no expansion action.
- Every Task behavior setting has one concise visible description and keyboard/touch-accessible optional information.
- Desktop and phone checks prove the new composition and existing persistence behavior without weakening prior regression assertions.

## Visible-copy inventory

These are starting English strings, not new backend semantics. Keep existing stable keys when their meaning remains valid.
Descriptions wrap naturally. Do not shrink fonts, truncate text, or enforce a one-line height.
Retain unique details from current strings in info content. Remove repeated statements rather than duplicating them in multiple popups.

| Setting | Short description | Info content |
| --- | --- | --- |
| Open new tasks automatically | Open a task after you create it. | Staying on the current view does not prevent task or agent start. |
| Agent-generated titles | Let the agent name new tasks from their prompt. | Required prompt, provisional name, rename fallback, manual edits, and save timing. |
| Profile for agent-created tasks | Choose the default profile for tasks created by agents. | Explicit/workflow precedence, effective creating-session values, parent fallback, scope, and cost implications. |
| Prevent auto-start on open | Keep agents stopped when reopening tasks after a restart or from the final workflow step. | Start agent remains available. Preserve the current switch meaning. |
| Archive confirmation | Review cleanup options before archiving a task. | Existing cleanup, subtask, and confirmation behavior. |
| Unread divider | Mark messages received while a task was out of view. | Read tracking continues when the visual divider is off. |
| Anchored prompt | Keep your last prompt visible while scrolling on desktop. | Trigger conditions and desktop-only scope. |
| Jump to last prompt | Show a button to return to your latest prompt. | Existing visibility threshold and navigation behavior. |
| Jump to start | Show a button to return to the start of the conversation. | Existing scroll threshold and destination. |
| Auto-scroll control | Show a button to pause or resume transcript auto-scroll. | Per-session behavior and what this preference changes. |
| Todo checklist | Show the agent's todo checklist in the task panel. | Desktop panel placement and manual panel additions. |
| Only show nonempty checklist | Hide the pinned checklist when it has no items. | Dependency on the checklist toggle and retained saved value. |
| Enable automatic-session limit | Queue automatic starts when the session limit is reached. | Instance scope and interaction with workflow WIP limits. |
| Maximum automatic sessions | Manual starts can exceed this limit. | Positive-integer constraint, effective source, and automatic-start behavior. |
| Maximum queued messages | Limit pending messages per session. Use 0 for unlimited. | Admission timing, existing queue preservation, retries, and override precedence. |
| Allow queued message merging | Combine related queued messages before the agent reads them. | Sender, adjacency, workflow/system exclusions, and entity-reference limit. |
| Automatic message merging | Merge compatible consecutive messages automatically. | Session override, compatibility checks, and new-admission-only behavior. |
| Keep host awake | Prevent the computer running Kandev from sleeping during tasks. | OS mechanism, release timing, remote host scope, and platform limitations. |

Profile option descriptions become “Reuse the creating session's profile” and “Use the workspace's default profile”.
Keep the full precedence rules in the setting's info content, including cases where workflow profiles win.
Avoid changing technical wording by inference. Compare retained information with the existing profile and runtime source contracts.

## ASCII UI preview

### UI-05 / UI-06 / UI-07: Concise settings

```text
Task behavior          [Tasks] [Conversation] [Runtime]
Group heading
+--------------------------------------------------+
| Setting label (i)                       [control]|
| One short description.                           |
+--------------------------------------------------+
Runtime: all limit, queue, and power sections expanded.
Phone: tabs below title; tap (i) opens a details drawer.
```

See the [full previews](plan.md#ascii-ui-preview).
Tab labels, expanded groups, optional information, and preserved Save are required.
The applicable criteria are listed in frontmatter.

## Technical notes

Use `SettingsPageHeader`, `SettingsTabs`, `SettingsTabsList`, `SettingsTabsPanel`, and `useSettingsTab`.
Keep visited panels mounted. Use the existing target-to-tab mapping pattern from Data & Logs.
Keep both queue/session state owners single-mounted and preserve their contributor IDs.
Aggregate panel dirty/error status without creating duplicate save contributors.

Add `SettingsInfo` beside the label in `SettingsRow`, outside the associated label element.
Reuse `Tooltip`, `Drawer`, and `useTouchDrawer`. Use the sleep info control as a reference.
Keep control helpers associated through `aria-describedby`. Info has its own localized accessible name.
Use short noninteractive technical content on desktop and the same content in a scrollable phone drawer.

Remove runtime disclosure props and obsolete runtime-only attention/summary helpers after checking all callers.
Keep general disclosure discovery support used by other settings surfaces.
Update scoped self-documenting-settings guidance to distinguish short inline effects from optional implementation details.
Current permission, validation, and managed-value notices remain inline.

## Verification

Start from the repository root. Dependencies already exist in this worktree.
Write focused tests before behavior changes. The two concise-help E2E files below are new outputs of this work order.

```bash
(cd apps/web && pnpm exec vitest run components/settings/settings-info.test.tsx components/settings/settings-group.test.tsx components/settings/settings-tabs.test.tsx components/settings/task-behavior-settings.test.tsx components/settings/task-behavior-tabs.test.ts components/settings/settings-save-provider.test.tsx components/settings/mcp-task-agent-profile-default-settings.test.tsx components/settings/sleep-inhibition-settings.test.tsx hooks/domains/settings/use-settings-tab.test.ts lib/settings-discovery/target.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm exec eslint components/settings hooks/domains/settings/use-settings-tab.ts lib/settings-discovery --max-warnings 0)
(cd apps/web && pnpm e2e:run e2e/tests/settings/task-behavior-concise-help.spec.ts e2e/tests/settings/settings-composition.spec.ts e2e/tests/settings/settings-manual-save.spec.ts e2e/tests/system/message-queue-settings.spec.ts e2e/tests/system/session-capacity-settings.spec.ts e2e/tests/settings/todo-list-panel.spec.ts e2e/tests/task/mcp-task-agent-profile-default.spec.ts e2e/tests/task/unread-divider-preference.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/settings/mobile-task-behavior-concise-help.spec.ts e2e/tests/settings/mobile-settings-composition.spec.ts e2e/tests/system/mobile-message-queue-settings.spec.ts e2e/tests/system/mobile-session-capacity-settings.spec.ts e2e/tests/settings/mobile-general-settings.spec.ts e2e/tests/task/mobile-mcp-task-agent-profile-default.spec.ts e2e/tests/task/mobile-unread-divider-preference.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Also run every existing test whose selector or navigation path changes and record its exact command.
Use `rg` to inventory tests opening Task behavior before editing them, including task, workflow, and kanban suites.
Replace the old `openTaskBehaviorRuntime` helper's expansion action with selection of the visible Runtime tab.
Conversation-specific tests must select Conversation. Preserve their value and persistence assertions.
The managed E2E runner builds fresh artifacts. Do not overlap suites or bypass its resource limits.

Required scenarios:

- Tasks default, explicit Runtime query, unknown-query fallback, and every existing target-to-tab mapping.
- Repeated discovery into a previously visited hidden panel, with actual focus after activation.
- Edit across tabs, Save/Reset all contributors, preserve drafts on tab change, and reveal a failed inactive-tab save once.
- Inline load/permission/managed-value errors, without popup-only recovery instructions.
- Runtime controls visible immediately after tab selection. No collapsed settings sections.
- Hover and keyboard focus reveal desktop help. Touch opens the drawer; dismissal returns focus and preserves the draft.
- Real 44px hit targets, 767px/768px geometry, coarse-pointer tablet, long translations, and no horizontal document overflow.
- Concise visible text. MCP identifiers and fallback algorithms appear only in optional info.
- Current manual-save, capacity, queue, title, profile, and conversation assertions remain intact.

## Files likely touched

- `apps/web/components/settings/task-behavior-settings.tsx` and its tests
- `apps/web/components/settings/settings-group.tsx` and its tests
- `apps/web/components/settings/settings-info.tsx` and `.test.tsx` (new)
- Existing creation, title, profile, opening, archive, unread, transcript, todo, and sleep settings components
- `apps/web/components/settings/system/message-queue-settings.tsx` and `session-capacity-settings.tsx`
- `apps/web/lib/settings-discovery/catalog/preferences.ts`
- `apps/web/e2e/helpers/settings-composition.ts` and all affected route consumers
- New concise-help E2E files and existing composition/runtime tests
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,pseudo}/`
- `apps/web/AGENTS.md`, affected Task behavior sections under `docs/public/`, and this package

## Dependencies

Tasks 01 through 05 are complete. Reuse their implementation and review fixes.

## Risks

- Hidden visited panels can consume discovery focus too early.
- Tab changes can lose drafts if domain owners unmount.
- Shortening copy can erase scope or precedence facts. Preserve them in info and retain active restrictions inline.
- Hover-only help can exclude phones or keyboard users.
- Tests can continue asserting old disclosure behavior unless all route consumers are audited.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/settings-composition.md)
- [System design](../../specs/ui/system-design/settings-composition.md)
- [Header tabs contract](../../specs/ui/system-design/settings-header-tabs.md)
- Existing Data & Logs composition and sleep-prevention tooltip/drawer
- `/tdd`, `/mobile-parity`, `/e2e`, and `apps/web/AGENTS.md`

## Results

Implemented on 2026-09-22. Tasks, Conversation, and Runtime reuse the header tabs and retain visited drafts.
Every Task behavior control has concise copy and optional desktop/touch information.
Runtime sections stay expanded. The existing save coordinator exposes contributor status for tab badges and one-time error recovery.
Discovery skips optional info buttons and focuses the setting after tab activation.

Validation:
- Focused Vitest suite: 10 files, 63 tests passed, including save-coordinator state, Reset recovery, and all nine discovery mappings.
- Desktop regression suite: 37 passed, including composition, manual save, cross-tab Save/Reset/failure, queue/capacity, todo, MCP profile, and unread preference.
- Mobile regression suite: 22 passed, including 390/767/768/900px touch geometry, long pseudo translations, drawer focus return, runtime permissions, profile, and unread preference.
- Typecheck, scoped ESLint, six-locale/pseudo checks, documentation catalog/spec lint, and harness checks passed.
- Final targeted browser reruns: 14 desktop and 7 mobile tests passed against fresh production artifacts, including the final profile markup and capacity description association.

RED tests reproduced missing info interaction, tab ownership, contributor status, stale failure after Reset, and discovery focusing the info button.
Browser regressions also caught outdated selectors and tooltip-copy assertions, which now use the intended tab/help flow without removing persistence or permission checks.
The Traditional Chinese generator ran with `--namespace settings`; its unrestricted run found a pre-existing residual Simplified Chinese string in `workflows.openAgentSettings` and refused to write that unrelated namespace.


Final browser reruns (from `apps/web`):

```bash
pnpm e2e:run e2e/tests/settings/task-behavior-concise-help.spec.ts e2e/tests/system/message-queue-settings.spec.ts e2e/tests/system/session-capacity-settings.spec.ts e2e/tests/task/mcp-task-agent-profile-default.spec.ts
pnpm e2e:run --no-build --project mobile-chrome e2e/tests/settings/mobile-task-behavior-concise-help.spec.ts e2e/tests/system/mobile-session-capacity-settings.spec.ts e2e/tests/task/mobile-mcp-task-agent-profile-default.spec.ts
```

The second command reused the immediately preceding fresh build. No source changed between these runs.
The final focused Vitest rerun passed all 63 tests after the last production edit.
Public docs are reference/how-to navigation updates; the obsolete all-in-one Task Behavior screenshot was removed from the operations guide.
Harness validation passed (19 harness tests, 36 specification-linter tests, full harness lint, and the targeted `harness-lint` hook).
The frontend guide shrank from 300 to 297 lines.
