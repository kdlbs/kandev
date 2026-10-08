---
id: turn-changed-files-03
title: Executor-owned immutable Git capture
status: done
wave: 3
depends_on: []
  - turn-changed-files-02
plan: plan.md
requirements:
  - REQ-TASKS-TURN-CHANGES-002
  - REQ-TASKS-TURN-CHANGES-003
  - REQ-TASKS-TURN-CHANGES-007
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-002.1
  - AC-TASKS-TURN-CHANGES-002.2
  - AC-TASKS-TURN-CHANGES-002.3
  - AC-TASKS-TURN-CHANGES-002.4
  - AC-TASKS-TURN-CHANGES-002.5
  - AC-TASKS-TURN-CHANGES-002.6
  - AC-TASKS-TURN-CHANGES-002.7
  - AC-TASKS-TURN-CHANGES-002.8
  - AC-TASKS-TURN-CHANGES-003.1
  - AC-TASKS-TURN-CHANGES-003.3
  - AC-TASKS-TURN-CHANGES-007.1
  - AC-TASKS-TURN-CHANGES-007.2
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
---

# Executor-owned immutable Git capture

## Summary

Capture fresh Git endpoints and compare exact immutable objects on the executor.

## Scope and owned files

- New modular `turn_checkpoint_*` files under `internal/agentctl/server/process`, integrated through the existing Git operator.
- New `server/api` checkpoint routes using registered checkout resolution, not browser-supplied paths.
- Shared strict NUL raw/status/numstat parsers in `internal/common/turnchanges`.
- Exact safe-command additions in `internal/common/securityutil/git.go`; reuse `common/subproc` admission and process termination.
- Agentctl client wire operations under runtime ownership; real Git fixtures and registered HTTP tests.

## Exclusions

No workflow transitions, write-tool event accounting, backend shell access to remote checkouts, or restoration operations.

## Implementation acceptance

1. Produce fresh private-index start/end trees, unique reachability refs, and exact OIDs without changing real index, staged data, HEAD, branches, or operation files.
2. Real Git fixtures prove dirty-baseline isolation, between-turn isolation, generators, repeated edits/revert, staging/commits, kinds, binary/null counts, and literal paths.
3. Safe sparse handling, conflicts, submodules, nested scopes, deadlines, cross-instance checkout locks, retries, and output limits return accurate completeness or explicit failure.

## Verification

Fixture assertions snapshot real index bytes/entries, HEAD/refs, merge/rebase files, config, working content, and status before/after capture.
Use a separate built-in raw Git oracle, not the production parser, for expected patches/counts.
Include same-size rapid writes, assume-unchanged, manual skip-worktree, split/sparse indexes, unborn HEAD, and two worktrees of one repository.

```bash
cd apps/backend
go test -trimpath ./internal/common/turnchanges ./internal/common/securityutil ./internal/agentctl/server/process ./internal/agentctl/server/api -run 'TurnCheckpoint|TurnChangeNumstat|TurnChangeRaw|TurnChangeGitFlag' -count=1
go test -trimpath -race ./internal/agentctl/server/process ./internal/agentctl/server/api -run 'TurnCheckpoint.*(Lock|Concurrent|Cancel|Retry|Limit)' -count=1
```

Scope filesystem-name and executable-bit assertions only where the host genuinely lacks support.
No broad skip of portable Git behaviors on Windows.

## Dependencies and risks

Depends on immutable DTOs from 02. Mutable status helpers can hard-link read-only indexes; capture must use its own inode.
Capture admission does not stop external writers. Unsafe non-cone/index combinations must fail visibly rather than invent deletions.

## Results

Implemented private-index capture, immutable reachability refs, strict raw/numstat comparison, registered agentctl routes, and runtime client calls. Tests cover dirty/staged baselines, same-size writes, staged commits, repeated edits and reverts, literal paths, file kinds and binary null counts, cone sparse and split indexes, unmerged conflicts, absent gitlinks, missing indexes, SHA-256 repositories, separate worktrees, concurrent operators, stale/cancelled locks, byte/entry bounds, and unchanged user index/config/refs.

Validation passed:

- Focused capture, API, agentctl client, parser, model contract, and Git safe-flag tests.
- Race tests for concurrent capture, file-lock cancellation/recovery, and registered routes.
- `go run ./cmd/sqlguard ./internal`.
- Targeted `golangci-lint` across touched Go packages: 0 issues.

The capture byte preflight bounds candidate working-tree content at 256 MiB per endpoint. Git clean filters remain inside the bounded subprocess deadline. Cross-process behavior is exercised through concurrent independent `GitOperator` instances sharing the checkout lock file; the container/network executor matrix remains for work order 09.

### Review follow-up (2026-10-08)

Compare and export requests use the accepted start/end commit and tree OIDs, validate that exact pair, and use refs only for reachability. Owned checkpoint refs are deleted with an expected-OID check; failed cleanup keeps a durable retry intent and reconnect drains it. Focused processor, API, and coordinator tests passed. This does not add Kubernetes, Sprites, plugin, or transport qualification evidence.
