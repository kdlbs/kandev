---
id: "01-navigation-controller"
title: "User message navigation controller"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/user-message-navigation.md"
---

# Task 01: User Message Navigation Controller

## Acceptance

- List-level navigation orders every rendered `author_type=user` prompt and selects the adjacent stop from the current viewport origin.
- One previous action loads successive 20-row pages until it finds a stop, confirms exhaustion, or makes no progress; next never loads a page.
- Busy, known-boundary, duplicate-action, failure, and session-change cancellation behavior matches the spec.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run hooks/use-message-navigation.test.ts
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/hooks/use-message-navigation.ts`
- `apps/web/hooks/use-message-navigation.test.ts`
- `apps/web/hooks/use-lazy-load-messages.ts` only if a backward-compatible load result is required

## Dependencies

None.

## Inputs

- Spec `What`, `Failure Modes`, and boundary/pagination scenarios.
- Existing generation cancellation in `apps/web/hooks/domains/session/use-session-search.ts`.
- Existing bounded drain loop in `apps/web/components/task/chat/use-drain-older-messages.ts`.
- Existing pagination contract in `apps/web/hooks/use-lazy-load-messages.ts`.

## Output Contract

Report controller API, files changed, focused tests and typecheck run, blockers, and residual risks; set this task to `done` and tick it in `plan.md`.
