---
id: "01-git-history-content"
title: "Git sources and on-demand historical reads"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TURN-CHANGES-002
  - REQ-TASKS-TURN-CHANGES-003
  - REQ-TASKS-TURN-CHANGES-004
  - REQ-TASKS-TURN-CHANGES-007
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-002.1
  - AC-TASKS-TURN-CHANGES-002.8
  - AC-TASKS-TURN-CHANGES-004.1
  - AC-TASKS-TURN-CHANGES-004.4
  - AC-TASKS-TURN-CHANGES-004.11
system_design:
  - ../../specs/tasks/system-design/turn-change-preservation.md
---

# Task 01: Git sources and on-demand historical reads

## Summary

Add typed Git source metadata and source-aware historical reads. Preserve legacy SQL history while new captures render without SQL patch payloads.

## In scope

Internal source locators, common-repository resolution, accepted-object validation, Git-backed patch/context reads, stable file IDs, and legacy reader compatibility.

## Out of scope

Runtime destruction, compaction policy, public controls, and forced legacy-data removal.

## Acceptance

1. A new Git-backed turn renders the exact accepted interval with no SQL patch/rendering payloads.
2. Canonical counts, whitespace filtering, authorization, special paths, and old/new context retain their contracts after worktree removal.
3. Temporary query failure leaves verified snapshot metadata intact; legacy SQL and unsupported-engine history retain their compatible readers.

## Verification

Record a behavioral RED regression before production changes. Use real Git/database fixtures for source and cleanup claims.

```bash
cd apps/backend
go test -trimpath ./internal/task/changes ./internal/task/repository/sqlite ./internal/task/service ./internal/task/handlers -run 'TurnGitHistory|TurnChangeSource|TurnChangeContent' -count=1
go test -trimpath -race ./internal/task/changes ./internal/task/repository/sqlite -run 'TurnGitHistory|TurnChangeSource' -count=1
go run -trimpath ./cmd/sqlguard ./internal
```

Run changed-package Go lint with actual paths. For web commands, install workspace dependencies from `apps/` if this worktree lacks them.
Use the configured PostgreSQL conformance fixtures where applicable, or record missing coverage explicitly.

## Files likely touched

- `apps/backend/internal/task/changes/content.go`
- `apps/backend/internal/task/changes/git_content.go (new)`
- `apps/backend/internal/task/models/turn_changes.go`
- `apps/backend/internal/task/repository/turn_changes.go`
- `apps/backend/internal/task/repository/sqlite/turn_changes_schema.go`
- `apps/backend/internal/task/repository/sqlite/turn_change_content.go`
- `apps/backend/internal/task/service/service_turn_changes.go`
- `apps/backend/internal/agentctl/server/process/turn_checkpoint_diff_view.go`

Add focused adjacent regression files. Extract helpers rather than exceeding scoped size limits.

## Dependencies

None.

## Inputs

- [System design](../../specs/tasks/system-design/turn-change-preservation.md)
- Root and scoped AGENTS guidance, relevant existing tests, and the accepted decision.

## Risks

Do not treat a missing SQL content link as missing Git content. Browser inputs never select arbitrary repository paths or refs.

## Parallelism

`sequential`

## Results

Implemented a typed content-source boundary on repository intervals, an executor object export for the exact accepted endpoint pair, and a backend-owned Git archive store. Local turns read through a verified common-Git locator when the accepted commits and trees survive; archive exports transfer only objects the backend does not already hold.
New captures render patches and old/new blobs on demand from retained Git objects without SQL patch or rendering payloads. Reads select the exact destination section, use the accepted endpoint attributes, and enforce stream bounds before allocating a full response.
Legacy SQL rows keep their existing content-addressed reader, and a temporary archive read failure returns a retryable error without rewriting durable availability. Archive responses are bound to the accepted change set, checkout, hash algorithm, endpoints, and required object closure before acknowledgement.

Behavioral evidence:

- `go test -trimpath ./internal/task/changes ./internal/task/repository/sqlite ./internal/task/handlers -run 'TurnGitHistory|TurnChangeSource|TurnChangeContent' -count=1`
- `go test -trimpath -race ./internal/task/changes ./internal/task/repository/sqlite -run 'TurnGitHistory|TurnChangeSource' -count=1`
- `go run -trimpath ./cmd/sqlguard ./internal`
- Changed-package Go lint: 0 issues.

Review remediation added common-Git reads after linked-worktree removal, missing-object archive transfer, task and installation capacity admission, copy/rename destination selection, accepted .gitattributes behavior, bounded patch/blob reads, closure verification, and archive identity validation.
Remaining gap: PostgreSQL conformance cases were not run because `KANDEV_TEST_POSTGRES_DSN` is unset.
Existing SQL history is not migrated or force-deleted.
