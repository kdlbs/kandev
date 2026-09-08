---
spec: docs/specs/ui/requirements/acp-shell-command-output.md
created: 2026-08-10
status: implemented
issue: https://github.com/kdlbs/kandev/issues/2480
---

# Implementation Plan: ACP Shell Command Echo Normalization

## Overview

OMP sends the visible `$ <command>` text and the command result as adjacent ACP content items, while its structured final `rawOutput` does not expose a stdout field recognized by Kandev. Kandev currently treats any non-nil final `rawOutput` as proof that stdout was already normalized, so it skips the final command-echo strip and persists the two content items glued together. The repair makes that state reflect whether stdout was actually recognized and adds an adapter-level regression test for OMP's exact event shape.

## Backend

### Track actual final-stdout normalization

Files:

- `apps/backend/internal/agentctl/server/adapter/transport/acp/shell_output.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/tool_call_update_test.go`

Changes:

- Make `applyFinalShellResult` report whether the final result contained a recognized stdout field and therefore committed the leading-command normalization.
- In `NormalizeShellToolUpdate`, derive `finalStdoutCommitted` from that result instead of setting it for every non-nil `rawOutput`.
- Preserve the existing single-strip guarantee when the final result does contain recognized stdout, including real output that legitimately starts with a second `$ <command>` line.
- Add an adapter-level regression test that seeds OMP's `execute` tool call and completes it with two adjacent text content items (`$ <command>` and real output) plus its structured `rawOutput` container.

No persistence, API, frontend, or provider-specific branch is added. The existing provider-neutral ACP adapter remains the normalization boundary established by ADR-0036.

## Tests

- **What:** OMP's structured final shell update removes the leading command presentation content and preserves the complete adjacent real output.
  **File:** `apps/backend/internal/agentctl/server/adapter/transport/acp/tool_call_update_test.go`.
  **How:** Adapter integration test through `convertToolCallUpdate` and `convertToolCallResultUpdate` using the current OMP mapper shape.
- **What:** Recognized final stdout is normalized exactly once and cumulative/live shell paths keep their existing behavior.
  **File:** existing tests in `apps/backend/internal/agentctl/server/adapter/transport/acp/shell_output_test.go`.
  **How:** Run the full ACP adapter package after the focused red/green regression.

No browser E2E is added because the browser contract and renderer do not change; the defect occurs before persistence at the ACP adapter boundary, and the adapter integration test exercises the complete changed path.

## Verification Results

Diagnostic reproduction completed before implementation:

- The throwaway OMP regression failed as expected because Kandev retained
  `$ <command>` directly before `/usr/bin/gh`.
- The throwaway test was removed after confirming the root cause.
- Focused regressions passed: 2 tests.
- Full ACP adapter package passed: 764 tests.

PR fixup verification:

- Review remediation commit `59f10560c8563a3fd68a5885cb555ecddaa302fc` added
  exit-code assertions for the metadata-only OMP result, clarified the
  recognized-stdout wording, and changed the task verification to use the
  repository backend test target.
- Focused regressions passed: 2 tests.
- `rtk make -C apps/backend test` passed.
- At the exact PR head above, CI reported 36 passed, 0 failed, and two E2E
  shards still in progress: `E2E Shard 2/14` and `E2E Shard 6/14`.
- The review thread was replied to and resolved; no unresolved review threads
  or actionable issue comments remain. GitHub reports the PR as mergeable with
  `mergeStateStatus=UNSTABLE` while those checks run.

## Implementation Waves And Parallel Candidates

- [x] [Task 01: Normalize OMP structured final shell output](task-01-normalize-omp-shell-output.md) (`done`)

The task is sequential. It owns the complete repair and its regression coverage; no subagent delegation is authorized by this plan.

## Risks And Out Of Scope

- The fix must distinguish “final result present” from “final stdout recognized” without stripping an already-normalized value twice.
- This repair does not infer or synthesize a separator between arbitrary content items. It removes only the already-evidenced leading `$ <command>` presentation content.
- Provider-side output missing before it reaches Kandev remains out of scope.
- Installing or changing OMP is out of scope; the captured public mapper shape and failing Kandev adapter reproduction are sufficient to own this repair.
