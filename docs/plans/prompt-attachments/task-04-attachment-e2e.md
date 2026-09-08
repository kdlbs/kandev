---
id: "04-attachment-e2e"
title: "Attachment E2E coverage"
status: pending
wave: 4
depends_on: ["03-frontend-staged-uploads"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/prompt-attachments.md"
---

# Task 04: Attachment E2E coverage

## Acceptance

1. A desktop task-creation scenario uploads a file above the former 10 MiB
   limit, creates the task, and verifies transcript metadata plus the
   agent-visible workspace path.
2. A `mobile-chrome` chat scenario completes the same above-10-MiB user outcome
   and proves a synthetic 100 MiB + 1 rejection is contained, localized,
   touch-reachable, and introduces no horizontal overflow.
3. A focused failure/expiry scenario preserves prompt text and successfully
   recovers through retry or reattachment; every new test is observed failing
   before the implementation fix and then passing against a fresh production
   build.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm e2e:run tests/task/create-task-attachment-warning.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-attachment-size-warning.spec.ts
```

If the scenarios are renamed or split, replace the commands above with the
final exact spec paths and confirm Playwright discovers the intended test count.

## Files likely touched

- `apps/web/e2e/tests/task/create-task-attachment-warning.spec.ts`
- `apps/web/e2e/tests/chat/mobile-attachment-size-warning.spec.ts`
- `apps/web/e2e/helpers/api-client.ts` only if attachment seeding/control needs
  a reusable API method
- The nearest existing page object only if repeated UI actions justify it

## Dependencies

- Task 03 completes the end-to-end backend/frontend flow.

## Parallelism

Parallel-safe with task 05 after task 03. This task owns E2E files only and does
not change product, schema, shared package, lockfile, or documentation files.

## Inputs

- Spec scenarios, especially 22.8 MiB success, exact/oversize behavior, reload,
  expiry, and mobile parity
- Plan: E2E Tests and Mobile design contract
- Task 03 final test IDs and rendered states

## Output contract

Report RED and GREEN commands/results, discovered test counts, screenshots/error
artifacts inspected, files changed, cleanup/teardown evidence, blockers, risks,
and synchronized task/plan status.

## Results

Updated the existing desktop task-create and mobile attachment-warning scenarios to use the exact 100 MiB + 1-byte rejection boundary. Dedicated successful large-file and expiry-recovery Playwright scenarios remain follow-up work because the E2E suite was not run in this turn.
