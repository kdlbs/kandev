---
id: "01-local-source-fallback"
title: "Task-bound local Git source fallback"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-001
acceptance_criteria:
  - AC-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-001.1
  - AC-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-001.2
  - AC-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-001.3
  - AC-AGENTS-NATIVE-CODE-REVIEW-LOCAL-SOURCE-001.4
system_design:
  - ../../specs/agents/system-design/native-code-review-recovery.md
---

# Task 01: Task-bound local Git source fallback

## Summary

When the agentctl read a review normally uses fails for a local or worktree
executor, read the task's own persisted checkout directly so the review can
still collect the working tree, staged, untracked, and committed changes. Keep
the local read within the session's existing `session.exec` authorization, use
the exact persisted comparison target, and keep untracked patch hashes equal
across both readers.

## In scope

- `internal/review/local_git.go` and `internal/review/local_source.go`: the
  bounded local Git reader (`Source`, `ValidateSource`, `CollectLocalChanges`).
- `internal/backendapp/review_local_source.go`: the task-bound session reader
  that admits only `local`, `worktree`, and `local_pc` executors.
- `internal/agent/handlers/review_change_source.go`: the fallback branch in
  `ReviewChangeSource.UncommittedFiles` / `CommittedFiles`, including failed
  comparison responses.
- `internal/common/unidiff/untracked.go`: the shared bounded formatter used by
  agentctl and the local review collector.
- The index blob size guard for staged-only local changes.

## Out of scope

The caller-supplied filesystem path API (an explicit operator/admin path), any
grant framework, any schema change, remote executors, and the runtime
review/utility integration.

## Verification

- `go test ./internal/agent/handlers -run 'TestReviewChangeSource' -count=1`
- `go test ./internal/backendapp -run 'TestReviewLocalFallbackAnchorsOnLiveTarget|TestReviewAgentctlUnavailableUsesLocalGit|TestReviewLocalFallbackUsesQualifiedComparisonTarget' -count=1`
- `go test ./internal/review -count=1`
- `go test ./internal/common/unidiff -count=1`
- `go test ./internal/agentctl/server/process -run '^(TestBuildUntrackedDiff_CancellationDuringLineConstruction|TestBuildUntrackedDiffUsesCanonicalPatch)$' -count=1`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`

## Results

Passed:

- `go test ./internal/agent/handlers -run '^TestReviewChangeSource' -count=1`
- `go test ./internal/backendapp -run '^(TestReviewLocalFallbackAnchorsOnLiveTarget|TestReviewAgentctlUnavailableUsesLocalGit|TestReviewLocalFallbackUsesQualifiedComparisonTarget)$' -count=1`
- `go test ./internal/review -count=1`
- `go test ./internal/common/unidiff -count=1`
- `go test ./internal/agentctl/server/process -run '^(TestBuildUntrackedDiff_CancellationDuringLineConstruction|TestBuildUntrackedDiffUsesCanonicalPatch)$' -count=1`
- `bash scripts/lint-go-changed apps/backend/internal/agent/handlers/git_handlers_test.go apps/backend/internal/agent/handlers/review_change_source.go apps/backend/internal/agent/handlers/review_change_source_test.go apps/backend/internal/agentctl/server/process/workspace_git_diff.go apps/backend/internal/agentctl/server/process/workspace_git_diff_test.go apps/backend/internal/backendapp/review_local_source.go apps/backend/internal/backendapp/review_routing_test.go apps/backend/internal/common/unidiff/untracked.go apps/backend/internal/common/unidiff/untracked_test.go apps/backend/internal/review/diff.go apps/backend/internal/review/diff_test.go apps/backend/internal/review/local_git.go apps/backend/internal/review/local_git_test.go apps/backend/internal/review/local_source.go` (0 issues)
- `python3 scripts/list-docs.py validate` (415 decisions and 1,835 specifications validated)
- `python3 scripts/lint-spec-files.py --all` (passed)
- `gofmt -l` on all changed Go files (no output)
- `git diff --check` (passed)

The exact current `main` base (`ad5cd35e7ec0c777867c2b905a3e05ce97ee5c09`) merges with the PR head without conflicts. GitHub checks were queued on the pre-fixup head; their state is refreshed after the push.
