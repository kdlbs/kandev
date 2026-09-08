---
id: "03-frontend-quarantine-domain"
title: "Frontend quarantine domain"
status: done
wave: 3
depends_on: ["02-bulk-quarantine-api"]
plan: "plan.md"
spec: "../../specs/system-page/requirements/storage-maintenance.md"
---

# Task 03: Frontend quarantine domain

Wire the bulk purge contract through shared frontend types, API calls, job tracking, and controller
actions.

## Acceptance

- API calls emit the exact eligible/all confirmation payloads and preserve encoded per-entry paths.
- The storage controller exposes `clearEligible` and `forceClearAll`, tracks their shared
  `storage-quarantine-delete` job, refreshes overview/history/quarantine on terminal state, and
  reports action-specific feedback.
- Existing individual restore and eligible deletion behavior remains intact.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- lib/api/domains/system-api.test.ts hooks/domains/system/use-storage-maintenance.test.tsx
```

## Files likely touched

- `apps/web/lib/types/system.ts`
- `apps/web/lib/api/domains/system-api.ts`
- `apps/web/lib/api/domains/system-api.test.ts`
- `apps/web/hooks/domains/system/use-storage-maintenance.ts`
- `apps/web/hooks/domains/system/use-storage-maintenance.test.tsx`

## Dependencies

Task 02.

## Parallelism

Sequential; Task 04 consumes the controller surface.

## Inputs

- Spec: API surface and bulk-result scenarios
- Plan: Types, API client, and domain controller
- Task 02 final request/response contract

## Risks

- Fast jobs may finish before WebSocket delivery; polling and terminal reload must remain active.

## Output contract

Report frontend contract shapes, controller actions/job lifecycle, files changed, exact test
results, blockers/risks, and update this task plus `plan.md` status.
