---
created: 2026-09-23
status: complete
requirements:
  - REQ-INTEGRATIONS-GITHUB-REVIEW-CLEANUP-001
  - REQ-PLATFORM-PROVIDER-BACKOFF-001
system_design:
  - ../../specs/integrations/system-design/github-review-task-cleanup.md
  - ../../specs/platform/system-design/pr-watch-and-bounded-storage.md
legacy_specs: []
---

# Implementation plan: Bound GitHub review cleanup polling

## Overview

Exclude archived review tasks from scheduled cleanup, then bound feedback
requests from the remaining eligible records. The first work order removes
the recurring load that caused the reported quota incident. The second closes
the review-cleanup backoff gap left by PR #3319. Work orders are sequential
because both change the scheduled cleanup path.

## Evidence and requirement conformance

The five-minute `reviewQueueLoop` calls `CleanupMergedReviewTasks` for enabled
watches and `CleanupAllOrphanedReviewTasks` for disabled watches. Both paths
select historical rows without archive filtering. `shouldDeleteReviewTaskWithClient`
fetches full PR feedback before it checks for user-authored messages or enabled
lifecycle prompts. Under Auto, those conditions retain the row, so the next
cycle repeats the request. A temporary SQLite-backed Go repro confirmed two
feedback calls across two cleanup runs for one archived, merged review task
with a user-authored message. The repro file was removed after the test passed.

The PR-watch circuit applies to task-owned PR watches only. Review cleanup
does not consult it, and its feedback failures are swallowed inside the
cleanup batch. The review loop waits for Search capacity, while feedback
primarily consumes Core capacity. A successful PAT feedback request for a
merged PR normally makes six REST reads before pagination. The reported 89
archived and eight active rows are user-provided counts; production request
headers were not available to verify them.

The archive rule is missing from Integrations requirements. Platform's active
`REQ-PLATFORM-PROVIDER-BACKOFF-001` already requires classified backoff for
background integration loops. This package adds the Integrations rule and
extends the existing Platform design; it does not create a second backoff
requirement or change the established ownership boundary.

## Scope

### In scope

- Skip feedback and deletion for archived review tasks in scheduled cleanup.
- Keep incomplete reservations and missing-task records eligible for recovery.
- Avoid scheduled Auto feedback when local task state already requires retention.
- Preserve explicit cleanup, watch deletion, and reset behavior.
- Admit scheduled feedback by Core quota and a separate review-cleanup circuit.
- Stop further requests in the affected scope after a classified failure.
- Add focused backend regression tests and clarify public integration guidance.

### Out of scope

- Live database edits, deletion of historical rows, or a schema migration.
- Changes to issue-watch cleanup, PR-watch identity, or search query matching.
- New UI controls, copy, settings, or credential sources.
- Changes to manual cleanup admission or explicit destructive watch actions.

## Technical approach

Task 01 adds scheduled-only eligibility selectors in
`apps/backend/internal/github/store.go` and uses them in the scheduled review
cleanup service paths. Keep complete inventory selectors for manual cleanup,
watch deletion, and reset. Apply Auto retention checks before scheduled
feedback. Update `docs/public/integrations.md` to state that archived review
tasks pause routine cleanup while explicit reset can still delete them.

Task 02 keeps a review-cleanup circuit separate from the PR-monitor circuit.
Add a poller-facing cleanup result that reports classified feedback failures
without changing public cleanup responses. Apply Core quota admission before
feedback. Stop the remaining workspace batch on shared auth or rate failure;
isolate PR-specific configuration failures to their record. Credential or
watch changes reset the relevant circuit. A successful search or PR-monitor
call cannot clear a review-cleanup failure. Keep metrics bounded and free of
identifiers.

## Tests

| Acceptance criterion | Planned evidence |
| --- | --- |
| `AC-INTEGRATIONS-GITHUB-REVIEW-CLEANUP-001.1` | SQLite-backed enabled and disabled watch tests assert no archived-row feedback across two scheduled cycles. |
| `AC-INTEGRATIONS-GITHUB-REVIEW-CLEANUP-001.2` | Unarchive test asserts one retained dedup row becomes eligible without a duplicate task. |
| `AC-INTEGRATIONS-GITHUB-REVIEW-CLEANUP-001.3` | Mixed active, archived, empty-reservation, and missing-task fixture checks eligible rows and orphan recovery. |
| `AC-INTEGRATIONS-GITHUB-REVIEW-CLEANUP-001.4` | Auto tests assert zero feedback for user messages and lifecycle prompts, including a failed local check. |
| `AC-INTEGRATIONS-GITHUB-REVIEW-CLEANUP-001.5` | Manual cleanup and reset tests assert archived records remain in complete inventories. |
| `AC-PLATFORM-PROVIDER-BACKOFF-001.1` through `.4` | Poller tests assert same-cycle stop, separate scopes, fingerprint reset, and one due probe. |

Metrics tests also assert bounded labels with no task, watch, PR, repository,
or workspace IDs. Existing credential and quota health output stays intact.

## End-to-end evidence

The backend poller test drives the scheduled entry point through the real
SQLite store and a counting GitHub client. It proves the user-visible task and
record remain intact without provider feedback. No rendered UI changes, so a
browser E2E test would not exercise the defect more faithfully.

## Work orders

- [x] [Task 01: Make scheduled cleanup archive-aware](task-01-archive-eligibility.md)
- [x] [Task 02: Bound eligible cleanup requests](task-02-cleanup-backoff.md)

## Verification results

Task 01 targeted cleanup tests and public-doc validators passed. Task 02
focused cleanup/circuit tests and the complete `internal/github` package
tests passed. The backend build passed, including a post-review-fix build of
`cmd/kandev`. Regression coverage confirms routine watch polling keeps a
record circuit open, a real configuration edit resets it, and an expired Core
quota snapshot allows feedback again. Public-doc validators and `git diff
--check` passed. Backend-wide `make test` completed with two failures in the
unchanged `internal/agentctl/server/process/probe` package; config-discovery
tests that picked up task-injected home config passed when rerun with isolated
HOME and config paths unset.

## Risks

- Filtering shared inventory methods would hide archived tasks from explicit
  deletion and reset. Scheduled cleanup needs its own selectors.
- An inner join would hide empty reservations and hard-deleted-task records.
- Feedback makes several concurrent upstream reads; one failed fetch may have
  multiple in-flight requests. The circuit must stop later records in the
  same scope.
- A success from search or the PR monitor must not clear a cleanup failure.
- Core quota is tracked across credentials in the current tracker, so a Core
  exhaustion pause is conservative across workspaces.
