---
id: "03-expose-session-choices"
title: "Expose the initial session choice"
status: complete
wave: 3
depends_on:
  - "02-route-explicit-recipients"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.8
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.9
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.10
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.12
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.13
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.6
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.8
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.9
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.11
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.12
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 03: Expose the initial session choice

## Summary

Extend the step selector with the initial-session choice. Move lifecycle
navigation above search and keep it visible while the recipient list scrolls.

## In scope

- Initial-target versus legacy-profile derivation, profile logos, labels, and search.
  Do not expose earlier-step choices or placeholders in this slice.
- Initial-agent description and visible reuse/park/terminal fallback explanation.
- Lifecycle navigation pinned above the list, separate start/end view, and
  trigger summaries that identify explicit targets.
- Save/discard and dirty tracking for target changes, explicit null clearing,
  and failed save preservation.
- Read-only navigation with disabled mutation, conditional-rule exclusion,
  without silently removing existing conditional rules.
- Mobile inset drawer, dynamic viewport and keyboard containment, one scroll
  owner, safe areas, touch sizing, focus return, and desktop keyboard navigation.
- Reuse MobilePickerSheet and add an optional fixed-content slot above scrolling
  children. Preserve existing task, thread, and settings consumers. Keep the
  useResponsiveBreakpoint branch; useTouchDrawer is not an extra required path.
- English, pt-pt, zh-cn, zh-hk, zh-tw, and generated pseudo-locale copy.

## Out of scope

- Source-step targets, reference repair UI, runtime routing, unrelated layout changes.

## Acceptance

- The selector offers initial and legacy profiles without claiming runtime
  sessions already exist; target updates clear conflicting profile values.
- Lifecycle navigation is visible before scrolling a long list and with no
  search matches. Both viewports retain saved values and predictable focus.
- Dirty/save/discard and read-only behavior include target state, while existing
  conditional rules are never silently removed.

## ASCII UI preview

These initial-only excerpts use the plan's UI-01 through UI-03 labels.
Source-step rows arrive in Task 07. These views record the reviewed structure.
Control order, grouping,
fixed lifecycle navigation, and the phone drawer are required. Box dimensions
and spacing are illustrative; use existing UI primitives and localized labels.

### UI-01: Review recipient selector

Entry: open the agent selector in the Review step. Proposed desktop popover:

```text
+--------------------------------------------------+
| Session lifecycle                              > |
| Reuse on start / Park on end                     |
+--------------------------------------------------+
| Search agents or workflow sessions...            |
+--------------------------------------------------+
| WORKFLOW SESSIONS                                |
| (*) Initial agent session                        |
|     Agent selected when the task starts          |
|                                                  |
| AGENT PROFILES                                   |
| ( ) No profile override                          |
| ( ) Claude / Fable                               |
| ( ) Codex / 5.6 Sol                              |
| ( ) Codex / 5.6 Luna                              |
| ...                                              |
+--------------------------------------------------+
```

Before: lifecycle navigation is below all profile rows in the scrolling region.
After: lifecycle navigation and search stay above the single scrolling list.
Search filters choices, never lifecycle navigation. With no matches, the list
shows a localized empty result while lifecycle navigation remains available.
Read-only workflows permit inspection and navigation but disable mutations.

### UI-02: Lifecycle settings

Entry: choose Session lifecycle. Both groups belong to the selected Review step.

```text
+--------------------------------------------------+
| < Back                  Session lifecycle        |
+--------------------------------------------------+
| Target: Initial agent session                    |
|                                                  |
| When this step starts:                           |
| (*) Reuse an available session                    |
|     Continue the original conversation.          |
|     If unavailable, start a fresh one.            |
| ( ) Start a new session                           |
|     Initial agent profile, fresh conversation.   |
|                                                  |
| When this step ends:                             |
| ( ) Complete the session                         |
|     This conversation cannot be reused.          |
| (*) Park the session                             |
|     Stop the agent; keep its conversation.       |
+--------------------------------------------------+
```

Keep the target label visible and return Back to UI-01. Explain that Plan
must park the initial conversation for Review to reuse it. The existing shared
Save changes action persists the draft; the selector adds no local save button.

### UI-03: Phone recipient drawer

Entry: tap the Review agent trigger. The inset bottom drawer uses UI-01's
choices and UI-02's lifecycle view with shared state and handlers.

```text
+----------------------------------+
| Review                           |
| [ Initial agent session       v ]|
|                                  |
|  +----------------------------+  |
|  | Agent session              |  |
|  | Session lifecycle        > |  |
|  | Reuse / Complete           |  |
|  +----------------------------+  |
|  | Search...                  |  |
|  +----------------------------+  |
|  | WORKFLOW SESSIONS          |  |
|  | (*) Initial agent session |  |
|  | AGENT PROFILES             |  |
|  | ...                       |  |
|  +----------------------------+  |
|  | Bottom safe-area spacing  |  |
|  +----------------------------+  |
+----------------------------------+
```

The header, lifecycle entry, and search remain outside the scrolling choices.
Constrain the drawer to the dynamic viewport and keep controls reachable with
the keyboard open. Use 44 px phone hit areas. Back/dismiss returns focus
predictably, and the document has no horizontal overflow.

UI-01 and UI-02 map to AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.8,
001.10, 002.6, and 002.11 (same AC prefix). UI-03 also covers
001.13. The work package's desktop and mobile session-targeting Playwright
scenarios prove structure and prompt ownership; ASCII alone is not evidence.

Preview source: [Plan UI-01 through UI-03](plan.md#ascii-ui-preview).
Keep these view labels and excerpts synchronized with the plan when revised.

## Verification

Use TDD for derivation and changed selector state. If dependencies are absent,
run once from `apps`:

```bash
rtk pnpm install --frozen-lockfile
```

From `apps/web`:

```bash
rtk pnpm exec vitest run lib/workflows/session-target-options.test.ts components/settings/workflow-step-agent-profile-selector.test.tsx components/settings/workflow-dirty-state.test.ts
rtk pnpm exec vitest run components/settings/canvas-host-components.test.tsx components/settings/canvas-host-route.test.tsx
rtk pnpm run typecheck
rtk pnpm exec eslint components/settings/workflow-step-agent-profile-selector.tsx components/settings/workflow-pipeline-editor-panels.tsx components/settings/workflow-step-mutations.ts components/settings/workflow-dirty-state.ts lib/workflows/session-target-options.ts
rtk pnpm exec eslint components/task/mobile/mobile-picker-sheet.tsx
rtk pnpm run i18n:zh-hant
rtk pnpm run i18n:pseudo
rtk pnpm run i18n:check
rtk pnpm run i18n:ratchet
```

Include any extracted selector files in targeted eslint. Rendered desktop and
phone verification belongs to Task 04; this work order alone does not establish
mobile behavior evidence.

## Files likely touched

- `apps/web/components/settings/workflow-step-agent-profile-selector.tsx`
- `apps/web/components/task/mobile/mobile-picker-sheet.tsx` (shared shell slot)
- `apps/web/components/settings/workflow-pipeline-editor-panels.tsx`
- `apps/web/components/settings/{workflow-step-mutations.ts,workflow-dirty-state.ts}`
- The workflow settings draft/save contributor that constructs step updates
- `apps/web/lib/workflows/session-target-options.ts` and its test (new)
- `apps/web/components/settings/workflow-step-agent-profile-selector.test.tsx`
- `apps/web/components/settings/workflow-dirty-state.test.ts`
- `apps/web/src/locales/*/workflows.json`

## Dependencies

Tasks 01 and 02 supply persisted initial targets and working execution semantics.

## Risks

The existing selector disables opening read-only data and puts lifecycle
navigation inside the scrolling surface. Both behaviors require explicit tests.
Preserve all shared-shell consumers when adding the fixed-content slot.

## Parallelism

`sequential`

## Inputs

- Design sections Combined step agent selector and Mobile design contract.
- `mobile-picker-sheet.tsx` fixed-header/scrolling-body exemplar.
- Existing workflow selector tests and shared settings save coordinator.
- `/mobile-parity` and `docs/i18n.md`.

## Results

Implemented the combined profile/session selector, lifecycle navigation above
search and scrolling choices, draft validation, translated copy, and shared
desktop/mobile state. Focused frontend tests passed 36 tests, typecheck and
full web lint passed with no warnings, and all translation gates passed.

A follow-up made the shared desktop/mobile lifecycle surface select **Park the
session** for new or unset policies. Explicitly saved completion remains
selected when reloaded.
