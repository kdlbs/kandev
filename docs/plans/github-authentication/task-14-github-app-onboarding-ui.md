---
id: "14-github-app-onboarding-ui"
title: "GitHub App onboarding UI"
status: done
wave: 10
depends_on: ["13-deployment-app-runtime-api"]
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 14: GitHub App Onboarding UI

## Inputs

- Task 13 API contract.
- Spec: deployment setup scenarios, identity explanation, workspace handoff, and desktop/mobile
  parity.
- Mobile contract in `plan.md`; nearest patterns are System page routes/sidebar and the existing
  mobile GitHub connection dialog.

## Acceptance

1. **System > GitHub App** presents none/environment/managed/invalid/webhook states, owner and public
   URL setup, permission disclosure, external form POST, callback result, and safe removal without
   rendering any secret.
2. Workspace GitHub connection treats an unconfigured App as an actionable route to System setup and
   clearly distinguishes deployment automation from PAT/CLI human identity.
3. Desktop and mobile share behavior; the direct page has one scroll owner, 44px touch targets,
   keyboard/focus support, and no horizontal overflow.

## Files Likely Touched

- `apps/web/app/settings/system/github-app/page.tsx` (new)
- `apps/web/src/settings-routes.tsx`
- `apps/web/components/app-sidebar/sections/settings/system-group.tsx`
- `apps/web/components/settings/system/github-app-settings.tsx` (new)
- `apps/web/components/settings/system/github-app-settings.test.tsx` (new)
- `apps/web/components/github/github-connection-dialog.tsx`
- `apps/web/components/github/github-connection-dialog.test.tsx` (new or existing)
- `apps/web/lib/api/domains/github-auth-api.ts`
- `apps/web/lib/api/domains/github-api.test.ts`
- `apps/web/lib/types/github.ts`

## Verification

```bash
cd apps/web && rtk pnpm test -- components/settings/system/github-app-settings.test.tsx components/github/github-connection-dialog.test.tsx lib/api/domains/github-api.test.ts
cd apps/web && rtk pnpm run typecheck
cd apps && rtk pnpm --filter @kandev/web lint
```

## Dependencies

Task 13.

## Output Contract

Report each responsive state, focus/touch/scroll decisions, unit coverage, files touched, commands
run, blockers, and remaining rendered-verification needs. Mark this task `done` and update `plan.md`
only after targeted tests pass.
