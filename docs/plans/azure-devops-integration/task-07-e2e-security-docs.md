---
id: "07-e2e-security-docs"
title: "E2E, security review, and documentation"
status: done
wave: 4
depends_on: ["06-frontend-browse"]
plan: "plan.md"
spec: "../../specs/azure-devops-integration/spec.md"
---

# Task 07: E2E, Security Review, And Documentation

## Acceptance

- Desktop and mobile Playwright tests prove connection, work-item browsing, PR browsing, and feedback detail against the Azure mock.
- Security review finds no cross-workspace credential use, arbitrary outbound host, secret logging, or unbounded response read.
- Public integration/setup documentation and scoped AGENTS guidance match shipped behavior; full verification passes.

## Verification

- `rtk make build-web` and `rtk make build-backend` from the repository root.
- `rtk pnpm e2e -- --project=chrome e2e/azure-devops/azure-devops.spec.ts` from `apps/web`.
- `rtk pnpm e2e -- --project=mobile-chrome e2e/azure-devops/mobile-azure-devops.spec.ts` from `apps/web`.
- `rtk make -C apps/backend fmt`, then backend test/lint and web test/typecheck/lint commands from the plan.

## Files Likely Touched

- `apps/web/e2e/azure-devops/*.spec.ts`
- `apps/web/e2e/azure-devops/mobile-*.spec.ts`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/fixtures/backend.ts`
- `docs/public/` Azure integration documentation.
- Scoped `AGENTS.md` files only where conventions changed.

## Inputs

- Completed Tasks 01-06.
- `/e2e`, `/mobile-parity`, `/qa`, `/verify`, `/docs-maintainer`, and security-auditor workflows.

## Output Contract

Report scenarios exercised, screenshots/viewport checks, security findings, docs changed, complete verification output, residual risks, and set this task plus its plan checkbox to done.
