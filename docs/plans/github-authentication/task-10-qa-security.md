---
id: "10-qa-security"
title: "Integrated QA and security verification"
status: done
wave: 7
depends_on:
  ["01-persistence-migration", "02-app-token-primitives", "03-workspace-credential-resolver",
   "04-app-oauth-webhooks", "05-service-routing", "06-executor-credentials",
   "07-http-health-mocks", "08-frontend-settings", "09-e2e-docs"]
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 10: Integrated QA And Security Verification

## Inputs

- Full spec, ADR 0047, completed Tasks 01-09, QA/security/code-review/verify skills.
- Final Verification and Risks sections in `plan.md`.

## Acceptance

- A conformance pass exercises every spec scenario, including concurrent isolation, token expiry,
  callback replay, webhook replay, missing permissions, wrong repository, and every supported
  executor credential path.
- Security review finds no cross-workspace/user fallback, confused-deputy repository expansion,
  unverified callback binding, or secret exposure in APIs, logs, args, env, DB metadata, snapshots,
  and generated diagnostics.
- Backend format/test/lint, frontend typecheck/test/lint, and focused desktop/mobile E2E all pass;
  spec, ADR, plan, task statuses, public docs, and scoped guidance agree with shipped behavior.

## Files Likely Touched

- Focused regression tests in files owned by Tasks 01-09
- `docs/specs/integrations/github-authentication.md`
- `docs/plans/github-authentication/plan.md`
- All task frontmatter statuses
- Relevant `AGENTS.md` files only when implementation established a durable convention

## Verification

```bash
rtk make -C apps/backend fmt
rtk make -C apps/backend test
rtk make -C apps/backend lint
cd apps/web && rtk pnpm run typecheck
cd apps && rtk pnpm --filter @kandev/web test
cd apps && rtk pnpm --filter @kandev/web lint
cd apps/web && rtk pnpm exec playwright test --config e2e/playwright.config.ts e2e/tests/integrations/github-authentication.spec.ts --project=chromium
cd apps/web && rtk pnpm exec playwright test --config e2e/playwright.config.ts e2e/tests/integrations/mobile-github-auth-settings.spec.ts --project=mobile-chrome
```

Completed on 2026-07-19. Backend format, tests, lint, binaries, the Sprites-tag compile check,
frontend typecheck, lint, build, all 5,380 frontend tests, public-doc validation, and focused
desktop/mobile E2E passed. Concurrency and replay regressions cover OAuth callbacks, personal token
generation, webhook delivery and repository changes, broker leases, executor reconnect, and exact
broker readiness. The final review found no blocking cross-workspace fallback, callback binding,
repository expansion, or secret exposure issue. The only residual check is a real deployed GitHub
App installation/OAuth/webhook round trip in staging.

## Output Contract

Record every command and environment covered, security findings and fixes, residual staging checks,
files touched, and readiness. Mark the plan done and spec shipped only when all acceptance criteria
pass.
