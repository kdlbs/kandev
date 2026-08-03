---
id: "05-backend-owned-cancel-control"
title: "Backend-owned cancel control"
status: completed
wave: 3
depends_on: ["04-cancellation-projection-contract"]
plan: "plan.md"
spec: "../../specs/ui/cancel-turn-progress.md"
---

# Task 05: Backend-owned cancel control

## Acceptance

- The frontend session contract hydrates and live-updates explicit true/false backend cancellation
  state plus its revision without changing coarse session state, task selection, or Office refetch
  behavior.
- Delayed REST/boot hydration with a lower revision cannot overwrite a newer live cancellation event.
- The shared cancel control renders progress when either the short-lived optimistic request flag or
  backend `cancellation_pending` is true; backend true survives a fresh `StateProvider`, while both
  false render the retryable idle control.
- Desktop, compact mobile, and minimal toolbars remain session-isolated and keep the current
  duplicate-click, tooltip, error, and spinner behavior.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web test -- --run lib/ws/handlers/agent-session.test.ts components/task/chat/chat-input-toolbar.test.tsx lib/state/slices/ui/ui-slice.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web run i18n:ratchet)
```

Follow TDD: first hydrate a new store with `cancellation_pending=true` and deliver revisioned
true/false live events against the existing code, then add the ordered merge contract and
effective-state union.

## Files likely touched

- `apps/web/lib/types/http.ts`
- `apps/web/lib/types/backend.ts`
- `apps/web/lib/ws/handlers/agent-session.ts`
- `apps/web/lib/ws/handlers/agent-session.test.ts`
- `apps/web/components/task/chat/chat-input-toolbar-primitives.tsx`
- `apps/web/components/task/chat/chat-input-toolbar.test.tsx`

## Dependencies

Task 04.

## Parallelism

Sequential. The frontend must compile against Task 04's exact payload and hydration contract.

## Inputs

- Spec: `What`, `Data model`, `API surface`, and desktop/mobile scenarios.
- Plan: `Session contracts and live updates`, `Shared cancel control`, and `Mobile design contract`.
- Existing patterns: `buildSessionUpdate`, `applyForegroundActivity`,
  `upsertTaskSessionFromEvent`, `chatInput.cancellingBySessionId`, and the shared `SubmitButton`.

## Output contract

Report the authoritative/optimistic merge rule, Red/Green component and handler evidence, exact
files/commands, responsive coverage, blockers/risks, and synchronize this task plus `plan.md` in the
same primary conversation.

## Results

Implemented the backend-owned session contract and shared desktop/mobile control merge.

- `cd apps && pnpm --filter @kandev/web test -- --run lib/ws/handlers/agent-session.test.ts components/task/chat/chat-input-toolbar.test.tsx lib/state/slices/ui/ui-slice.test.ts` — 3 files, 104 passed.
- `cd apps/web && pnpm run typecheck` — passed.
- `cd apps && pnpm --filter @kandev/web lint` — passed.
- `cd apps && pnpm --filter @kandev/web run i18n:ratchet` — passed (0 added, 11 modified files clean).
- `git diff --check` — passed.

The shared cancel control uses backend `true` OR the transient optimistic request flag, so backend
state survives a fresh store while both cleared values remain retryable. Session merges accept only
the newest `cancellation_revision`, preventing delayed REST/boot snapshots from restoring stale
progress. The frontend wire fields remain optional for partial in-memory/test rows, while backend DTO
and hydration boundaries always serialize them explicitly. No new copy, browser persistence, security
boundary, or external side effect was introduced; desktop and compact mobile continue using the same
`SubmitButton`.
