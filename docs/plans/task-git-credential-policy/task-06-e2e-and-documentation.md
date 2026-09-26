---
id: "06-e2e-and-documentation"
title: "Prove credential policy UX and document the boundary"
status: done
wave: 4
depends_on:
  - "03-managed-runtime-tools"
  - "04-settings-explanation"
  - "05-changes-identity-disclosure"
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-authentication.md"
---

# Task 06: Prove Credential Policy UX And Document The Boundary

## Acceptance

- Desktop and mobile E2E prove policy persistence/help plus managed and runtime-selected Changes
  disclosures, including keyboard/touch access, 44px targets, and no horizontal overflow.
- Public GitHub/executor/getting-started documentation separates automation method from task
  credential policy and gives accurate managed-versus-inherited troubleshooting.
- E2E runs use rebuilt production backend/Vite artifacts and record exact results.

## Verification

```bash
make -C apps/backend build
cd apps && pnpm --filter @kandev/web build:vite
cd apps/web && pnpm e2e:run --project chromium tests/integrations/github-workspace-settings.spec.ts tests/git/git-credential-identity.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/integrations/mobile-github-workspace-settings.spec.ts tests/git/mobile-git-credential-identity.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/integrations/github-workspace-settings.spec.ts`
- `apps/web/e2e/tests/integrations/mobile-github-workspace-settings.spec.ts`
- `apps/web/e2e/tests/git/git-credential-identity.spec.ts`
- `apps/web/e2e/tests/git/mobile-git-credential-identity.spec.ts`
- `apps/web/e2e/pages/github-auth-settings-page.ts`
- `apps/web/e2e/helpers/api-client.ts` only if a typed snapshot seed helper is needed
- `docs/public/integrations.md`
- `docs/public/executors.md`
- `docs/public/use-kandev.md`
- this task file and `plan.md`

## Dependencies

Tasks 03, 04, and 05.

## Parallelism

Sequential. It integrates all backend/frontend behavior and owns production E2E evidence.

## Inputs

- All task-policy spec scenarios.
- Completed Task 03 runtime proof, Task 04 settings UI, and Task 05 Changes disclosure.
- Existing GitHub settings page object and `_test/task-sessions` metadata seeding.

## Output contract

Report the exact desktop/mobile E2E scenarios, build and test results, public-doc wording changes,
residual out-of-scope security limits, files changed, and update task/plan status.
