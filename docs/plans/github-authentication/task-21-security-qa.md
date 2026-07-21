---
id: "21-security-qa"
title: "Security and QA verification"
status: done
wave: 8
depends_on: ["19-registration-e2e", "20-registration-documentation"]
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 21: Security And QA Verification

## Acceptance

1. Independent security review finds no cross-workspace/registration resolution, pre-HMAC mutation,
   callback state confusion, secret exposure, unsafe deletion, or public-origin bypass.
2. Integrated QA exercises every spec scenario and records any real-GitHub staging-only checks.
3. Formatting, backend tests/lint, frontend typecheck/tests/lint, targeted desktop/mobile E2E, and
   diff checks pass in the required order.

## Verification

```bash
rtk make -C apps/backend fmt
rtk make -C apps/backend test
rtk make -C apps/backend lint
cd apps/web && rtk pnpm run typecheck
cd apps && rtk pnpm --filter @kandev/web test
cd apps && rtk pnpm --filter @kandev/web lint
cd apps/web && rtk pnpm e2e:run tests/integrations/github-authentication.spec.ts -- --project=chromium
cd apps/web && rtk pnpm e2e:run --no-build tests/integrations/mobile-github-auth-settings.spec.ts -- --project=mobile-chrome
test -z "$(rg -l 'KANDEV_GITHUB_APP' apps docs/public || true)"
git diff --check
```

## Results

- Independent review found and fixed personal-token rollback generation, App credential deletion,
  runtime publication race, stale registration selection, and import secret-reset defects.
- Backend GitHub tests pass with and without the race detector: 923 tests.
- Full web tests pass: 5,724 passed and 4 skipped. CLI tests pass: 280 tests.
- Desktop and mobile GitHub authentication and App registration Playwright suites pass.
- Formatting, monorepo typecheck, script tests, full lint, conflict checks, and diff checks pass.
- Real GitHub manifest conversion, installation, webhook delivery, and OAuth callbacks remain
  staging checks because they require a public HTTPS Kandev origin and GitHub-owned credentials.

## Files Likely Touched

- `docs/plans/github-authentication/task-21-security-qa.md`
- `docs/plans/github-authentication/plan.md`
- Focused production/test files only when review finds an actionable defect

## Dependencies

Tasks 19 and 20.

## Inputs

- Entire approved spec and ADR.
- All Task 11-20 output contracts.
- Use security-auditor, QA, simplify, code-review, and verify skills/subagents as applicable.

## Output Contract

Report findings ordered by severity, fixes made, all commands/results, external staging gaps, files
touched, residual risks, and update this task plus `plan.md` to done only when acceptance passes.
