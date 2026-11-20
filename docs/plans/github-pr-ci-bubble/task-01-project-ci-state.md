---
id: "01-project-ci-state"
title: "Retain check summaries and project CI state"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.2
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.3
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.4
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.5
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.6
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.7
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.8
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.12
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.13
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.14
  - AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001.17
system_design:
  - ../../specs/integrations/system-design/github-pr-ci-bubble.md
---

# Task 01: Retain check summaries and project CI state

## Summary

Make independent CI display state available through compact task rows and full PR records.
Retain enough selected check metadata to distinguish skipped-only results without changing check action semantics.
This additive contract can pass independently before any rendered UI changes.

## In scope

- Use `/tdd`. Add the selected-check, persistence, sync, projection, and mapping tests before changing production behavior.
- Add the design's optional head-scoped `check_summary`, including observed and completed-skipped counts.
- Populate it through every detailed status constructor, including feedback persistence and watch snapshots that copy detailed results.
- Preserve same-head metadata on lightweight sync. Clear it on a new head and replace it on an authoritative detailed empty result.
- Add the bounded JSON column through idempotent migration. Cover all explicit reads, writes, qualified columns, and legacy table rebuilds.
- Derive and serialize `ci_state`, representative identity, and stale workflow evidence through both event and rebuild paths.
- Add `PRCIState`, summary/TaskPR types, compact mapping, and the pure full-record classifier.
- Keep explicit none separate from legacy omission. Keep skipped, cancelled, neutral, and stale checks outside invented failure/progress states.

## Out of scope

- Rendered indicators, disclosure copy, locale changes, and public documentation.
- Provider reads, query shape, scheduling, check selection, and merge or repair policy changes.

## Acceptance

1. Selected metadata survives storage/reload and same-head lightweight sync, while authoritative zero results and head changes clear obsolete metadata.
2. Event/rebuild projections and Go/TypeScript classifiers agree on priority, skipped neutrality, open-PR filtering, and explicit clearing.
3. Fresh/migrated/replayed database schemas and existing check-action regressions pass. Unknown data never authorizes an action.

## Verification

Run the block from the repository root. Install workspace dependencies once before the first package command in this worktree.
Use the existing environment-gated PostgreSQL test setup. Record skipped PostgreSQL checks explicitly if no test DSN is configured.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/github -run 'CheckSummary|ComputeOverallCheckStatus|CountCheckResults|ChecksPopulated|SelectCurrentPRChecks|ActiveWorkflowWithoutJobs|PostgresStoreSchemaReplay' -count=1)
(cd apps/backend && go test ./internal/task/statussummary -count=1)
(cd apps/backend && go test ./internal/backendapp -run 'StatusSummary|CIState' -count=1)
(cd apps/web && pnpm exec vitest run lib/task-pr-info.test.ts components/github/pr-ci-state.test.ts lib/github/check-buckets.test.ts)
(cd apps/web && pnpm run typecheck)
git diff --check
```

Run `bash scripts/lint-go-changed` with the exact changed Go file paths before completing this work order.
Include any additional changed backend test by exact name in the recorded command if its name falls outside the filter.
No browser test is needed for this non-rendering contract expansion. Task 03 proves integrated user behavior.

## Files likely touched

- `apps/backend/internal/github/models.go`, `client_helpers.go`, `service_pr_watch.go`, `service_pr_feedback_sync.go`, and detailed snapshot adapters
- `apps/backend/internal/github/store.go` and store helpers involved in task PR migration/read/write/rebuild
- New `apps/backend/internal/github/check_summary.go`, `check_summary_test.go`, and `store_check_summary_test.go`
- `apps/backend/internal/github/service_test.go`, `store_postgres_test.go`, and feedback sync tests when needed
- `apps/backend/internal/task/statussummary/model.go`, `projector_pr.go`, `projector_events.go`, and observation equality if new inputs are required
- New `apps/backend/internal/task/statussummary/projector_ci_state_test.go`
- `apps/backend/internal/backendapp/status_summary_adapter.go` and its tests
- `apps/web/lib/types/github.ts`, `github-checks.ts`, `task-status-summary.ts`
- `apps/web/lib/task-pr-info.ts` and `task-pr-info.test.ts`
- New `apps/web/components/github/pr-ci-state.ts` and `pr-ci-state.test.ts`

## Dependencies

None. Read the existing selected-check and workflow-attention contracts before making changes.

## Risks

`checks_total` excludes some observed results, so it cannot establish all-skipped status.
Legacy table rebuilds can discard a column unless their copy lists include it.
Cached feedback must not gain a new observation order merely because it is persisted again.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/github-pr-ci-bubble.md)
- [CI classification](../../specs/integrations/system-design/github-pr-ci-bubble.md#ci-classification)
- [Selected metadata](../../specs/integrations/system-design/github-pr-ci-bubble.md#selected-check-summary)
- [Compact projection](../../specs/integrations/system-design/github-pr-ci-bubble.md#compact-projection-and-freshness)
- `/tdd` and its backend test reference, plus the scoped backend/web `AGENTS.md` files

## Results

Completed on 2026-11-10.

- Added `CheckSummary` with head SHA, observed count, and completed-skipped count. `newPRStatusWithCheckState` populates it from the same selected checks used for the rollup, and watch snapshots copy it.
- Added the additive nullable `github_task_prs.check_summary` JSON column, idempotent migration, qualified reads, inserts, updates, and legacy table-rebuild copies. Reads tolerate malformed or wrong-head metadata as unknown.
- `SyncTaskPR` replaces the summary on detailed observations, preserves it on lightweight same-head syncs, clears it on a head change, and publishes only on a semantic change.
- Extended `statussummary.PullRequestSummary` with `ci_state`, `ci_pr_number`, `ci_repository`, and `ci_stale`. The ordered Go decision table matches the TypeScript classifier and is derived independently from the draft-muted aggregate state.
- Added `PRCIState`, `CheckSummary`, compact summary fields, `taskPRInfoFromSummary` mapping, and the pure `pr-ci-state.ts` classifier.
- Updated the workflow-attention requirement and design to defer conflict placement and CI priority to this package.

Verification:

- `go test ./internal/github -run 'CheckSummary|ComputeOverallCheckStatus|CountCheckResults|ChecksPopulated|SelectCurrentPRChecks|ActiveWorkflowWithoutJobs|PostgresStoreSchemaReplay' -count=1`: passed. PostgreSQL replay skipped without `KANDEV_TEST_POSTGRES_DSN`.
- `go test ./internal/github -count=1`: passed.
- `go test ./internal/task/statussummary -count=1`: passed.
- `go test ./internal/backendapp -run 'StatusSummary|CIState' -count=1`: passed.
- `go build ./...`: passed.
- `bash scripts/lint-go-changed` on every changed Go file: 0 issues.
- `pnpm exec vitest run lib/task-pr-info.test.ts components/github/pr-ci-state.test.ts`: 22 tests passed.
- `pnpm run typecheck`: passed.
