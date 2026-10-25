---
id: "06-storage-quarantine-e2e"
title: "Storage quarantine E2E"
status: done
wave: 6
depends_on:
  - "04-responsive-quarantine-ui"
  - "05-operator-documentation"
plan: "plan.md"
spec: "../../specs/system-page/requirements/storage-maintenance.md"
---

# Task 06: Storage quarantine E2E

Prove the user-visible quarantine lifecycle on desktop and Pixel 5 mobile layouts.

## Acceptance

- Desktop coverage shows a protected real quarantine entry, its deadline, and successful
  force-clear removal; eligible-only coverage verifies protected remainder and completed-job
  feedback.
- Mobile coverage reaches both bulk confirmations with touch interactions, asserts the distinct
  phrases/outcomes, and proves no document horizontal overflow.
- Production Vite and Go artifacts are rebuilt by the managed runner before the focused specs run.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/system/storage-maintenance.spec.ts tests/system/mobile-storage-maintenance.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/system/storage-maintenance.spec.ts`
- `apps/web/e2e/tests/system/mobile-storage-maintenance.spec.ts`

## Dependencies

Tasks 04 and 05.

## Parallelism

Sequential final validation.

## Inputs

- Spec: desktop and mobile quarantine scenarios
- Plan: E2E Tests and Mobile design contract
- Existing orphan-workspace and managed-Go-cache seed helpers

## Risks

- E2E setup cannot wait through a production retention period; use the real filesystem path for
  force deletion and keep time-controlled eligible deletion integration in backend tests.

## Output contract

Report scenarios, production-build runner command/result, screenshots or failure artifacts
inspected, files changed, blockers/risks, and update this task plus `plan.md` status.
