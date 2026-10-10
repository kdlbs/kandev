---
id: "02-ordered-enrichment"
title: "Fence and bound tracker publication"
status: done
wave: 2
depends_on: ["01-basic-status"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.2
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.3
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.4
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.5
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.6
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.7
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.8
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.20
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.21
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.22
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.31
system_design:
  - ../../specs/platform/system-design/workspace-git-status.md
---

# Task 02: Fence and bound tracker publication

## Summary

Make the tracker the sole publisher and run one bounded enrichment worker. Late work repairs the cache while obsolete work cannot replace newer state.

## In scope

- Allocate observation ordinals and source epoch/revisions. Publish accepted basic and enriched snapshots through one ordered path.
- Remove independent update/attach cache writes. Replay accepted quality without fabricated clean timestamps.
- Implement one running enrichment job plus one replaceable pending slot, fingerprint deduplication, deep copies, and cancel/drain cleanup.
- Pin index and literal HEAD for enrichment. Validate worktree content, index replacement, comparison generation/ref, and repository identity.
- Keep existing byte caps, binary/oversized handling, budget overshoot, background admission, and command deadlines.
- Limit changed-evidence correction to one basic refresh. Add channel-gated concurrency, timeout, replay, and shutdown regressions.

## Out of scope

Foreground response handling and UI presentation. No unlimited worker pool or per-file goroutines.

## Acceptance

- A cancelled/deadline-expired waiter does not prevent a later accepted cache/stream result. An unchanged dirty workspace replays that result without mutation.
- Older basic/enriched completions and changed HEAD/index/content/comparison state cannot overwrite a newer accepted snapshot or attach unrelated diffs.
- Repeated unchanged requests start no duplicate enrichment. Jobs remain bounded, interactive basics avoid background capture, and Stop drains subprocesses and index files.

## Verification

Run from repository root. Use `/tdd` and run each new regression red before implementation.
If `apps/node_modules` is absent, first run `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/backend && go test -race ./internal/agentctl/server/process ./internal/agentctl/server/api -count=1)
(cd apps/backend && go test -race ./internal/common/subproc -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/workspace_tracker.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_status.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_diff.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_index.go`
- `apps/backend/internal/agentctl/server/process/workspace_stream.go`
- `apps/backend/internal/agentctl/server/process/workspace_monitor.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_poll.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_status_concurrency_test.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_status_publication_test.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_status_progressive_test.go`
- `apps/backend/internal/agentctl/server/process/workspace_stream_test.go`
- `apps/backend/internal/agentctl/server/api/git_status_fresh_concurrency_test.go`

## Dependencies

Complete Task 01 first. Read its Results before changing the shared contract.

## Risks

- Do not copy the current monitor fingerprint without adding missing identity/content evidence.
- Publication and notification lock ordering must not deadlock Stop or subscriber detach.
- The old attach-no-broadcast test intentionally changes to ordered broadcasts without duplicate job work.

## Parallelism

`sequential`. This work order does not authorize delegation.

## Inputs

- [Requirement](../../specs/platform/requirements/workspace-git-status.md).
- [System design](../../specs/platform/system-design/workspace-git-status.md).
- [Plan](plan.md), accepted ADR, scoped `AGENTS.md`, and existing tests beside owned code.

## Results

The baseline full API suite exposed two outdated assertions after progressive publication landed: a cache miss correctly returns complete membership with pending detail, and the test gate counted the enrichment identity fence as a second primary observation. The API assertions now pin the membership/detail split and the concurrency gate now holds the porcelain membership query. The first full race run also exposed a test setup race because the stream test installed its enrichment gate after starting the worker; moving the gate before the first observation made the ownership boundary deterministic.

- `(cd apps/backend && go test -race ./internal/agentctl/server/process ./internal/agentctl/server/api -count=1)`: passed; process 130.276s, API 61.569s.
- `(cd apps/backend && go test -race ./internal/common/subproc -count=1)`: passed; 8.109s.
- `golangci-lint run ./internal/agentctl/server/process ./internal/agentctl/server/api`: passed; 0 issues.
- Focused progressive publication, evidence fencing, shutdown, and stream regressions: passed.
- `git diff --check`: passed.

The full desktop/mobile browser proof remains in Task 05. Task 02 changes only backend behavior and tests.
