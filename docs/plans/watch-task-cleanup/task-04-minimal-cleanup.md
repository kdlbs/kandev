---
id: "04-minimal-cleanup"
title: "Use minimal PR reads for cleanup"
status: done
wave: 4
depends_on: ["03-sibling-candidates"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-WATCH-CLEANUP-001
acceptance_criteria:
  - AC-INTEGRATIONS-WATCH-CLEANUP-001.6
system_design:
  - ../../specs/integrations/system-design/watch-task-cleanup.md
---

# Task 04: Use minimal PR reads for cleanup

## Summary

Use minimal PR reads for cleanup. Implement with TDD and preserve workspace identity.

## In scope

Replace cleanup-only GetPRFeedback calls with GetPR and conditional review
reads. Fetch viewer identity only when an approval candidate requires it.
Preserve all cleanup policies and approval matching. Propagate each fetch error
to the batch handling from Task 02. Update existing cleanup stubs and tests.

Add TestCleanupUsesMinimalPRReads to service_review_cleanup_test.go. Assert
zero comments, checks, workflow-run, and job reads. Terminal PRs require only
GetPR. Open PRs retain approval-based cleanup. Include rate limits at each
required fetch and preserve partial counts.

## Out of scope

New UI controls, credential policies, global schedulers, and unrelated providers.

## Acceptance

- Meet every linked acceptance criterion with deterministic regressions.
- Preserve existing policy, scope, and error-handling contracts.
- Record the expected red test result and passing package results.

## Verification

Run from the repository root.

```bash
(cd apps/backend && go test ./internal/github -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/github/service_cleanup.go`
- `apps/backend/internal/github/service_review_cleanup_test.go`
- `apps/backend/internal/github/service_cleanup_policy_test.go`

## Dependencies

03-sibling-candidates.

## Risks

Mocks must implement the smaller read path. Preserve existing review approval semantics.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/watch-task-cleanup.md)
- [Design](../../specs/integrations/system-design/watch-task-cleanup.md)
- Source files and existing adjacent tests listed above. New helper/test files are created as needed.

## Results

Implemented the minimal GitHub cleanup read path. Cleanup now fetches PR state
with `GetPR`, fetches reviews only for open PRs, and requests the authenticated
user only when at least one approval review exists. Cleanup no longer requests
comments, check runs, workflow runs, or workflow jobs through the feedback
helper. Fetch errors use the Task 02 batch handling, including rate-limit stop
and partial-count behavior.

Added deterministic coverage for terminal and open PR reads, approval-based
cleanup, historical-row cleanup, and rate limits. The focused red tests failed
before the production change as expected. Verification passed:

```bash
(cd apps/backend && go test ./internal/github -count=1)
```
