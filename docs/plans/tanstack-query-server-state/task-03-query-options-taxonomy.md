---
id: "03-query-options-taxonomy"
title: "Query options taxonomy"
status: done
wave: 2
depends_on: ["01-query-foundation"]
plan: "plan.md"
spec: "../../specs/ui/tanstack-query-server-state.md"
---

# Task 03: Query Options Taxonomy

## Acceptance

- `apps/web/lib/query/keys.ts` defines typed, serializable key factories for
  every server-state domain.
- `apps/web/lib/query/query-options/*` defines query option factories over the
  existing domain API clients.
- Unit tests cover key stability, representative API mapping, and infinite
  query cursor behavior.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- apps/web/lib/query`
- `cd apps && pnpm --filter @kandev/web typecheck`
- No standalone E2E gate before readers consume these factories; complete Wave
  2 only after the Docker E2E gate in `plan.md` passes.

## Files Likely Touched

- `apps/web/lib/query/keys.ts`
- `apps/web/lib/query/query-options/*.ts`
- `apps/web/lib/query/query-options/*.test.ts`
- `apps/web/lib/api/domains/*`

## Dependencies

- Task 01.

## Inputs

- Old PR: `origin/pr/1130:apps/web/lib/query/keys.ts` and
  `origin/pr/1130:apps/web/lib/query/query-options/*`.
- TanStack docs: query keys, query options, important defaults, testing.

## Output Contract

Update this task to `done`, list covered domains, and list any server-state
domain intentionally deferred to a later task.

## Output

- Expanded `apps/web/lib/query/keys.ts` with serializable key factories for
  boot/features, workspaces/repositories/branches, workflows/tasks, sessions,
  settings, office, integrations, and system data.
- Added query option factories under `apps/web/lib/query/query-options/` for
  features, workspace, kanban/tasks, session, settings, office, and system
  reads.
- Added infinite query factories for workspace task pages, office tasks, and
  session messages. Cursor/page values are page params, not stable filter keys.
- Added a query-options barrel export.

## Deferred Domains

- Integration-specific query option factories are keyed but deferred to
  `task-09-integrations-automations-system` so each integration can migrate its
  readers and mutations together.
- Runtime stream surfaces such as terminal/process output are keyed/allowlisted
  through the bridge audit work but intentionally remain outside QueryClient
  until `task-08-session-runtime-streams` decides which streams should stay
  outside TanStack Query.

## Commands Run

- `cd apps/web && pnpm test -- lib/query/keys.test.ts lib/query/query-options/query-options.test.ts`
- `cd apps/web && pnpm test -- lib/query lib/ws` — 23 files, 168 tests passed
- `cd apps/web && pnpm typecheck`
