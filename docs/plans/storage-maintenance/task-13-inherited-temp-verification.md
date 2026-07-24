---
id: "13-inherited-temp-verification"
title: "Review and verify inherited temp behavior"
status: done
wave: 9
depends_on: ["10-inherit-service-temp", "11-e2e-temp-cleanup", "12-inherited-temp-docs"]
plan: "plan.md"
spec: "../../specs/system-page/storage-maintenance.md"
---

# Task 13: Review and verify inherited temp behavior

Review the simplified implementation for environment, process-lifecycle, cache, concurrency, and
documentation regressions, then run repository-wide verification.

## Acceptance

- Review proves no application path recreates per-instance `kandev-agent` temp roots or injects
  instance-specific temp variables, while service overrides and managed `GOCACHE` remain correct.
- QA covers concurrent agents sharing temp-derived caches, archive/delete process reaping, E2E
  fixture cleanup, and absence of the superseded Storage API/UI surface.
- Formatting, backend tests/lint, frontend typecheck/tests/lint, focused desktop/mobile E2E, public
  docs validation, and `git diff --check` pass or exact environmental blockers are reported.

## Verification

```bash
make -C apps/backend fmt
make -C apps/backend test
make -C apps/backend lint
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web test
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm e2e:run tests/system/storage-maintenance.spec.ts
cd apps/web && pnpm e2e:run tests/system/mobile-storage-maintenance.spec.ts -- --project=mobile-chrome
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- only files required by review, simplification, QA, verification, and docs reconciliation findings

## Dependencies

Tasks 10–12.

## Inputs

- Completed implementation packets
- `/code-review`, `/simplify`, `/qa`, `/verify`, and ADR 0045

## Output contract

Report findings/fixes, commands/results, residual collision/legacy-cleanup risks, final spec/plan
status, and update this task plus the serialized plan checkbox when complete.

## Completion

- Removed the obsolete instance-resource release classification that existed only for the retired
  agent-temp cleanup error. Every unresolved process teardown now retains the instance and port for
  retry; focused tests and the process/instance race suites pass.
- Removed the stale VS Code temp-setup failure test and reconciled the earlier workspace Git-status
  plan so it records that per-instance temp teardown was superseded by ADR 0045's amendment.
- Hardened backend fixture health polling for asynchronous child-process spawn errors. The fixture
  lifecycle suite passes all six setup, spawn, health, success, ordering, isolation, and cleanup
  error cases.
- Confirmed production searches contain no agent-temp root creator, temp-variable injector,
  cleanup classifier, Storage resource/API/UI symbol, or marker implementation. Managed `GOCACHE`
  injection remains confined to its existing opt-in lifecycle provider paths.
- Formatting, repository-wide typecheck/tests/lint, desktop Storage E2E (3 tests), mobile Storage
  E2E (1 test), public-doc tests (58 tests), public-doc validation (40 pages), race tests, and
  `git diff --check` pass. Both managed E2E runs left the pre-existing `/tmp/kandev-e2e-*` count
  unchanged at three.
- Residual boundaries are deliberate: shared-temp tool collisions require narrow tool-specific
  fixes when observed; explicit Unix service temp paths must remain short enough for local socket
  limits; an uncatchable hard kill can still leave E2E scratch for host policy; and inactive legacy
  `/tmp/kandev-agent/*` roots require deliberate stopped-service cleanup.
