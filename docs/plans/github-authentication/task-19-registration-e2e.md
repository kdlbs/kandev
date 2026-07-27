---
id: "19-registration-e2e"
title: "Registration E2E coverage"
status: done
wave: 7
depends_on: ["18-workspace-authentication-ux"]
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 19: Registration E2E Coverage

## Acceptance

1. Playwright proves different Apps in two workspaces, intentional known-App reuse,
   create/import/install cancellation/failure, and personal cleanup on switch.
2. Old System Settings registration specs are removed or rewritten as workspace settings flows.
3. Desktop and mobile screenshots are inspected; horizontal overflow is zero and all auth actions
   remain usable with 44px targets and no overlap.

## Verification

```bash
cd apps/web && rtk pnpm e2e:run tests/integrations/github-authentication.spec.ts -- --project=chromium
cd apps/web && rtk pnpm e2e:run --no-build tests/integrations/mobile-github-auth-settings.spec.ts -- --project=mobile-chrome
```

## Files Likely Touched

- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/fixtures/backend.ts`
- `apps/web/e2e/tests/integrations/github-authentication.spec.ts`
- `apps/web/e2e/tests/integrations/mobile-github-auth-settings.spec.ts`
- `apps/web/e2e/tests/settings/github-app-registration.spec.ts` (remove/merge)
- `apps/web/e2e/tests/settings/mobile-github-app-registration.spec.ts` (remove/merge)
- `apps/web/e2e/helpers/layout-assertions.ts`

## Dependencies

Task 18.

## Inputs

- Spec scenarios and **Success Criteria**.
- Task 16 mock/reset API and Task 18 stable UI locators.
- Use the E2E and mobile-parity skills.

## Output Contract

Report scenarios, commands/results, screenshots/overflow checks, files touched, external staging
gaps, blockers/risks, and update this task plus `plan.md` to done.
