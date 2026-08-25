---
id: "04-prove-desktop-and-mobile-recovery"
title: "Prove desktop and mobile recovery"
status: done
wave: 4
depends_on:
  - "03-expose-responsive-recovery-controls"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003
  - REQ-UI-CI-PR-MERGE-QUEUE-RECOVERY-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.1
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.3
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003.2
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003.3
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003.7
  - AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.8
  - AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.9
  - AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.10
  - AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.11
  - AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.12
  - AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.13
  - AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.14
  - AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.15
system_design:
  - ../../specs/integrations/system-design/github-pr-merge-queue-recovery.md
  - ../../specs/ui/system-design/ci-pr-merge-queue-recovery-controls.md
---
# Task 04: Prove Desktop and Mobile Recovery

## Summary

Add deterministic mock GitHub transitions for queue removal and a new head.
Prove the same repair-and-requeue outcome through desktop hover and mobile tap
surfaces.

## In scope

- Extend mock GitHub seeding for queue entry, removal, head, and check evidence.
- Add API-client helpers for the E2E transitions.
- Add a desktop hover-popover recovery scenario.
- Add a mobile drawer recovery scenario.
- Start both scenarios with the pull request already in the queue.
- Assert contextual title, subtitle, supporting text, and status transitions.
- Assert the localized classified cause and generic unknown-cause fallback.
- Prove deduplication, touch geometry, scroll ownership, and overflow behavior.

## Out of scope

- Live writes to GitHub.
- Product-video or screenshot capture.
- Full merge-queue management.

## Acceptance

- One removal produces one visible repair round on desktop and mobile.
- Enabling both options on an active entry produces no second queue request.
- The same head does not requeue. A new eligible head queues once.
- The mobile drawer keeps 44-pixel rows and no document overflow.

## Verification

```bash
pnpm e2e:run tests/pr/ci-automation-options.spec.ts -- --grep "merge queue recovery"
pnpm e2e:run --no-build --project mobile-chrome tests/pr/mobile-ci-automation-options.spec.ts -- --grep "merge queue recovery"
```

Run the commands from `apps/web`. Run the first command with a fresh build.

## Files likely touched

- `apps/backend/internal/github/mock_client.go`
- `apps/backend/internal/github/mock_controller.go`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/tests/pr/ci-automation-options.spec.ts`
- `apps/web/e2e/tests/pr/mobile-ci-automation-options.spec.ts`

## Dependencies

- Task 03 completes the integrated backend and UI behavior.

## Risks

- The mock must reproduce the provider event order without fixed sleeps.
- The desktop test must reset pointer state before hover assertions.

## Parallelism

`sequential`

## Inputs

- Both system designs.
- Existing CI automation and merge-queue E2E fixtures.

## Results

Added deterministic mock GitHub merge-queue transitions and merge-attempt
inspection. The mock provider now carries queue membership, removal evidence,
head changes, check runs, and queued merge outcomes through the normal status
sync and event bus.

Added desktop hover-popover and mobile drawer scenarios. Both start with an
active queue entry, enable both existing switches without a duplicate merge
request, observe one classified actionable repair round, block a same-head
requeue, queue exactly one new eligible head, and observe the resulting active
queue entry. The mobile scenario also uses touch input and verifies 44-pixel
switch rows, drawer-owned scrolling, and no document-level horizontal
overflow. GitHub selectors and component expectations were updated for the
new auto-merge/requeue label.

Verification:

- `pnpm e2e:run tests/pr/ci-automation-options.spec.ts -- --grep "merge queue recovery"`: fresh-build desktop run passed (1 test).
- `pnpm e2e:run --no-build --project mobile-chrome tests/pr/mobile-ci-automation-options.spec.ts -- --grep "merge queue recovery"`: mobile run passed (1 test).
