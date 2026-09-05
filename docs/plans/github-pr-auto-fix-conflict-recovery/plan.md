---
created: 2026-09-05
status: done
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002
  - REQ-UI-CI-PR-AUTOMATION-001
  - REQ-UI-CI-PR-MERGE-QUEUE-RECOVERY-001
system_design:
  - ../../specs/integrations/system-design/github-pr-auto-fix-conflicts.md
  - ../../specs/integrations/system-design/github-pr-merge-queue-recovery.md
  - ../../specs/ui/system-design/ci-pr-automation-01.md
  - ../../specs/ui/system-design/ci-pr-merge-queue-recovery-controls.md
legacy_specs: []
---
# Implementation Plan: GitHub PR Auto-Fix Conflict Recovery

## Overview

Add ordinary merge conflicts to the per-PR auto-fix checkpoint. Correct the
queue-removal guard so a retained failed-check removal can start repair after
Kandev misses the active queue entry.

Backend signal behavior lands first. Localized help and public documentation
then describe the complete trigger set. Desktop and mobile E2E tests prove the
user outcome last.

## Confirmed root cause

`ciAutomationBuildDelta` includes failed checks and comments. It does not
include `TaskPR.MergeableState == "dirty"`, so an ordinary conflict produces an
empty delta and no prompt.

`ciAutomationQueueRemovalBelongsToCurrentHead` also requires a non-empty
`last_merge_signature`. A removal-only poll records the current-head baseline,
but it has no signature. The evaluator therefore rejects a retained actionable
removal when Kandev missed the active queue entry.

## Scope

### In scope

- Add ordinary merge conflicts to the current auto-fix checkpoint and prompt.
- Deduplicate stable conflicts across polls and restarts.
- Re-arm conflict repair after resolution, a new head, or a target change.
- Accept failed-check queue removals from a durable removal-only current-head
  baseline.
- Preserve queue-removal event deduplication and same-head requeue protection.
- Update localized help and public GitHub automation documentation.
- Prove the behavior through backend tests and responsive E2E tests.

### Out of scope

- Changing the auto-fix switch label.
- Prompting before pull-request checks settle.
- Automatic selection of a merge strategy.
- Same-head requeue after a merge-queue removal.
- GitLab merge-request behavior.
- A database migration or new automation option.

## Technical approach

### Auto-fix checkpoint

Extend `ciAutomationCheckpoint` in
`apps/backend/internal/orchestrator/event_handlers_github_ci_automation.go` with
an additive conflict snapshot. Include the normalized state, head commit, head
branch, and base branch.

Build the conflict delta from `TaskPR` beside the current full-feedback delta.
Refresh the checkpoint when a prior conflict clears. Render conflict details
through the existing sanitized `{{pr.feedback}}` snapshot.

### Merge-queue removal eligibility

Change `ciAutomationQueueRemovalBelongsToCurrentHead` to accept a non-empty
`last_queue_attempt_head_sha` that equals the current pull-request head. Do not
require `last_merge_signature` for auto-fix.

Keep `last_queue_fix_event_id` as the repair deduplication identity. Keep the
existing same-head auto-merge guard and merge-attempt journal unchanged.

### Help and documentation

Update `autoFixRoundExplanation` and `autoFixPromptDescription` in all required
GitHub locale catalogs. The text must name ordinary conflicts and actionable
queue removals without changing the switch label.

Update the GitHub automation sections in `docs/public/integrations.md` and
`docs/public/sessions-and-review.md`. These explanation pages must describe the
same trigger and deduplication behavior.

### Responsive evidence

Reuse the existing PR status popover on desktop and PR status drawer on phones.
No composition or geometry changes are planned.

Extend the existing mock GitHub E2E flow with a `dirty` mergeability update.
Enable auto-fix through each responsive surface and assert one visible repair
round. Repeat the same snapshot and assert that no duplicate round appears.

## Tests

- `AC-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001.1` through `.8`: add
  orchestrator tests for conflict delta creation, prompt rendering, stable-state
  deduplication, prompt-free clearing, and re-arming.
- `AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.1`, `.3`, `.7`, and `.9`:
  update the queue-recovery test to start from a removal-only current-head
  baseline and prove one failed-check repair round.
- `AC-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001.9`: assert the shared help
  content in the existing desktop and mobile automation E2E files.
- `AC-UI-CI-PR-AUTOMATION-001.8` and
  `AC-UI-CI-PR-MERGE-QUEUE-RECOVERY-001.2`: prove that both responsive surfaces
  explain the complete trigger set and queue-recovery behavior.

The first failing backend regressions are
`TestHandleTaskPRCIAutomationAutoFixesConflict` and
`TestCIAutomationMergeQueueRecoveryAcceptsRemovalOnlyBaseline`.

## E2E tests

- Desktop: extend `apps/web/e2e/tests/pr/ci-automation-options.spec.ts` with a
  merge-conflict auto-fix scenario in the existing hover popover.
- Mobile: extend
  `apps/web/e2e/tests/pr/mobile-ci-automation-options.spec.ts` with the same
  outcome through the existing touch drawer.
- Both scenarios use stable test IDs, assert the visible conflict snapshot and
  help text, and prove that one unchanged poll does not add a second round.

The mobile test uses the current PR status drawer as its nearest exemplar. The
drawer remains the only scroll owner, and the existing 44-pixel switch rows are
unchanged.

## Work orders

- [x] [Task 01: Correct auto-fix signals](task-01-correct-auto-fix-signals.md)
- [x] [Task 02: Explain auto-fix signals](task-02-explain-auto-fix-signals.md)
- [x] [Task 03: Prove responsive auto-fix](task-03-prove-responsive-auto-fix.md)

## Verification results

Passed:

- `go test -tags fts5 ./internal/orchestrator -run 'Test(CIAutomationConflictSignal|HandleTaskPRCIAutomationAutoFixesConflict|CIAutomationMergeQueueRecoveryAcceptsRemovalOnlyBaseline|CIAutomationMergeQueueRecoveryQueuesOneRepairPerRemoval)$'` from `apps/backend`.
- `go test -tags fts5 ./internal/orchestrator -run 'TestCIAutomation(ConflictSignal|RenderSnapshotIncludesConflict)|TestHandleTaskPRCIAutomationAutoFixesConflict' -count=1` from `apps/backend`.
- `pnpm run i18n:check` from `apps/web`.
- `node --test scripts/validate-public-docs.test.mjs` and `node scripts/validate-public-docs.mjs` from the repository root.
- `python3 scripts/lint-spec-files.py --all` from the repository root.
- `pnpm e2e:run --no-build tests/pr/ci-automation-options.spec.ts -- --grep 'merge conflict|merge queue recovery'` from `apps/web`: 2 passed.
- `pnpm e2e:run --no-build --project mobile-chrome tests/pr/mobile-ci-automation-options.spec.ts -- --grep 'merge conflict|merge queue recovery'` from `apps/web`: 2 passed.

## Risks

- A stale removal event can appear after the active queue entry disappears.
  The current-head baseline and event ID bound repair to one retained event.
- GitHub can report mergeability as unknown during recomputation. Only `dirty`
  starts conflict repair, and unknown state must not clear prior deduplication.
- A conflict and failed checks can appear in one evaluation. The combined
  checkpoint must consume one round, not two.
- Localized help can grow inside the phone drawer. The focused mobile E2E test
  must confirm that the current scroll and overflow behavior remains valid.
