---
created: 2026-09-27
status: complete
requirements:
  - REQ-INTEGRATIONS-GITHUB-RATE-001
  - REQ-INTEGRATIONS-GITHUB-RATE-002
  - REQ-INTEGRATIONS-GITHUB-RATE-003
  - REQ-INTEGRATIONS-GITHUB-RATE-004
system_design:
  - ../../specs/integrations/system-design/github-rate-limit-coordination.md
legacy_specs: []
---

# Implementation Plan: PR 3143 repair

## Overview

Repair confirmed defects without replacing principal-wide admission or the
Workflow Sync retry owner. All six work orders are implemented and verified.
The implementation was initially left local and uncommitted as requested.
See Delivery results for the current PR state.

## Review basis

- PR: https://github.com/kdlbs/kandev/pull/3143
- Related issue: https://github.com/kdlbs/kandev/issues/3176
- Previous inspected head: `fa960ef6edff762a0a768c4e4a60b531dc199921`.
- Current contributor head: `ffd4f76be095db4291cdf762082c8365097d5478`.
- Current fetched main: `359b5ffdbb6e25592bc3a46d88db1dbf94ff103c`.
- Merge base: `04b722121ad50f0dcb41c7c16d8bd6ca5c4b93df`.
- Local branch: `feature/fix-workflow-sync-gi-d0x-ea3dzi7c`.
- Backup: `backup/pr-3143-before-rebase-20260927` preserves the earlier checkout.
- The PR was open and non-draft at the start of implementation. The local branch
  has since been rebased to remove unrelated changes; it is not pushed and its
  current head is `95a30d761b88bab0fd6795e97ba6a5146cf0e6f7`.

The initial rebase attempt stopped at commit `4765502cf`, the first of 78
commits. Conflicts affected GitHub clients and specification indexes. Task 01
later completed the history-preserving rebase onto current main. Task 06
rewrote the local topic history to remove two unrelated changes; the branch
still has `origin/main` as an ancestor.

The earlier PR helper report is historical and does not certify this repair.
Focused tests and documentation/specification checks are recorded below and in
each work order. No complete backend build, full backend test/lint gate, or
browser E2E suite ran.

## Scope

### In scope

- Restore progress across paced multi-request automatic syncs.
- Preserve recovery state across actual request cancellation.
- Classify HTTP 200 GraphQL errors from their payload and quota evidence.
- Restore the maintainer-requested internal-only snapshot boundary.
- Reconcile the branch history and isolate unrelated CI/test changes.
- Preserve the contributor's existing sanitization and auth-circuit integration.

### Out of scope

- Distributed quota coordination or direct agent-shell command interception.
- A new rate dashboard, plugin API, database migration, or rendered UI.
- Pushing, posting reviews, resolving threads, or merging the PR.
- Reimplementing the provider error sanitizer that the contributor already fixed.
- Generic QA or repository-wide verification.

## Assumption check

Confirmed: on 2026-09-27 the user authorized implementation of this package and
requested that the result remain uncommitted. The package uses the recorded
maintainer feedback as the snapshot boundary:
https://github.com/kdlbs/kandev/pull/3143#issuecomment-5468064001

The contributor later restored the tool and changed the specifications.
The PR description and accepted ADR still describe operation-local details.
This package explicitly restores the earlier maintainer direction. It does not
treat the contributor's restoration as approval of a new boundary.

The integration system owns this repair because it owns provider admission and
workflow-source recovery. Existing requirements remain the source of truth.
Added criteria clarify GraphQL payloads, cancellation, progress, and tool absence.

## Technical approach

### Provider classification

Use one payload-aware classification path in `internal/github`.
Keep successful zero-remaining responses distinct from rate-error responses.
Preserve a later Retry-After boundary, primary reset evidence, and fallback rules.
Do not expose partial GraphQL data through error details.

### Durable cancellation

Remove the eager recovery reset from manual attempts.
Use the manual mode to bypass the automatic schedule.
Record terminal outcomes with a bounded context that retains identity values.
Never use that context for provider calls or workflow application.
Retain a later stored boundary when cancellation supplies weaker information.
Use `internal/office/configsync/reconcile_run.go:recordWriteContext` as the local pattern.

### Automatic continuation

Keep nonblocking admission and the bounded scheduler.
Attach fetch progress to the queued operation instead of restarting the operation.
Bind progress to configuration, credential fingerprint, and a local sync generation.
Recheck these identities under the workspace lock before each request and apply.
Discard stale progress after manual completion, configuration change, or shutdown.
Do not solve this by sleeping inside every worker or by bypassing admission.

### Agent contract

Remove the snapshot registration, action, handlers, prompts, and orphaned public DTOs.
Keep coordinator observations internal and retain operation-local error details.
Public documentation changes belong to the removal work order, not this design turn.

## Findings and evidence

| Finding | Current evidence | Disposition |
| --- | --- | --- |
| P1: automatic sync can repeat the directory forever | `rate_coordinator.go:240` defers pacing. `runAutomaticJob` restarts `fetchFiles` | Task 04 |
| P2: real cancellation cannot persist recovery | `service.go:344` uses the canceled context after `prepareManualSync` clears state | Task 03 |
| P2: GraphQL HTTP 200 loses classification or retry evidence | `rate_error.go:139` accepts primary only at 403/429 | Task 02 |
| Architecture mismatch: snapshot tool restored | Current MCP registration versus maintainer removal request | Task 05 |
| Scope drift | Artifact action and SSH test changes do not implement rate coordination | Task 06 |
| Earlier raw-error disclosure | `RateTracker.ObserveSecondary` now stores a constant reason | Fixed, retain regression coverage |

Static reproduction sequences:

1. Directory response completes quickly. Release sets a future pacing time.
   The file request defers. The scheduler requeues the workspace without progress.
   The next run repeats the directory and sets another pacing time.
2. Seed a retry deadline. Start manual sync, then cancel its actual context.
   The eager reset succeeds, but `RecordSyncFailure` uses canceled `ExecContext`.
3. Return HTTP 200 with `errors[].type=RATE_LIMITED`, remaining zero, and a future reset.
   The classifier selects secondary and its fallback instead of primary and reset.
   A message-only rate error without that type becomes unknown.

These are source traces, not executed reproductions. The work orders name the
required red tests. Existing comments cover related concerns, so no duplicate
author-facing comment was posted. This focused recheck is not an exhaustive
full-file security audit of the 77-file PR.

## Tests

| Acceptance criteria | Required evidence |
| --- | --- |
| RATE-001.1, .2, .4, .6 | PAT HTTP and CLI GraphQL payload tables, retry metadata, subsequent admission |
| RATE-003.1, .2, .4, .6 | Actual canceled request, fresh database read, service restart, successful recovery |
| RATE-002.2, .3; RATE-003.5, .7 | Directory plus two files through real admission, bounded workers, stale continuation disposal |
| RATE-004.1 through .4 | Tool absence on both profiles, safe failed operation, success without quota fields |

The table uses the `AC-INTEGRATIONS-GITHUB-` prefix.
Each work order contains full identifiers and exact commands.

## End-to-end evidence

Task 04 exercises automatic scheduling and workflow reconciliation with a local
HTTP provider, plus typed admission deferral and stale-continuation handling.
Separate GitHub tests exercise real nonblocking admission for batched PR and
branch GraphQL requests. Task 03 includes the real HTTP handler and durable
store boundary. Existing browser behavior remains unchanged; no rendered UI
change applies.

## Work orders

- [x] [Task 01: Reconcile the branch with main](task-01-rebase.md)
- [x] [Task 02: Classify GraphQL rate payloads](task-02-graphql.md)
- [x] [Task 03: Preserve canceled recovery](task-03-cancellation.md)
- [x] [Task 04: Resume deferred synchronization](task-04-continuations.md)
- [x] [Task 05: Restore the operation-local contract](task-05-agent-contract.md)
- [x] [Task 06: Remove unrelated PR changes](task-06-scope.md)

Execute sequentially. No work order authorizes delegation.

## Companion package

The [original package](../github-rate-limit-coordination/plan.md) remains the
historical execution record. Its linked repair notices distinguish old receipts
from completed corrections. Keep its original passing counts as historical
evidence; use this package for the repair results.

## Implementation verification before delivery

All six work orders are complete. Focused tests passed:

- `go test ./internal/mcp/server ./internal/mcp/handlers ./internal/github ./internal/workflowsync -count=1`
- `go test -race ./internal/workflowsync ./internal/github -run 'Test.*(Automatic|Deferred|Continuation|Pacing|Admission|Coordinator|Batched)' -count=1`
- Additional GraphQL classification and actual-cancellation regressions are listed in Tasks 02 and 03.

Documentation, specification, and scope checks passed:

- `node --test scripts/validate-public-docs.test.mjs` (62 tests)
- `node scripts/validate-public-docs.mjs` (47 pages)
- `python3 scripts/list-docs.py validate` (316 decisions and 1203 specifications)
- `python3 scripts/lint-spec-files.py --all`
- `git diff --exit-code origin/main -- .github/actions/download-artifact-retry/action.yml .github/scripts/e2e-tests-workflow-contract_test.py apps/backend/internal/agent/runtime/lifecycle/executor_ssh_keepalive_test.go`
- `git diff --check`

No complete backend build, full backend test/lint gate, or browser E2E suite
ran. At implementation handoff, the branch was locally rebased and the repair
worktree was uncommitted. The Delivery results section records later actions.

## Risks

- The coordinator remains process-local. Other processes and direct shell
  commands do not share its observations or admission decisions.
- Automatic progress is memory-only and is discarded on shutdown. Workflow
  Sync retry and suspension state remains durable.
- This was a focused repair and verification, not a full-file review or
  exhaustive security audit of every PR path.
- The local branch history and worktree differ from the remote PR until an
  explicitly authorized delivery action updates it.
