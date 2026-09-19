---
id: "03-web-descriptor-reconciliation"
title: "Reconcile descriptors in the web app"
status: done
wave: 3
depends_on: ["02-descriptor-api-boot"]
plan: "plan.md"
spec: "../../specs/ui/requirements/quick-terminal.md"
---

# Task 03: Reconcile descriptors in the web app

## Acceptance

- Web API types and calls map the server descriptor contract without routing terminal rows through
  Quick Chat conversation APIs.
- Boot, workspace resync, and reconnect preserve persisted terminals, active-kind/last-terminal
  selection, and sibling isolation while adopting server sequence/session/status values.
- New terminal creation and lifecycle changes persist the descriptor; restored exited/error or
  unavailable tabs render status and never start a replacement implicitly.
- Detach-on-unmount, StrictMode replay, late start resolution, natural exit disarming, explicit
  close/404 handling, and existing Agents stop-on-unmount behavior remain correct.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web exec vitest run lib/api/domains/quick-terminal-api.test.ts lib/state/slices/ui/quick-chat-actions.test.ts lib/state/slices/ui/quick-chat-sync.test.ts lib/state/hydration/hydrator.test.ts components/quick-chat/use-quick-chat-modal.test.ts components/quick-chat/quick-terminal-tab-view.test.tsx components/settings/pty-terminal-view.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web exec eslint lib/api/domains/quick-terminal-api.ts lib/types/http-quick-terminal.ts lib/state/slices/ui/types.ts lib/state/slices/ui/quick-terminal-actions.ts lib/state/slices/ui/quick-chat-sync.ts lib/state/default-state.ts lib/state/hydration/hydrator.ts hooks/use-quick-chat-resync.ts components/quick-chat/use-quick-chat-modal.ts components/quick-chat/quick-terminal-tab-view.tsx components/settings/pty-terminal-view.tsx)
(cd apps && pnpm --filter @kandev/web run i18n:ratchet)
(cd apps && pnpm --filter @kandev/web run i18n:check)
```

## Files likely touched

- `apps/web/lib/types/http-quick-terminal.ts` (new)
- `apps/web/lib/api/domains/quick-terminal-api.ts` (new)
- `apps/web/lib/api/domains/quick-terminal-api.test.ts` (new)
- `apps/web/lib/api/index.ts`
- `apps/web/lib/state/slices/ui/types.ts`
- `apps/web/lib/state/slices/ui/quick-terminal-actions.ts`
- `apps/web/lib/state/slices/ui/quick-chat-sync.ts`
- `apps/web/lib/state/slices/ui/quick-chat-sync.test.ts`
- `apps/web/lib/state/default-state.ts`
- `apps/web/lib/state/hydration/hydrator.ts`
- `apps/web/lib/state/hydration/hydrator.test.ts`
- `apps/web/hooks/use-quick-chat-resync.ts`
- `apps/web/hooks/use-quick-terminal-launcher.ts`
- `apps/web/components/quick-chat/use-quick-chat-modal.ts`
- `apps/web/components/quick-chat/use-quick-chat-modal.test.ts`
- `apps/web/components/quick-chat/quick-terminal-tab-view.tsx`
- `apps/web/components/quick-chat/quick-terminal-tab-view.test.tsx`
- `apps/web/components/settings/pty-terminal-view.tsx`
- `apps/web/components/settings/pty-terminal-view.test.tsx`
- `apps/web/src/locales/en/sidebar.json`

## Dependencies

Task 02.

## Parallelism

Sequential. API, hydration, store, lifecycle, and modal ownership share the same tab identity and
must be implemented as one frontend contract.

## Inputs

- Spec Data model, API surface, State machine, Failure modes, and Scenarios.
- Existing `QuickTerminalTab`, `quick-terminal-actions`, `quick-chat-sync`, and detachable PTY
  lifecycle behavior.
- Existing server conversation resync and localized terminal lifecycle status components.

## Output contract

Report changed frontend files, behavior for stale sessions and late starts, exact focused test/lint/
typecheck/i18n outcomes, and any unresolved concurrency risk. Synchronize task/plan status.

## Results

- Added the descriptor API mapping, boot/resync reconciliation, durable lifecycle updates, restored-session attach behavior, stale-session status UI, and descriptor-owned close flow.
- `(cd apps && pnpm --filter @kandev/web exec vitest run --run lib/api/domains/quick-terminal-api.test.ts lib/state/slices/ui/quick-chat-actions.test.ts lib/state/slices/ui/quick-chat-sync.test.ts lib/state/hydration/hydrator.test.ts hooks/use-quick-chat-resync.test.ts components/quick-chat/quick-terminal-tab-view.test.tsx components/quick-chat/use-quick-chat-modal.test.ts components/settings/pty-terminal-view.test.tsx components/app-sidebar/app-sidebar-new-task-item.test.tsx)` passed (9 files, 124 tests).
- Web typecheck, changed-file ESLint, i18n ratchet, and i18n checks passed. The i18n check reported only the existing advisory 657 zh-cn catalog issues and 5 orphan English keys.
