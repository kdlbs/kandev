---
id: "01-basic-status"
title: "Complete basic status before enrichment"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.1
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.3
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.4
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.5
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.9
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.13
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.14
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.19
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.24
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.31
system_design:
  - ../../specs/platform/system-design/workspace-git-status.md
---

# Task 01: Complete basic status before enrichment

## Summary

Separate complete membership from secondary Git work. Expand quality fields across the existing Go/TypeScript DTO boundaries before later work consumes them.

## In scope

- Split branch/HEAD identity from comparison probes. Preserve pinned tracked/untracked collection, mixed facets, rename handling, symlinks, and submodules.
- Return complete basic membership without per-file diffs, divergence, merge-base, or branch totals.
- Add optional status/detail/order fields and file/facet `diff_state` to shared types and every copied projection. Keep old decoding valid.
- Add disposable-repository phase tests with an enrichment barrier. Establish immutable snapshot copies and explicit index ownership transfer.
- Inventory pending-value consumers now. Do not claim zero statistics as known when details are pending.

## Out of scope

Recovery transport and rendered UI. No new polling interval, package, or Git library.

## Acceptance

- Before the enrichment barrier releases, the public fresh read exposes every eligible path and both mixed facets with correct identity.
- Existing source policies and skip vocabulary remain decodable. Complete clean membership is explicit, and bare roots cannot claim clean status.
- Commands retain the requested basic admission class and cancel with tracker lifetime. Type expansion passes both backend and frontend compilation.

## Verification

Run from repository root. Use `/tdd` and run each new regression red before implementation.
If `apps/node_modules` is absent, first run `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/backend && go test ./internal/agentctl/server/process ./internal/agentctl/server/api -run 'Test(WorkspaceTracker|GitStatus|ApplyPorcelain|UnquoteGitPath|.*Diff.*|.*Untracked.*|.*Symlink.*)' -count=1)
(cd apps/backend && go test ./internal/agent/runtime/agentctl ./internal/agentctl/types/... -count=1)
(cd apps/web && pnpm run typecheck)
git diff --check
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/workspace_git_status.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_diff.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_index.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_status_progressive_test.go`
- `apps/backend/internal/agentctl/types/streams/git.go`
- `apps/backend/internal/agentctl/server/api/git.go`
- `apps/backend/internal/agent/runtime/agentctl/git.go`
- `apps/backend/internal/agent/runtime/agentctl/client.go`
- `apps/backend/internal/agent/runtime/lifecycle/events.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/web/lib/types/git-events.ts`
- `apps/web/lib/state/slices/session-runtime/types.ts`

## Dependencies

None.

## Risks

- Existing tests can assume that `GetGitStatus` already contains diffs. Update that expectation to eventual enriched snapshots, not a longer timeout.
- Retained indexes must cover staged and mixed-facet diffs. A new test file avoids the Go test-file size ceiling.

## Parallelism

`sequential`. This work order does not authorize delegation.

## Inputs

- [Requirement](../../specs/platform/requirements/workspace-git-status.md).
- [System design](../../specs/platform/system-design/workspace-git-status.md).
- [Plan](plan.md), accepted ADR, scoped `AGENTS.md`, and existing tests beside owned code.

## Results

The regression was first run against the baseline and failed because the status payload lacked quality metadata and already contained enriched diffs. It now proves complete staged-only, unstaged-only, mixed, and untracked membership is returned with `files_complete=true` and pending details before any diff command runs. Phase fields survive the agentctl HTTP result, lifecycle event, backend notification, and frontend WS store adapter.

- `go test ./internal/agentctl/server/process ./internal/agentctl/server/api -run 'Test(WorkspaceTracker|GitStatus|ApplyPorcelain|UnquoteGitPath|.*Diff.*|.*Untracked.*|.*Symlink.*)' -count=1`: passed.
- `go test ./internal/agentctl/server/api -run '^TestHandleGitStatusMulti_SingleRepoUsesEmptyRepositoryName$' -count=1`: passed.
- `go test ./internal/agent/runtime/agentctl ./internal/agentctl/types/... -count=1`: passed.
- `go test ./internal/agent/runtime/lifecycle -run '^TestPublishGitStatus_PropagatesRepositoryName$' -count=1`: passed.
- `go test ./internal/backendapp -run '^TestBuildGitStatusNotificationIncludesAncestryEvidence$' -count=1`: passed.
- `pnpm exec vitest run lib/ws/handlers/git-status.test.ts`: passed (14 tests).
- `pnpm run typecheck`: passed.
- `git diff --check`: passed.
