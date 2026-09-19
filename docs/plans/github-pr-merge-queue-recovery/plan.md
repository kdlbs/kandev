---
created: 2026-08-24
status: done
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-001
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003
  - REQ-UI-CI-PR-MERGE-QUEUE-RECOVERY-001
system_design:
  - ../../specs/integrations/system-design/github-pr-merge-queue-recovery.md
  - ../../specs/ui/system-design/ci-pr-merge-queue-recovery-controls.md
legacy_specs: []
---
# Implementation Plan: GitHub PR Merge Queue Recovery

## Overview

Extend the existing GitHub poller with durable queue-removal evidence. Then use
that evidence in the current auto-fix and auto-merge evaluator.

Provider state lands first because retry safety depends on a stable event ID
and head SHA. Runtime automation follows that contract. The shared desktop and
mobile controls then expose the result. E2E work completes the package.

## Scope

### In scope

- Poll GitHub for the latest merge-queue removal event.
- Persist the pull-request head, active queue-attempt identity, and removal
  evidence.
- Send one auto-fix round for each actionable removal.
- Prevent same-head requeue loops.
- Requeue a new eligible head through the existing queue-aware merge action.
- Explain recovery in the current PR hover popover and phone drawer.
- Update the public GitHub integration guide.

### Out of scope

- Merge-group webhooks.
- Automatic flaky-check retries without a new pull-request head.
- A third queue-recovery switch.
- Manual queue removal or priority controls.
- Coordinated requeue ordering for a GitHub pull-request stack.
- GitLab merge requests.

## Technical approach

### Provider snapshot and persistence

Extend `prFieldsBlock` and `batchedPRResult` in
`apps/backend/internal/github/graphql.go`. Read the active queue entry, current
head SHA, and latest `RemovedFromMergeQueueEvent`.

Extend `PRStatus` and `TaskPR` in `apps/backend/internal/github/models.go`.
Add the new SQLite columns and thread them through every TaskPR write path in
`apps/backend/internal/github/store.go`.

Extend `prepareTaskPRSyncState` in
`apps/backend/internal/github/service_pr_watch.go`. Active queue fields clear
on an authoritative null. The last removal remains until a newer event arrives.

### Auto-fix and auto-merge

Extend the CI checkpoint in
`apps/backend/internal/orchestrator/event_handlers_github_ci_automation.go`.
Queue-removal evidence becomes actionable only with failed-check or conflict
evidence.

Map reviewed GitHub removal-reason forms to a normalized cause. Do not treat
the removal event's `beforeCommit` as a merge-group commit. Reuse the current
prompt queue, coalescing key, session binding, and 10-round limit.

Add the head SHA to `ciAutomationMergeSignature`. Store the last queue-attempt
head for Kandev-created and externally observed attempts. A queue removal
blocks the same head. A new eligible head permits one new queue-aware merge
request.

### Responsive controls and copy

Keep two switches in `pr-ci-automation-rows.tsx`. Change the auto-merge label
to `Auto-merge or requeue when ready`.

Adapt the section title, subtitle, and supporting text for normal, queued, and
removed contexts. Keep both switch labels stable. Add one compact,
non-interactive queue status below the switches. Derive all presentation state
in a pure helper from `TaskPR`, per-PR options, and
`TaskCIPRAutomationState`.

Use the normalized removal cause for localized copy. Use generic removal copy
for manual, branch-protection, and unknown causes. Do not use the raw provider
reason as a label.

Update the information control and prompt dialog copy. Add every new key to all
five required locale catalogs. Update the GitHub section in
`docs/public/integrations.md`.

### Integrated test support

Extend the mock GitHub controller and E2E API client with queue-removal and head
transition inputs. Seed an actionable removal, observe one prompt, publish a
new head, and observe one queued merge attempt.

## Tests

- `AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-001.*`: GraphQL conversion,
  SQLite round-trip, sync transitions, and restart-safe reads in
  `apps/backend/internal/github`.
- `AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.*`: evaluator tests for
  reviewed reason forms, conflicts, deduplication, unknown removal, durable
  dispatch, commit-identity safety, and the round limit.
- `AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003.*`: evaluator tests for
  same-head blocking, new-head requeue, enable-while-queued adoption, and
  GitHub rejection.
- `AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.*`: pure state tests and focused
  component tests for labels, help, status, and errors.

## E2E tests

- Desktop: add a merge-queue recovery scenario to
  `apps/web/e2e/tests/pr/ci-automation-options.spec.ts`. Open the hover popover,
  start with an already queued PR, enable both switches, remove the PR, and
  observe repair, wait, and requeue states.
- Mobile: add the same user outcome to
  `apps/web/e2e/tests/pr/mobile-ci-automation-options.spec.ts`. Use touch input
  and the `mobile-chrome` project.
- Both scenarios assert no duplicate request when the options are enabled on
  the active queue entry. They then assert one repair prompt and one queue
  request per eligible transition. The mobile scenario also asserts 44-pixel
  rows, internal drawer scrolling, and no document-level horizontal overflow.

## Work orders

- [x] [Task 01: Persist queue recovery evidence](task-01-persist-queue-recovery-evidence.md)
- [x] [Task 02: Evaluate repair and requeue](task-02-evaluate-repair-and-requeue.md)
- [x] [Task 03: Expose responsive recovery controls](task-03-expose-responsive-recovery-controls.md)
- [x] [Task 04: Prove desktop and mobile recovery](task-04-prove-desktop-and-mobile-recovery.md)

## Verification results

- `go test -tags fts5 ./internal/github ./internal/orchestrator`: 3,770 tests passed.
- `go test -race -tags fts5 ./internal/github`: 1,661 tests passed.
- `make -C apps/backend lint`: passed with zero issues.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Web focused recovery and regression tests: 4 files, 69 tests passed.
- `pnpm run typecheck`: passed.
- Full web lint with `--max-warnings 0`: passed.
- `pnpm run i18n:check`: passed for all five catalogs; existing orphan warnings only.
- Public documentation tests: 61 passed; 41 pages validated.
- Fresh-build desktop recovery E2E: 1 test passed.
- Mobile recovery E2E: 1 test passed.
- `git diff --check`: passed.

## Risks

- GitHub exposes the removal reason as free-form text. Automatic action cannot
  use an unrecognized form. Unknown forms remain visible but do not start a
  repair.
- GitHub does not document the removal event's `beforeCommit` as the temporary
  merge-group commit. The first version does not attach merge-group check logs
  unless an exact commit identity is available.
- A one-minute poll can first observe removal after a later push. The safe
  fallback can delay one automatic requeue instead of risking a retry loop.
- GitHub can remove several stacked pull requests after one lower failure. The
  first version evaluates each linked pull request independently.
