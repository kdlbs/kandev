---
created: 2026-09-10
status: complete
requirements:
  - REQ-UI-MOBILE-CONFIRMATION-001
  - REQ-UI-MOBILE-CONFIRMATION-002
  - REQ-UI-MOBILE-CONFIRMATION-003
system_design:
  - ../../specs/ui/system-design/mobile-action-confirmations.md
legacy_specs: []
---

# Implementation Plan: Mobile Action Confirmations

## Overview

Give phone archive and existing inline confirmations a focused surface. Build
the shared presentation and host first, connect archive as the first complete
flow, then migrate picker, content, saved-artifact, and management actions.
Execute work orders 01 through 06 sequentially. Each adoption owns its tests;
there is no separate generic QA or verification phase.

The [requirements](../../specs/ui/requirements/mobile-action-confirmations.md),
[system design](../../specs/ui/system-design/mobile-action-confirmations.md),
and [decision](../../decisions/2026-09-10-mobile-confirmation-surfaces.md) form the
design package. Existing UI contracts were reconciled for the intended phone
presentation. Implementation was explicitly requested on 2026-09-10.

## Scope

### In scope

- A confirmation step inside the open Tasks/picker/filter sheet, and compact
  standalone bottom confirmation from a page.
- Phone archive across rows, Kanban, bulk, detail flows, and command panel;
  existing inline actions across the inventory below.
- State/scroll/focus restoration, localized content, safe areas, accessible
  actions, stale-request cancellation, and existing domain callback behavior.
- Focused component and production-build mobile/desktop regression evidence.

### Out of scope

- Backend, cleanup, recovery, Undo, permissions, API/SDK, persistence, or
  confirmation-preference changes.
- Desktop/tablet composition changes or a global responsive-menu rewrite.
- Existing full task-delete/discard/type-to-confirm/system maintenance alerts.
- Bespoke external-plugin UI, broad refactoring, broad test audits, publication,
  or delegated implementation.

## Technical approach

### Shared presentation

Add `MobileConfirmationContent`, `MobileActionConfirmation`, and
`MobileConfirmationHost` under `apps/web/components/confirmation/`. Reuse
`@kandev/ui/drawer` and the application's `useResponsiveBreakpoint`. Host
registration is local, source-keyed, and cleans up after commit. Keep the origin
mounted but hidden/inert beside its confirmation outlet; never hide a portal
inside its own hidden ancestor. Keep one active modal/focus boundary.

Add opt-in hosting to `MobilePickerSheet`; register other hosts at actual
drawer/dialog owners. Do not change the global semantics of
`InlineConfirmActions`, `ActionConfirmPopover`, `Dialog`, or `Drawer`.

### Archive integration

Share archive content/state between its non-phone alert and phone renderer.
Adapt `TaskArchiveConfirmation`, `TaskArchiveConfirmDialog`,
`TaskArchiveConfirmFlow`, the row adapter, `SessionTaskSwitcherSheet`, and the
command-panel owner. Phone routing wins over old `inline`/`forceDialog` hints;
desktop classification gating and tablet behavior remain unchanged. Use the
same preference, cleanup model, cascade values, and domain navigation.

### Consumer inventory and ownership

Paths below are under `apps/web/components/`. Each row is an adoption boundary,
not permission to rewrite the owning domain.

| Work order | Adapters and owned mobile hosts |
| --- | --- |
| 02 | `task/task-archive-confirmation.tsx`, `task/task-archive-confirm-dialog.tsx`, `task/task-archive-confirm-flow.tsx`, `task/task-switcher-archive-confirmation.tsx`, `task/task-switcher-context-menu.tsx`, `task/mobile/session-task-switcher-sheet.tsx`, `kanban/task-multi-select-toolbar.tsx`, command-panel confirmation owner |
| 03 | `task/terminal-close-inline-confirmation.tsx`, `task/mobile/mobile-terminals-section.tsx`, `task/mobile/mobile-sessions-section.tsx`; terminal picker host through `mobile-picker-sheet.tsx` |
| 04 | `task/file-browser-parts.tsx`, `task/file-context-menu.tsx`, `task/chat/reset-context-button.tsx`, `task/task-plan-revision-restore-actions.tsx`, `task/task-detach-confirm-dialog.tsx`, `review/walkthrough-overlay.tsx`; existing file/plan/review parent surfaces |
| 05 | `confirmation/saved-task-view-delete-confirmation.tsx`, task sidebar-filter and Threads view hosts, GitHub/GitLab preset sidebars and filter hosts, `integrations/presets-scope-bar-base.tsx`, `jira/my-jira/list-toolbar.tsx` |
| 05 | `settings/prompt-delete-confirmation.tsx`, `settings/layouts/layout-profile-delete-confirmation.tsx`, `settings/agent-profile-delete-dialog.tsx`, `task/layout-preset-selector.tsx`, `task/saved-layout-delete-confirmation.tsx` and their owning surfaces |
| 06 | `watches/watcher-delete-action.tsx`, `jira/jira-action-bar.tsx`, `linear/linear-settings.tsx`, `azure-devops/azure-devops-settings.tsx`, `sentry/sentry-instance-card.tsx` |
| 06 | `settings/plugins/uninstall-plugin-dialog.tsx`, `settings/system/users-table.tsx`, `settings/secrets-list-item-row.tsx`, `settings/workflow-sync-dialog.tsx`; existing form-dialog host for sync removal |

Before each adoption, search its mobile E2E selectors for the old inline
container/role. Replace interactions with the new surface without deleting
outcome, focus, geometry, or failure assertions. Preserve existing test IDs
where meaningful; use `data-legacy-testid` if an ID must be renamed. Read scoped
`AGENTS.md` for Threads, review, and chat before editing those subtrees.

### Content, documentation, and compatibility

Use explicit titles/subjects and the current localized consequences. Archive
uses the current non-destructive full-dialog variant; true delete actions use
destructive styling. New labels must reach all five real catalogs plus pseudo.
Each adopting work order owns its needed domain copy and catalog updates.

Phone interaction guidance is recorded in `apps/web/AGENTS.md` and the how-to
in `docs/public/mobile-remote-access.md`, linked from the archive section of
`docs/public/tasks-and-workflows.md`. `/docs-maintainer` identified that public
documentation impact during planning; the public text was updated alongside
implementation. Requirements and system-design status now describe the
implemented contract.

## Tests

Methods below name required scenarios; new test files are explicitly noted.
The work orders contain the exact commands and TDD sequence.

| Acceptance criteria | Unit/component evidence |
| --- | --- |
| 001.2-.7; 002.3-.7; 003.1-.2, .5 | New `mobile-confirmation-host.test.tsx`: preserves form/scroll owner, restores focus, cancels one step, clears stale registrations. New `mobile-action-confirmation.test.tsx`: standalone/hosted routing, boundary cancellation, captured target, double activation, rejection after dismissal. New `mobile-confirmation-content.test.tsx`: semantics, action variants, disabled state, live locale. |
| 001.1; 002.1-.2; 003.3-.4, .6 | `task-archive-confirmation.test.tsx`, `task-archive-confirm-dialog.test.tsx`, `session-task-switcher-sheet.test.tsx`, `use-task-archive-confirm.test.ts`: phone routing, disabled/failed classification, preference bypass, cascade/bulk values, shared warnings and navigation. |
| 001.1-.5; 003.1-.2, .5-.6 | `mobile-sessions-section.test.tsx` and new `terminal-close-inline-confirmation.test.tsx`: named target, same-host Cancel, duplicate prevention, no wait for teardown. |
| 001.1-.5; 002.1-.2; 003.1-.2, .5-.6 | Existing file-context-menu, reset-context-button, detach and walkthrough tests plus new revision-restore tests: source-target isolation, menu handoff, owner feedback, protected actions. |
| 001.1-.5; 002.1-.2, .6; 003.1-.2, .5-.6 | Saved-view shell/host tests and existing agent-profile/layout/prompt tests: exact IDs, no select/default action, built-in/last-view guard, preserved editor state, current rollback. |
| 001.1-.5, .7; 003.1-.2, .5-.6 | Existing provider-settings, secrets, users, plugin-uninstall and workflow-sync tests: one mutation, current failure feedback, removal-form retry without a second modal. |

The abbreviated AC suffixes in these tables refer to
`AC-UI-MOBILE-CONFIRMATION-<group>.<criterion>`. Work-order frontmatter lists
full IDs. Retained domain requirements are linked from each relevant order.

## E2E tests

| Flow and required outcome | AC groups | Evidence on `mobile-chrome` |
| --- | --- | --- |
| Scroll Tasks, open archive, see one active surface, Cancel/Back restores exact context; archive non-active/active/parent/bulk tasks; dismiss delayed classification and reject stale results | 001, 002, 003 | New `tests/task/mobile-action-confirmations.spec.ts`; existing mobile archive preference, redirect, and Kanban card specs |
| Standalone archive geometry at 320px/767px and landscape; long names, pseudo/Portuguese, light/dark; hit-test footer; rotate/breakpoint dismissal; task-delete stays centered | 001.3, .6-.7; 002; 003.2-.4 | New mobile action-confirmation scenarios and updated `tests/task/mobile-confirmation-text-hierarchy.spec.ts`, `tests/kanban/mobile-card-archive-confirmation.spec.ts` |
| Close terminal and use sibling before teardown; cancel/delete session with shared warnings and later failure feedback | 001.2-.5; 002; 003.5-.6 | `tests/terminal/mobile-terminal-close.spec.ts`, `tests/session/mobile-session-deletion.spec.ts` |
| Delete file, reset context, restore plan, detach task, and delete walkthrough through their real phone entry points | 001; 002; 003.1-.2, .5-.6 | Existing mobile file/reset/plan specs plus new `tests/task/mobile-content-confirmations.spec.ts` |
| Delete saved sidebar/Threads/integration views without selecting them; cancel restores editor values; delete prompt/layout/profile | 001; 002; 003.1-.2, .5-.6 | Existing mobile sidebar/Threads/integration saved-view, prompt/profile/layout specs |
| Remove integration/watch, secret, user, plugin and workflow-sync configuration; cancellation and owner error paths remain usable | 001; 002; 003.1-.2, .5-.6 | Existing mobile integration/watch/secret specs and new `tests/settings/mobile-management-confirmations.spec.ts` |

Use existing isolated `test-base` fixtures, seed via API, assert outcomes via
UI and persistence where relevant. Restore changed settings after each case.
Use causal transport waits and finite-animation settlement. Keep mobile files
named `mobile-*.spec.ts`; run desktop separately with `chromium`. A separate
touch-enabled 768px scenario inside the focused desktop regression verifies
the tablet boundary without changing the mobile project's device fixture.

## Work orders

| Order | Work order | Status | Dependency |
| --- | --- | --- | --- |
| 01 | [Shared mobile surfaces](task-01-shared-mobile-surfaces.md) | done | None |
| 02 | [Archive surfaces](task-02-archive-surfaces.md) | done | 01 |
| 03 | [Session and terminal pickers](task-03-picker-confirmations.md) | done | 01 |
| 04 | [Task content actions](task-04-content-confirmations.md) | done | 01 |
| 05 | [Saved artifact deletion](task-05-saved-artifact-confirmations.md) | done | 01 |
| 06 | [Management removals](task-06-management-confirmations.md) | done | 01 |

All orders are sequential because shared adapters/catalogs and test helpers can
overlap. Dependencies express prerequisites, not authorization for delegation.
All six orders were implemented in the primary session after the user's
explicit implementation request. Their results are recorded in the work orders.

## Verification results

All six work orders are complete. No production or permanent test files changed
during the design-only turn; implementation followed the explicit user request.

- Final combined affected-component run: **360 tests passed across 46 files**.
  Earlier per-order RED/GREEN evidence remains in each work order.
- Browser scenarios passed across the focused runs:

  | Adoption | Phone scenarios | Desktop/tablet regressions |
  | --- | --- | --- |
  | Archive | 12 | 11 |
  | Session/terminal pickers | 2 | 1 |
  | Task content and Quick Chat | 7 | 2 |
  | Saved views/profiles/layouts/prompts | 10 | 7 |
  | Integration and system management | 10 | 7 |

- Inspected phone captures cover same-host and standalone composition, narrow
  translated text, dark/light themes, landscape pseudo, long file names,
  saved-view/profile deletion, integration removal, and workflow form removal.
  Full task-delete and existing conflict alerts retain their original safeguards.
- All 72 changed web production files pass ESLint with zero warnings. Typecheck,
  the production E2E build, changed TS/TSX Prettier check, i18n validation and
  the new-code ratchet pass. Existing build chunk-size/deprecation warnings
  remain non-blocking and unrelated to this change.
- Public documentation: 46 pages validated and the validator test passed.
  All specification files and all 30 specification-linter tests passed.
- `git diff --check` passed. No new translation keys, backend/API/schema,
  permission, transport, cleanup-policy or plugin-runtime changes were needed.
- Every remaining legacy inline consumer is accounted for in work order 06;
  retained standalone popover/full-alert exceptions are explicitly recorded.
- Test coverage follows real existing flows: the authenticated mobile user
  guard stays in its auth spec, and detach/walkthrough checks reuse their
  established fixtures. No persistent platform workers or tasks were created.
- The existing ADR index contains a pre-existing unrelated missing link to
  `2026-09-05-script-capable-html-preview-isolation.md`; the new decision link
  resolves. No unrelated decision history was changed.

## Risks

- Hiding/unmounting the origin can destroy the request or lose scroll/editor
  state. A host outlet must be outside the hidden origin and preserve ownership.
- Radix/Vaul focus and close events can dismiss two layers or refocus a removed
  menu item. Test exact menu-to-host transfer and nested Escape behavior.
- Current callers differ in callback close timing and retry behavior; a generic
  awaited mutation wrapper would regress terminal close and optimistic actions.
- Archive classification, live row removal, and breakpoint changes can deliver
  late effects. Token/target invalidation must prevent a stale submission.
- The screenshot's text can lag current cleanup policy. Use current domain
  content; do not bake branch-deletion claims into the new visual component.
