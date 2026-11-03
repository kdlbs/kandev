---
id: "01-align-child-navigation"
title: "Align child navigation and session ownership"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-SUBTASK-ACTIVATION-001
acceptance_criteria:
  - AC-TASKS-SUBTASK-ACTIVATION-001.1
  - AC-TASKS-SUBTASK-ACTIVATION-001.2
  - AC-TASKS-SUBTASK-ACTIVATION-001.3
  - AC-TASKS-SUBTASK-ACTIVATION-001.4
  - AC-TASKS-SUBTASK-ACTIVATION-001.5
system_design:
  - ../../specs/tasks/system-design/subtask-activation.md
---

# Task 01: Align child navigation and session ownership

## Summary

Make successful subtask creation open the exact child conversation through the
SPA router. Keep all task-page session consumers within the rendered task during
the handoff. Prove the result with real-router integration and desktop/phone E2E.

## In scope

- Read the [creation handoff and ownership design](../../specs/tasks/system-design/subtask-activation.md)
  and the [diagnostic evidence](plan.md#evidence-and-root-cause).
- Start with a failing regression named `opens the created child in route subscribers`
  in `use-subtask-submit.navigation.test.tsx`. Use the real router, link builder,
  and Zustand store. Mock the create transport and unrelated providers only.
- Keep `session_id`, `primary_session_id`, and null precedence. Use the existing
  router replacement path, include the session query, close before navigation,
  and publish `setActiveSession` inside `onNavigated`.
- Add mounted task-page ownership evidence before correcting candidate admission.
  Mock only unrelated heavy surfaces and transports. Observe the actual effective
  task/session pair and status/recovery calls, rather than testing a copied selector.
- Run the scenario matrix and exact commands. Record RED and GREEN results.

## Out of scope

Backend changes, generic router changes, other URL-helper callers, new lifecycle
behavior, new translations, dialog styling, and creation recovery retries.

## Acceptance

1. The real submit/router integration opens the exact child session and closes
   first. Deferred or cancelled navigation does not publish the child selection.
2. The mounted task page never combines a rendered task with a foreign or unknown
   session. Delayed hydration recovers the child's owned session without another launch.
3. Desktop and phone UI submission show the child's conversation before reload.
   Every observed status/recovery pair has verified ownership and existing submit cases pass.

## Scenario matrix

| Boundary | Cases |
| --- | --- |
| Successful response | `session_id`, only `primary_session_id`, both fields with `session_id` precedence |
| Existing failure paths | Rejected create retains feedback, no session closes without activation, double submit creates once |
| Navigation | Real `useParams` and `useSearchParams` update, replacement preserves history length, deferred proceed and cancel |
| Mounted task | Parent rendered with child global selection, child row already loaded, child row delayed |
| Candidate ownership | Foreign active selection, unknown active row, foreign/unknown route session, owned fallback after a rejected candidate, no eligible candidate |
| Read concurrency | Child route read deferred while parent remains mounted, late parent response after child hydration |
| Rendered outcome | Exact child title/conversation and child-owned status request before reload, then after reload, on desktop and phone |

Arm WebSocket request capture before submit. Resolve ownership from seeded parent
sessions and the exact child create response. Keep the observer through a positive
child status response and settled rendered conversation. Use the repository's
causal waits for any additional negative observation window. Do not treat a broad
`/t/` URL assertion, timeout, mocked navigation call, or reload as proof of the handoff.

## ASCII UI preview

UI-01: Creation from an open parent. See the [full preview](plan.md#ascii-ui-preview).

```text
Desktop: task list | Child task + conversation | existing panels
Phone:   Child header
         Child conversation (one scroll owner)
         Composer + existing bottom navigation
Both:    dialog closed, /t/child?sessionId=child-session
         status/recovery -> (Child, Child session)
```

Criteria `.1`, `.2`, and `.5` require consistent identity and a focused phone
conversation. Existing layout and translated labels supply the presentation.
The phone entry is the existing task-actions menu, followed by direct navigation.
Keep current safe areas, scroll ownership, and touch targets.

## Verification

From the repository root, install dependencies once if this is a fresh worktree:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Run these commands after the TDD implementation. The managed E2E runner builds
fresh assets and starts isolated test instances. Run desktop and phone sequentially.

```bash
(cd apps/web && pnpm exec vitest run components/task/use-subtask-submit.test.ts components/task/use-subtask-submit.navigation.test.tsx components/task/task-page-session-ownership.test.tsx components/task/dockview-desktop-layout.test.ts lib/routing/client-router.test.ts src/task-detail-route-session-selection.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/use-subtask-submit.ts components/task/task-page-content.tsx components/task/task-page-session-ownership.ts components/task/use-subtask-submit.test.ts components/task/use-subtask-submit.navigation.test.tsx components/task/task-page-session-ownership.test.tsx components/task/dockview-desktop-layout.tsx components/task/dockview-desktop-layout-hooks.ts components/task/dockview-desktop-layout.test.ts components/task/task-layout.tsx e2e/helpers/document-navigation.ts e2e/helpers/session-status-ownership.ts e2e/helpers/ws-capture.ts e2e/tests/task/subtask.spec.ts e2e/tests/task/mobile-subtask-activation.spec.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/task/subtask.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-subtask-activation.spec.ts -- --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

If a small ownership helper becomes necessary, keep it beside the task page.
Add its source and test paths to the corresponding commands before completing
the work order. Do not export the private page hook only to make it testable.

## Files likely touched

- `apps/web/components/task/use-subtask-submit.ts`
- `apps/web/components/task/use-subtask-submit.test.ts`
- `apps/web/components/task/use-subtask-submit.navigation.test.tsx` (new)
- `apps/web/components/task/task-page-content.tsx`
- `apps/web/components/task/task-page-session-ownership.ts` (new)
- `apps/web/components/task/dockview-desktop-layout.tsx`
- `apps/web/components/task/dockview-desktop-layout-hooks.ts`
- `apps/web/components/task/task-layout.tsx`
- `apps/web/components/task/task-page-session-ownership.test.tsx` (new)
- `apps/web/e2e/helpers/document-navigation.ts` (new)
- `apps/web/e2e/helpers/session-status-ownership.ts` (new)
- `apps/web/e2e/helpers/ws-capture.ts`
- `apps/web/e2e/tests/task/subtask.spec.ts`
- `apps/web/e2e/tests/task/mobile-subtask-activation.spec.ts` (new)

## Dependencies

None. The existing router, navigation guard, state actions, and responsive layouts
already provide the required interfaces.

## Risks

- A global active-task ownership check accepts the child's session while the
  rendered task still belongs to the parent. Use the resolved rendered task.
- An absent session row is not foreign ownership proof or permission to launch.
  Existing hydration must supply the row before admission.
- UI tests can hide the race by mocking the router, eagerly hydrating every row,
  or navigating away before the child conversation appears.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/subtask-activation.md), criteria `.1` through `.5`.
- [System design](../../specs/tasks/system-design/subtask-activation.md).
- `apps/web/lib/routing/client-router.ts` and `client-router.test.ts`.
- `apps/web/lib/state/slices/kanban/kanban-slice.ts`, `setActiveSession`.
- `apps/web/src/task-route-projection.ts` and `task-detail-route-session-selection.test.tsx`.
- `apps/web/components/task/task-page-content-canonical.test.tsx` for real-store mounting.
- `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts`, existing New Subtask opener.
- Root and web `AGENTS.md`, `/tdd`, `/e2e`, and `/mobile-parity`.

## Results

Completed on 2026-11-03 against repository commit `8cd31a0896a` plus this change.

- RED: `use-subtask-submit.navigation.test.tsx` failed on the real router
  subscription (`taskId` stayed `parent-task`, the session query was absent) and
  `task-page-session-ownership.test.tsx` failed every case that observed the
  mixed pair, including the delayed-hydration case.
- GREEN: the same command passed after `useSubtaskSubmit` moved to
  `useRouter().replace(linkToTask(childId, { sessionId }), { scroll: false, onNavigated })`
  and `useTaskPageData` admitted only candidates whose session row belongs to the
  rendered task. Deferred navigation publishes `setActiveSession` inside
  `onNavigated`; cancellation leaves the parent selection and does not recreate.
- Unit/integration: 6 files, 55 tests passed (`use-subtask-submit.test.ts`,
  `use-subtask-submit.navigation.test.tsx`,
  `task-page-session-ownership.test.tsx`, `dockview-desktop-layout.test.ts`,
  `client-router.test.ts`, `task-detail-route-session-selection.test.tsx`).
- Broader page regression: `task-page-content*.test.*` and
  `task-detail-route-recovery.test.tsx`, 7 files, 101 tests passed.
- `pnpm run typecheck` passed (with `NODE_OPTIONS=--max-old-space-size=6144`).
- `eslint --max-warnings 0` passed for both production files, both unit test
  files, `task-page-session-ownership.ts`, both E2E specs, and both E2E helpers.
- Desktop E2E: `subtask.spec.ts`, 16/16 passed on `chromium`, including the
  extended `create subtask from the sidebar task context menu` case. Its
  pre-reload evidence is passive: exactly one visible `session-chat` carries the
  child's `data-session-id`, the chat contains the child-only prompt marker and
  not the parent marker, and an armed main-frame document-navigation observer is
  still empty after the positive child status request. The explicit reload is a
  separate later persistence check that repeats those exact-session/content
  assertions.
- Phone E2E: `mobile-subtask-activation.spec.ts`, 1/1 passed on `mobile-chrome`
  with the same passive exact child-session/content and no-navigation evidence,
  plus an editable composer, reachable bottom navigation, no document horizontal
  overflow, and child-owned status requests before and after the explicit reload.
- Fixup review findings: an unhydrated explicit route session stays pending
  instead of showing an older task session; automatic session ensuring is
  disabled while that ownership is unresolved or an owned session exists; and
  the desktop Dockview layout uses the global active session only when its row
  belongs to the rendered task. Unit coverage includes route-pending,
  same-task active-precedence, ensure-gating, and Dockview ownership cases.
- Docs: `python3 scripts/list-docs.py validate` (428 decisions, 1,895
  specifications), `python3 scripts/lint-spec-files.test.py` (36 tests), and
  `python3 scripts/lint-spec-files.py --all` passed. `git diff --check` passed.
- The requirement is `active`, the design is `current`, and this work order and
  the plan are complete.
