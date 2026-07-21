---
id: "17-frontend-registration-client"
title: "Frontend registration client"
status: done
wave: 5
depends_on: ["16-workspace-app-lifecycle-api"]
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 17: Frontend Registration Client

## Acceptance

1. Frontend types/API/hooks expose workspace-keyed catalog, create/import/rename/delete/install,
   status, and callback contracts with no singleton deployment types or fetches.
2. Workspace changes clear prior catalog/actor/health/form state before refetch.
3. The System GitHub App route/sidebar/page and deployment-only hook are removed without breaking
   settings navigation.

## Verification

```bash
cd apps/web && rtk pnpm run typecheck
cd apps && rtk pnpm --filter @kandev/web test -- lib/api/domains/github-api.test.ts hooks/domains/github/use-github-status.test.tsx components/settings/settings-layout-client.test.tsx
```

## Files Likely Touched

- `apps/web/lib/types/github.ts`
- `apps/web/lib/api/domains/github-auth-api.ts`
- `apps/web/lib/api/domains/github-api.test.ts`
- `apps/web/hooks/domains/github/use-deployment-app-registration.ts` (remove/replace)
- `apps/web/hooks/domains/github/use-deployment-app-registration.test.tsx` (remove/replace)
- `apps/web/hooks/domains/github/use-github-status.ts`
- `apps/web/hooks/domains/github/use-github-status.test.tsx`
- `apps/web/lib/state/slices/github/types.ts`
- `apps/web/lib/state/slices/github/github-slice.ts`
- `apps/web/src/settings-routes.tsx`
- `apps/web/app/settings/system/github-app/page.tsx` (remove)
- `apps/web/components/app-sidebar/sections/settings/system-group.tsx`

## Dependencies

Task 16.

## Inputs

- Spec: registration catalog/workspace/personal API and UX state isolation.
- Task 16 response/error contracts.
- Follow `apps/web/AGENTS.md` and mobile-parity shared-logic rules.

## Output Contract

Report client/state contract, tests run, files touched, blockers/risks, and update this task plus
`plan.md` to done.
