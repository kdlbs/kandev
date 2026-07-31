---
spec: docs/specs/ui/add-panel-pr-submenu.md
created: 2026-07-31
status: draft
---

# Implementation Plan: Task Add-Panel PR Submenu

## Overview

The task-view "+" add-panel menu (`AddPanelMenuItems`) currently renders every
linked GitHub PR as a flat `DropdownMenuItem`. For tasks with many PRs this
makes the menu too tall. The change collapses the PR rows into a Radix
`DropdownMenuSub` ("Pull requests") whenever more than one PR is linked,
keeping the single-PR inline row and the per-PR test ids unchanged. E2E covers
both the desktop flow (existing multi-PR spec must open the submenu first) and a
new mobile bottom-sheet spec, since `app/globals.css` already applies nested
`dropdown-menu-sub-content` bottom-sheet styling below 640px.

Implementation is a pure frontend change in one component plus tests. No backend
or state changes. Order: component + unit test first, then E2E.

---

## Frontend

### `apps/web/components/task/dockview-add-panel-items.tsx`

- Extract the PR section into a small `PRPanelMenuItems` sub-component taking
  `{ prs: TaskPR[]; onOpenPR: (pr: TaskPR) => void }` so it stays under the
  file/function lint limits and is unit-testable without store hooks.
- Behavior:
  - `prs.length === 0` → render `null`.
  - `prs.length === 1` → render the current inline `DropdownMenuItem`, label
    `prPanelLabel(pr.pr_number)` (`PR #N`), testid
    `add-panel-pr-item-${prIdentitySlug(pr)}`.
  - `prs.length > 1` → render
    `<DropdownMenuSub><DropdownMenuSubTrigger>` labeled **Pull requests** with
    `IconGitPullRequest`, testid `add-panel-pr-submenu`, and a
    `<DropdownMenuSubContent>` listing one row per PR. Each row keeps the
    current disambiguated label `${prPanelLabel(pr.pr_number)} — ${pr.repo}`
    and the same `add-panel-pr-item-${prIdentitySlug(pr)}` testid.
  - Add `max-h`/`overflow-y-auto` on the sub-content so a 10-PR list cannot
    exceed the viewport on short screens.
- `AddPanelMenuItems` replaces the inline `.map` with
  `<PRPanelMenuItems prs={state.prs} onOpenPR={(pr) => addPRPanel(prTaskKey(pr), activeSessionId)} />`.
- Imports: add `DropdownMenuSub`, `DropdownMenuSubTrigger`,
  `DropdownMenuSubContent` from `@kandev/ui/dropdown-menu`.
- GitLab MR rows stay inline (out of scope per spec).

### Store / state

None. `addPRPanel`, `prTaskKey`, `prIdentitySlug`, `prPanelLabel`, and
`activeSessionId` are all reused unchanged.

---

## Tests

- **What:** one PR renders inline; no submenu trigger.
  **File:** `apps/web/components/task/dockview-add-panel-items.test.tsx` (new).
  **How:** render `AddPanelMenuItems` inside `DropdownMenu`/`DropdownMenuContent`
  with `state.prs` of length 1; assert the inline item testid is in the
  document and the submenu trigger testid is absent.
- **What:** no PRs render no PR row and no submenu trigger.
  **File:** same test file. **How:** render with `state.prs: []`.
- **What:** two or more PRs render only the submenu trigger at top level.
  **File:** same test file. **How:** render with 2 PRs; assert the trigger is
  present and the inline `add-panel-pr-item-*` rows are not present until the
  submenu is opened.
- **What:** opening the submenu shows each PR row with repo-disambiguated label.
  **File:** same test file. **How:** open the Radix submenu (pointer-enter/move
  or keyboard on the trigger; follow `task-create-dialog-pill.test.tsx` /
  `executor-settings-button.test.tsx` patterns), then assert every
  `add-panel-pr-item-*` row is visible and labeled `PR #N — owner/repo`.
- **What:** selecting a submenu row calls `addPRPanel` with that PR's task key
  and the active session id.
  **File:** same test file. **How:** mock `@/lib/state/dockview-store`
  (`useDockviewStore` returning `addPRPanel: vi.fn()`), mock
  `@/components/state-provider` (`useAppStore` returning
  `{ tasks: { activeSessionId: "session-1" } }`), mock the remaining child hooks
  (`useTaskSessions`, `useSessionPendingInput`, `useRepositoryScripts`,
  `useEnvironmentId`) to render `AddPanelMenuItems`; click a row and assert
  `addPRPanel` was called with `"owner/repo/N"` and `"session-1"`.

> jsdom notes: Radix submenus only mount `SubContent` when open; open it with
> pointer events inside `act()`/`waitFor`. If Radix presence keeps the content
> out of the DOM in jsdom, stub `requestAnimationFrame` or mock
> `@kandev/ui/dropdown-menu`'s sub trio to a deterministic open-on-click pair —
> but prefer driving the real component first.

## E2E Tests

- **Scenario:** a task with two linked PRs; from the "+" menu the user opens the
  Pull requests submenu and picks the secondary PR, which opens its own panel
  (existing `pr-multi-popover.spec.ts` scenario).
  **File:** `apps/web/e2e/tests/pr/pr-multi-popover.spec.ts`.
  **What to verify:** after `session.addPanelButton().click()`, the test now
  asserts the `add-panel-pr-submenu` trigger is visible, opens it (click), then
  clicks `add-panel-pr-item-${OWNER}-api-77` and expects a second `prDetailTab`.
- **Scenario (mobile):** on a <640px viewport, the nested Pull requests submenu
  opens from the "+" menu, its rows are tappable, and the bottom-sheet
  presentation stays within the viewport.
  **File:** `apps/web/e2e/tests/task/mobile-add-panel-pr-submenu.spec.ts` (new,
  `mobile-*.spec.ts` so `mobile-chrome` picks it up), modeled on
  `apps/web/e2e/tests/task/mobile-external-link-menu.spec.ts`.
  **What to verify:** seed a task with two linked PRs (mirror
  `associateTwoPRs` from `pr-multi-popover.spec.ts`), open the "+" menu, open
  the submenu, assert a PR row is visible and ≥44px tall, tapping it opens the
  PR panel, and assert the nested menu box stays within the viewport with no
  document horizontal overflow (`assertNoDocumentHorizontalOverflow` helper,
  used by `apps/web/e2e/tests/gitlab/mobile-gitlab-parity.spec.ts`).

## Implementation Waves

Small feature — sequential, no parallel candidates.

```
Wave 1:
- [ ] [task-01-frontend-submenu](task-01-frontend-submenu.md)

Wave 2:
- [ ] [task-02-e2e](task-02-e2e.md)
```

## Open Questions

(Delete when empty.)
