---
id: "01-query-foundation"
title: "Query foundation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/tanstack-query-server-state.md"
---

# Task 01: Query Foundation

## Acceptance

- `@tanstack/react-query`, `@tanstack/react-query-devtools`, and
  `@tanstack/eslint-plugin-query` are installed at `5.101.1` or newer
  compatible v5 versions.
- `QueryProvider` wraps the SPA from `apps/web/src/main.tsx`, exposes the query
  client only for E2E, and does not break `StateProvider`.
- Query cache seeding helpers exist for boot payload/app-state route data and
  `StateHydrator`.

## Verification

- `cd apps && pnpm --filter @kandev/web typecheck`
- `cd apps && pnpm --filter @kandev/web test -- apps/web/src/boot-payload.test.ts apps/web/src/spa-routing.test.ts`
- `cd apps/web && pnpm e2e:docker tests/task/task-list.spec.ts tests/kanban/kanban-board.spec.ts`

## Files Likely Touched

- `apps/web/package.json`
- `apps/pnpm-lock.yaml`
- `apps/web/src/main.tsx`
- `apps/web/components/state-provider.tsx`
- `apps/web/components/state-hydrator.tsx`
- `apps/web/lib/query/client.ts`
- `apps/web/lib/query/provider.tsx`
- `apps/web/lib/query/seed.ts`
- `apps/web/eslint.config.mjs`

## Dependencies

None.

## Inputs

- Spec sections: What, API Surface, Persistence Guarantees.
- Old PR patterns: `origin/pr/1130:apps/web/lib/query/client.ts` and
  `origin/pr/1130:apps/web/lib/query/provider.tsx`.
- Current boundary: `apps/web/src/main.tsx`, not `app/layout.tsx`.

## Output Contract

Update this task to `done`, list files changed, tests run, and any boot-seeding
gaps left for domain tasks.

## Output

- Installed TanStack Query v5 foundation dependencies:
  `@tanstack/react-query`, `@tanstack/react-query-devtools`, and
  `@tanstack/eslint-plugin-query` at `5.101.1`.
- Added `QueryProvider`, browser singleton query-client defaults, query key
  factories, and boot/route-state cache seeding helpers.
- Wrapped the SPA root in `QueryProvider` and seeded cache from the Go boot
  payload before rendering.
- Updated `StateHydrator` to seed the active provider `QueryClient` during
  route transitions, including an injected-client regression test.
- No domain-specific boot-seeding gaps were closed in this task; later domain
  tasks still own full query-option coverage and hook migration.

## Commands Run

- `cd apps/web && pnpm test -- lib/query/client.test.ts lib/query/seed.test.ts lib/query/provider.test.tsx components/state-hydrator.test.tsx src/boot-payload.test.ts src/spa-routing.test.ts`
- `cd apps/web && pnpm typecheck`
- `cd apps/web && pnpm e2e:docker -- tests/system/ws-event-accounting.spec.ts tests/chat/message-add-ws-gap.spec.ts tests/task/task-list.spec.ts tests/kanban/kanban-board.spec.ts`
- `cd apps/web && pnpm e2e:docker --no-build --project mobile-chrome -- tests/chat/mobile-message-add-ws-gap.spec.ts`
