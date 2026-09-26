---
id: "01-session-scoped-cancel-state"
title: "Session-scoped cancel state"
status: done
wave: 1
depends_on: []
superseded_by: "05-backend-owned-cancel-control"
plan: "plan.md"
spec: "../../specs/ui/requirements/cancel-turn-progress.md"
---

# Task 01: Session-scoped cancel state

> Historical implementation record. The UI-slice flag remains only as immediate optimistic
> feedback; Task 05 replaces it as the authoritative owner with backend-hydrated session state.

## Acceptance

- A pending cancellation is represented in the `chatInput` UI slice by session ID and is cleared
  when the request succeeds, rejects, times out, or cannot be sent.
- Unmounting and remounting the desktop or compact mobile toolbar under the same `StateProvider`
  preserves the disabled spinner state for that session, while another session remains unaffected.
- Repeated activation during the pending request invokes `onCancel` only once; after failure, a
  still-visible control can be retried.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web test -- --run lib/state/slices/ui/ui-slice.test.ts components/task/chat/chat-input-toolbar.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web run i18n:ratchet)
```

Follow TDD: first add the remount regression and confirm it fails because `SubmitButton` resets its
local state, then move the state into the shared slice and make the focused tests pass.

## Files likely touched

- `apps/web/lib/state/slices/ui/types.ts`
- `apps/web/lib/state/slices/ui/ui-slice.ts`
- `apps/web/lib/state/slices/ui/ui-slice.test.ts`
- `apps/web/components/task/chat/chat-input-toolbar-primitives.tsx`
- `apps/web/components/task/chat/chat-input-toolbar.tsx`
- `apps/web/components/task/chat/chat-input-toolbar-desktop.tsx`
- `apps/web/components/task/chat/chat-input-toolbar-mobile.tsx`
- `apps/web/components/task/chat/chat-input-toolbar.test.tsx`

## Dependencies

None.

## Parallelism

Sequential. The state action, request guard, responsive prop wiring, and regression test form one
shared control contract.

## Inputs

- Spec: `What`, `Persistence guarantees`, `Failure modes`, and `Scenarios`.
- Plan: `Decision`, `Transient chat-input state`, `Shared cancel control`, and `Tests`.
- Existing patterns: `chatInput.planModeBySessionId` in the UI slice and the shared
  `SubmitButton` used by desktop, compact mobile, and minimal toolbars.

## Output contract

Report the behavioral change, exact files changed, Red/Green test evidence, typecheck and i18n
results, blockers or risks, and update this task plus `plan.md` status/results in the same primary
conversation.

## Results

- RED: `cd apps && pnpm --filter @kandev/web test -- --run components/task/chat/chat-input-toolbar.test.tsx` —
  2 expected remount-regression failures (desktop and mobile) against the local-state implementation.
- `cd apps && pnpm install --frozen-lockfile` — passed.
- `cd apps && pnpm --filter @kandev/web test -- --run lib/state/slices/ui/ui-slice.test.ts components/task/chat/chat-input-toolbar.test.tsx` —
  passed, 2 files / 52 tests.
- `cd apps/web && pnpm run typecheck` — first run caught the missing explicit `AppState` action export;
  after adding it, rerun passed.
- `cd apps && pnpm --filter @kandev/web run i18n:ratchet` — passed; 0 added and 7 modified files clean.
- `git diff --check` — passed.
- Security/trust or external side effects: none.
