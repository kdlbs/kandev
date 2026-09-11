---
id: "02-departure-presentation"
title: "Integrate departure presentation"
status: done
wave: 2
depends_on: ["01-removal-coordinator"]
plan: "plan.md"
requirements:
  - REQ-TASKS-REMOVAL-NAVIGATION-001
  - REQ-TASKS-REMOVAL-NAVIGATION-002
acceptance_criteria:
  - AC-TASKS-REMOVAL-NAVIGATION-001.1
  - AC-TASKS-REMOVAL-NAVIGATION-001.2
  - AC-TASKS-REMOVAL-NAVIGATION-001.4
  - AC-TASKS-REMOVAL-NAVIGATION-001.5
  - AC-TASKS-REMOVAL-NAVIGATION-002.1
  - AC-TASKS-REMOVAL-NAVIGATION-002.2
  - AC-TASKS-REMOVAL-NAVIGATION-002.3
  - AC-TASKS-REMOVAL-NAVIGATION-002.4
  - AC-TASKS-REMOVAL-NAVIGATION-002.5
system_design:
  - ../../specs/tasks/system-design/removal-navigation.md
---

# Task 02: Integrate departure presentation

## Summary

Connect accepted UI removal actions to the coordinator and prevent outgoing task
content or effects from remaining active. Preserve the current desktop and phone
navigation shells while the destination becomes ready.

## In scope

- A subscribed removal boundary above live task-detail and preview effects.
- Gate `useEnsureTaskSession` at enablement and dispatch; prevent initial-route
  task props or delayed session hydration from restoring the outgoing task.
- Integrate desktop sidebar, phone switcher, board CRUD, sidebar bulk actions,
  archive-confirmation/command/banner callers, and task-action message deletion.
- Close selected previews with their URL parameters; restore conditionally on failure.
- Localized status, one notification per operation/batch, focus transfer, and
  menu dismissal; avoid duplicate dirty-conflict toasts.
- A call-site inventory recording intentional Quick Chat/session-only exclusions.

## Out of scope

Backend lifecycle changes, new mobile composition, session-only deletion,
Quick Chat close/expiration, and browser suite implementation.

## Acceptance

- After acceptance, no outgoing task-dependent content mounts or ensure request
  dispatches during early WS events or delayed HTTP; confirmation cancellation
  leaves both content and requests unchanged.
- Every ordinary task removal entry point uses shared intent before mutation;
  unselected removal preserves current view/focus and bulk results retain retries.
- Desktop/phone loading, focus, preview closure, and failure recovery satisfy the
  same contract with localized copy and usable navigation.

## Verification

Run from the repository root after 01 has installed dependencies.

```bash
(cd apps/web && rtk pnpm exec vitest run components/task/task-page-content.test.tsx components/kanban-with-preview.test.ts components/task-preview-panel.test.tsx hooks/domains/session/use-ensure-task-session.test.ts hooks/use-task-actions.test.ts hooks/use-task-crud.test.ts hooks/use-sidebar-multi-select.test.ts hooks/use-task-archive-confirm.test.ts components/task/task-archive-confirm-dialog.test.tsx)
(cd apps/web && rtk pnpm run typecheck)
(cd apps/web && rtk pnpm run i18n:check)
(cd apps/web && rtk pnpm run i18n:ratchet)
rtk git diff --check
```

`hooks/use-task-crud.test.ts` is a planned new suite. Add behavioral RED tests
before integrating each surface. Run the relevant command again after its last
change. If additional test suites are changed, add their exact paths here.

## Files likely touched

- `apps/web/components/task/task-page-content.tsx` and `task-page-inner.tsx`
- `apps/web/components/task/task-removal-boundary.tsx` (proposed new component)
- `apps/web/components/kanban-with-preview.tsx`, `task-preview-panel.tsx`, and tests
- `apps/web/hooks/domains/session/use-ensure-task-session.ts` and tests
- `apps/web/components/task/task-session-sidebar.tsx`
- `apps/web/components/task/mobile/session-task-switcher-sheet-hooks.ts`
- `apps/web/hooks/use-task-crud.ts`, `use-sidebar-multi-select.ts`, and tests
- `apps/web/hooks/use-task-archive-confirm.ts` and archive confirmation tests
- `apps/web/components/task/chat/messages/action-message-actions.tsx`
- `apps/web/hooks/use-task-actions.ts`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/{common,tasks}.json`
- `apps/web/AGENTS.md` only if the resulting shared lifecycle guidance changes

## Dependencies

01 supplies operation state, ownership tokens, and coordinator.

## Risks

Returning early after task data hooks leaves side effects alive. Hiding a
Dockview tree with CSS leaves its panels mounted. Dialog focus restoration may
target the removed task; override it only for the accepted departure.
Read `components/task/chat/AGENTS.md` before editing action-message code.

## Parallelism

`sequential`

## Inputs

- Removal design: Presentation boundary and Entry points and mobile composition.
- Mobile exemplar: `components/task/mobile/session-task-switcher-sheet.tsx`.
- Existing task-page, preview, ensure-session, archive-confirmation, and bulk tests.
- `docs/i18n.md`; generate Traditional Chinese using `pnpm run i18n:zh-hant`.

## Results

Done on 2026-09-10.

- Added the shared detail/preview departure boundary, ensure-session gate,
  desktop and phone coordinator entry points, bulk integration, action-message
  deletion, localized status and success notifications, and guarded preview
  closure.
- Presentation verification command: 9 files, 94 tests passed.
- Web lint, i18n check, and i18n ratchet passed.
- The direct archive/delete audit keeps `/tasks` list mutations transport-only;
  Quick Chat and session-only deletion remain intentional exclusions.
- Repository typecheck still reports only the pre-existing duplicate declarations
  in `lib/types/http.ts` and `lib/ws/handlers/workflows.ts`.
