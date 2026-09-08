---
id: "03-add-auto-run-switch"
title: "Add the Auto-run switch"
status: done
wave: 3
depends_on: ["02-enforce-queue-auto-run"]
plan: "plan.md"
spec: "../../specs/ui/requirements/message-queue-run.md"
---

# Task 03: Add the Auto-run switch

## Intent

Project the server-owned policy in the queue panel, remove misleading header
actions, and keep exact Send Now on every row with full desktop/mobile
accessibility.

## Acceptance

1. Queue API, WebSocket status handling, Zustand meta, and `useQueue` carry the
   authoritative Auto-run boolean. Toggling sends the exact boolean, uses the
   existing mutation gate, and refetches after both success and failure.
2. The header renders one labeled Switch with state-specific localized helper
   copy. Header Run Next and bulk Send Now, their callbacks, and their test IDs
   are absent. Generic drain and all-scope Send Now API compatibility remains.
3. Every row, including index zero, exposes targeted Send Now. Clicking any row
   uses its persisted entry ID; no head-only Skip action exists.
4. The switch is disabled during queue mutation or backend cancellation,
   exposes checked/disabled semantics, wraps without overflow, and has a
   44-pixel effective coarse-pointer target.
5. English, Portuguese, simplified/traditional Chinese, and generated pseudo
   locales are complete. Obsolete Run Next keys have no remaining consumer.

## Verification

```bash
cd apps
pnpm install --frozen-lockfile
pnpm --filter @kandev/web exec vitest run components/task/chat/queued-ghost-list.test.tsx components/task/chat/queued-ghost-message.test.tsx components/task/chat/queued-ghost-pin.test.tsx hooks/domains/session/use-queue.test.ts lib/api/domains/queue-api.test.ts lib/state/slices/session/session-slice.upsert.test.ts lib/ws/handlers/agent-session.test.ts
cd web
pnpm run typecheck
pnpm exec eslint --max-warnings 0 components/task/chat/queued-ghost-panel-header.tsx components/task/chat/queued-ghost-list.tsx components/task/chat/queued-ghost-list.test.tsx components/task/chat/queued-ghost-message.tsx components/task/chat/queued-ghost-message.test.tsx hooks/domains/session/use-queue.ts hooks/domains/session/use-queue.test.ts lib/api/domains/queue-api.ts lib/api/domains/queue-api.test.ts lib/state/slices/session/types.ts lib/state/slices/session/session-slice.ts lib/state/slices/session/session-slice.upsert.test.ts lib/ws/handlers/agent-session.ts lib/ws/handlers/agent-session.test.ts
pnpm run i18n:pseudo
pnpm run i18n:check
pnpm run i18n:ratchet
```

If implementation keeps the switch inside the header instead of extracting
the named component, update the focused file/test list and record the reason.

## Files likely touched

- `apps/web/lib/api/domains/queue-api.ts`
- `apps/web/lib/api/domains/queue-api.test.ts`
- `apps/web/lib/state/slices/session/types.ts`
- `apps/web/lib/state/slices/session/session-slice.ts`
- `apps/web/lib/state/slices/session/session-slice.upsert.test.ts`
- `apps/web/lib/ws/handlers/agent-session.ts`
- `apps/web/lib/ws/handlers/agent-session.test.ts`
- `apps/web/hooks/domains/session/use-queue.ts`
- `apps/web/hooks/domains/session/use-queue.test.ts`
- `apps/web/components/task/chat/queued-ghost-auto-run-control.tsx` (new)
- `apps/web/components/task/chat/queued-ghost-auto-run-control.test.tsx` (new)
- `apps/web/components/task/chat/queued-ghost-panel-header.tsx`
- `apps/web/components/task/chat/queued-ghost-list.tsx`
- `apps/web/components/task/chat/queued-ghost-list.test.tsx`
- `apps/web/components/task/chat/queued-ghost-message.tsx`
- `apps/web/components/task/chat/queued-ghost-message.test.tsx`
- `apps/web/components/task/chat/queued-ghost-pin.test.tsx`
- `apps/web/src/locales/en/chat.json`
- `apps/web/src/locales/pt-pt/chat.json`
- `apps/web/src/locales/zh-cn/chat.json`
- `apps/web/src/locales/zh-hk/chat.json`
- `apps/web/src/locales/zh-tw/chat.json`
- `apps/web/src/locales/pseudo/chat.json`

## Dependencies

Task 02. The API action and status projection must be final.

## Parallelism

Sequential. This task owns selectors, copy, and UI behavior consumed by E2E.

## Inputs

- Spec: `What`, `API Surface`, `Responsive and Mobile Behavior`.
- Mobile contract in `plan.md`.
- Existing compact labeled switch pattern in
  `components/quick-chat/configuration-chat-toggle.tsx`.

## Risks

- The queue panel renders only when entries exist, but OFF must still survive
  an empty queue in the store/backend and reappear when later work arrives.
- Do not optimistically claim ON if the server rejected the mutation.
- Header wrapping must not create a second scroll owner or shrink touch
  targets.
- Remove only first-party bulk/drain plumbing, not backward-compatible API
  methods or row Send Now.

## Output contract

Report user-visible behavior, accessible semantics, locale changes, files
changed, exact commands and test counts, compatibility audits, blockers, and
residual risks. Set this task to `done`, record results below, and synchronize
`plan.md` before Task 04.

## Results

- Added the exact `message.queue.auto_run.set` client action, authoritative
  fetch/broadcast projection, queue meta, and reconciled hook mutation. Missing
  additive status fields default to ON for rolling compatibility.
- Replaced both legacy header dispatch actions with a labeled, described
  Switch. It wraps on a dedicated header row, exposes checked/disabled state,
  and expands its coarse-pointer hit area. Targeted Send Now remains on every
  queued row, including the head.
- Kept the control in `queued-ghost-panel-header.tsx`: its presentation is
  header-only and small enough that a one-use component would add indirection
  without reducing the large list component.
- Added all five real-locale strings, removed unused Run Next/bulk Send Now
  copy, and regenerated pseudo locale output.
- `vitest`: 7 files, 220 tests passed. `pnpm run typecheck`, focused ESLint,
  `i18n:check`, and `i18n:ratchet` passed. The i18n checker retained its
  existing advisory real-locale parity backlog; this task's chat keys are
  complete.
