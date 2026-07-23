---
spec: docs/specs/platform/workspace-git-status.md
created: 2026-07-19
status: done
---

# Implementation Plan: Workspace Git Status Scalability

## Overview

First keep reusable verification caches shared and all verification storage outside Git worktrees, ignore legacy worktree-local cache paths, and reclaim each owned agent temp root during permanent instance teardown. Then make diff enrichment linear and context-aware while preserving complete changed-path metadata and all existing limits. Next coalesce overlapping live observations through the workspace tracker using Kandev's established detached, bounded singleflight pattern. Finish with HTTP-boundary concurrency and multi-repository regression coverage.

The agent-temp teardown portion is a historical implementation record. ADR 0045 was amended on
2026-07-22, and Storage Maintenance Task 10 superseded it by removing per-instance temp injection
and cleanup. Host-local agents now inherit the Kandev service temporary environment.

The incident that motivated this work involved 13,869 untracked Go cache files. Diff enrichment repeatedly recomputed total diff bytes by scanning the complete file map inside per-file loops, producing quadratic work. Six concurrent fresh multi-repository requests then repeated that computation and held agentctl near full CPU for roughly three minutes.

## Backend

### Verification cache safeguard

- Add `/.verify-cache/` and `/.tmp/` to the repository root `.gitignore` so legacy or misconfigured local verification runs cannot add thousands of cache entries to Git status.
- Strengthen `.agents/skills/verify/SKILL.md` to preserve shared managed caches, isolate only invocation scratch, and keep every fallback outside the repository and all worktrees.
- Keep the ignore pattern root-scoped; do not hide similarly named paths that a fixture or nested repository may intentionally track.
- Verify the rule with `git check-ignore` and confirm the guidance still uses an external `mktemp` root.

### Agent temp teardown

- Track the exact session temp directory created by `ensureAgentTempEnv` on `process.Manager`.
- After shell, VS Code, workspace, adapter, and agent process teardown completes for permanent
  instance deletion, remove only that validated session directory. A later resume creates a new
  instance and does not depend on prior scratch. Apply the same cleanup when teardown finds the main
  agent already stopped so natural process exit cannot strand the directory.
- Reject cleanup targets that are empty, equal to the shared `kandev-agent` root, or outside that
  root. Preserve sibling session directories.
- Return cleanup failures to the instance teardown caller and add deterministic tests using an
  isolated test `TMPDIR`.

### Linear and cancellable diff enrichment

- Replace repeated `totalDiffBytes(update)` scans in the unstaged, staged, and untracked loops in `workspace_git_diff.go` with one request-local diff-budget accumulator initialized once and incremented whenever diff content is stored.
- Preserve the existing 10 MiB source-file limit, 256 KiB per-file output limit, 2 MiB per-status limit, overshoot semantics, binary handling, and skip reasons.
- Continue retaining every changed path after the total budget is exhausted. Avoid further file reads or diff synthesis for paths marked `budget_exceeded`.
- Check `ctx.Err()` before each per-file filesystem or diff operation and propagate cancellation so callers cannot publish or cache a partial successful snapshot.
- Add deterministic accumulator and cancellation tests. Use budget state plus structural review to guard the request-local accounting, and benchmark representative input sizes rather than asserting filesystem wall-clock thresholds in unit tests.

### Coalesced live observations

- Add a per-`WorkspaceTracker` `singleflight.Group`; `golang.org/x/sync/singleflight` is already used in the repository.
- Route fresh reads, empty-cache fallback, and poll or explicit refresh computation through the same per-tracker flight so no live path bypasses the overlap guard.
- Keep `updateMu` for the existing polling-skip versus explicit-refresh blocking policy. Only the expensive observation is shared.
- Use `DoChan` and let each caller select on its own context. A cancelled caller exits promptly without cancelling work needed by other callers.
- Run the shared body on a tracker-owned context with an independent deadline no greater than 60 seconds, matching the established detached-singleflight pattern in `internal/agent/handlers/git_handlers.go`. Root it in tracker lifetime so `WorkspaceTracker.Stop` cancels it.
- Check and propagate the shared context error at observation phase boundaries. A cancelled or timed-out computation must not return, publish, or cache partial success.
- Preserve cache ownership: a standalone fresh read does not update `currentStatus`; a poll that joins or initiates the shared observation may cache the completed result through its existing update path.

### API regression coverage

- Exercise concurrent `GET /api/v1/git/status/multi?fresh=true` requests against the same manager and tracker.
- Assert one underlying observation per repository, shared capture data for overlapping callers, independent waiter cancellation, and repository-scoped results.
- Retain partial success when one repository is invalid and other repositories succeed.
- Prefer channel-synchronized Git shims or a focused unexported loader seam over elapsed-time assertions.

## Frontend

No frontend changes. Existing Git-status response shapes and Changes/Review rendering remain unchanged.

## Tests

- **What:** Diff budget accounting is constant-time per changed entry and preserves existing output and skip semantics.
  **File:** `apps/backend/internal/agentctl/server/process/workspace_git_diff_test.go`
  **How:** Test accumulator boundaries, content accounting, and behavior after budget exhaustion. Retain existing oversized, binary, truncated, and budget tests.
- **What:** Cancellation stops untracked enrichment before remaining filesystem operations.
  **File:** `apps/backend/internal/agentctl/server/process/workspace_git_diff_test.go`
  **How:** Cancel a context at a deterministic operation boundary and assert no later files are opened or enriched.
- **What:** Same-repository live observations coalesce without coupling caller cancellation.
  **File:** `apps/backend/internal/agentctl/server/process/workspace_git_status_concurrency_test.go`
  **How:** Coordinate callers with channels; assert one loader invocation and identical completed results, while one cancelled waiter exits independently.
- **What:** Tracker shutdown or the shared deadline stops the underlying observation without updating the cache.
  **File:** `apps/backend/internal/agentctl/server/process/workspace_git_status_concurrency_test.go`
  **How:** Block at a controlled phase, cancel tracker lifetime or expire an injected deadline, and assert cancellation plus unchanged cached state.
- **What:** Existing fresh-versus-cached semantics remain unchanged.
  **File:** `apps/backend/internal/agentctl/server/process/workspace_git_status_test.go`
  **How:** Keep `TestGetGitStatus_FreshBypassesStaleCache` passing and cover empty-cache fallback through the shared observation path.
- **What:** Concurrent fresh HTTP requests preserve coalescing, repository identity, and partial success.
  **File:** `apps/backend/internal/agentctl/server/api/git_status_fresh_concurrency_test.go`
  **How:** Issue overlapping multi-repository requests through the handler and assert invocation counts and response entries rather than timing alone.
- **What:** Large untracked-set enrichment scales linearly.
  **File:** `apps/backend/internal/agentctl/server/process/workspace_git_diff_test.go`
  **How:** Add a benchmark at representative 1,000- and 10,000-entry sizes, deterministic unit assertions for request-local budget accounting, and structurally verify no whole-map scan occurs inside a per-file loop. Do not impose a filesystem wall-clock threshold in unit tests.
- **What:** Verification storage cannot appear as root-level Git-status entries and managed caches remain shared.
  **Files:** `.gitignore`, `.agents/skills/verify/SKILL.md`
  **How:** Assert `git check-ignore -q .verify-cache/go-cache/probe` and `git check-ignore -q .tmp/gocache/probe`, then inspect the verification skill for shared-cache preservation and external scratch requirements.
- **What:** Permanent instance teardown removes only its owned agent temp directory after subprocess teardown.
  **Files:** `apps/backend/internal/agentctl/server/process/manager.go`, `manager_temp_test.go`
  **How:** Use an isolated temporary root; create session and sibling sentinels, stop an already-stopped manager and a normally stopping manager, and assert only the owned session directory is removed. Cover invalid/root targets and cleanup errors.

## E2E Tests

No browser E2E changes. The API payload and user-visible rendering are unchanged; focused process and handler tests cover the affected behavior.

## Implementation Waves

Wave 0:

- [x] [task-00-verification-cache-safeguard](task-00-verification-cache-safeguard.md) - done

Wave 1 (parallel):

- [x] [task-01-linear-diff-enrichment](task-01-linear-diff-enrichment.md) - done
- [x] [task-04-agent-temp-teardown](task-04-agent-temp-teardown.md) - superseded by Storage
  Maintenance Task 10

Wave 2 (depends on Wave 1):

- [x] [task-02-coalesce-fresh-status](task-02-coalesce-fresh-status.md) - done

Wave 3 (depends on Wave 2):

- [x] [task-03-api-regression-verification](task-03-api-regression-verification.md) - done

## Verification

```bash
rtk git check-ignore -q .verify-cache/go-cache/probe
rtk git check-ignore -q .tmp/gocache/probe
rtk go test ./internal/agentctl/server/process -run 'TestManager_.*Temp'
rtk go test ./internal/agentctl/server/process -run 'Test(DiffBudget|EnrichUntrackedFileDiffs|GetGitStatus)'
rtk go test ./internal/agentctl/server/api -run 'Test.*GitStatus.*Fresh'
rtk go test -race ./internal/agentctl/server/process ./internal/agentctl/server/api
rtk make -C apps/backend fmt
rtk make -C apps/backend test
rtk make -C apps/backend lint
```

Run the `go test` commands from `apps/backend`. Run formatting before the full test and lint commands.

## Risks

- The shared body cannot inherit the first caller's context; otherwise that caller can cancel results still needed by peers.
- Detached work needs both tracker-lifetime cancellation and a bounded deadline so a timed-out HTTP caller cannot leave unbounded work behind.
- Pollers and fresh readers share computation but retain different cache-write behavior. Coalescing must not make fresh reads cache owners.
- Map iteration order determines which files receive the remaining diff budget today. The change must preserve semantics without claiming stable ordering.

## Open Questions

None.
