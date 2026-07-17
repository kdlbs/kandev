---
id: "01-mobile-topbar"
title: "Mobile Quick Chat topbar access"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/mobile-quick-chat-topbar/spec.md"
---

# Task 01: Mobile Quick Chat topbar access

## Acceptance

- PR #1751's accessible mobile `Quick Chat` button remains immediately before `Search tasks`
  and continues to open the existing workspace-scoped Quick Chat dialog.
- The mobile Kandev wordmark links to the active workspace's Home board, Home does not render a
  redundant page label, and non-Home pages retain their existing page-title visibility.
- The button remains absent without an active workspace, the stacked tablet/session/modal
  behavior is unchanged, and the mobile header has no horizontal page overflow.

## Files

- `apps/web/components/kanban/kanban-header-mobile.tsx`
- `apps/web/components/kanban/kanban-header-mobile.test.tsx`
- `apps/web/e2e/tests/chat/mobile-quick-chat-entry.spec.ts`
- `docs/plans/mobile-quick-chat-topbar/plan.md` (status only)
- `docs/plans/mobile-quick-chat-topbar/task-01-mobile-topbar.md` (status only)

## Inputs

- Spec: `What` and all `Scenarios` in
  `docs/specs/mobile-quick-chat-topbar/spec.md`.
- Plan: `Frontend`, `Tests`, `E2E Tests`, and `Risks` in `plan.md`.
- Existing patterns: `workspaceHomeHref` in
  `apps/web/components/app-sidebar/app-sidebar-workspace-navigation.ts`, and the existing
  mobile search/menu actions in `kanban-header-mobile.tsx`.
- Dependency: PR #1751 (`feature/quick-chat-on-mobile-wnn`, commit `6ff1d2f74`).

## Implementation Notes

- Use TDD: update the component tests first, confirm failure, then make the smallest header
  change that passes.
- Reuse the compact Quick Chat action and existing launcher from PR #1751; do not duplicate
  Quick Chat state or launch logic.
- Use semantic link/button roles and existing accessible names. Keep the action order explicit
  in the rendered DOM.
- Update the existing mobile Quick Chat E2E entry steps instead of adding a duplicative flow.

## Verification

From `apps/web`:

```bash
pnpm test -- components/kanban/kanban-header-mobile.test.tsx
pnpm e2e:run --host --project mobile-chrome -- tests/chat/mobile-quick-chat-entry.spec.ts
```

From the repository root:

```bash
make fmt
make typecheck
make test
make lint
```

## Output Contract

Report the behavior implemented, files changed, focused and full verification results, rendered
mobile layout observations, blockers, residual risks, and follow-up work. Set this task to
`in_progress` before code changes, then `done` and update `plan.md` only after acceptance and
verification pass.
