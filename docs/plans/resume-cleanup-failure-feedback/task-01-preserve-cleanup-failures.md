---
id: "01-preserve-cleanup-failures"
title: "Preserve and project cleanup failures"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.1
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.5
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.10
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.11
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.12
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.13
system_design:
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
---

# Task 01: Preserve and project cleanup failures

## Summary

Stop Resume at a failed predecessor cleanup and preserve a safe typed cause.
Manual/automatic responses and durable reload must retain the cause instead of
reporting the unchanged registration as a live agent.

## In scope

Both `resumeSession` cleanup boundaries; typed safe cause and normalization;
attempt-aware rollback and error publication; recovery/launch response mapping;
session/error-stamp ownership; projection and reload regressions.

## Out of scope

Runtime resource policy, cluster repair, forced record removal, new credentials,
conversation replacement, browser rendering, and SQL schema changes.

## Acceptance

1. Failed terminal preflight cleanup calls no launch. Failed stale fallback
   cleanup calls no second launch. Successful cleanup and live-concurrent
   controls preserve behavior; error chains and cancellation remain intact.
2. Both failures produce safe `runtime_cleanup_failed` evidence through current
   request and durable error projections. Preflight has no fabricated STARTING
   transition; fallback rollback is owned. Stale attempts cannot replace newer
   state or error. Raw transport responses never reach browser or persisted detail.
3. Tokens, runtime inventory and workspace credentials remain intact. A later
   successful same-session retry retires only its matching active failure and
   keeps history; independent restore failure retains its own cause.

## Verification

First run the new focused regressions and record their expected failures.
Then run the following from the repository root:

```bash
(cd apps/backend && go test -trimpath ./internal/orchestrator/executor -run 'TestResumeSession|TestSessionCleanupFailure|Test.*LaunchFailure' -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle -run 'Test.*BootstrapFailure|Test.*CleanupStale|TestStopAgentWithReasonRetainsExecutionWhenBackendCleanupFails' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/models -run 'Test.*AgentError|Test.*LaunchError' -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator ./internal/orchestrator/handlers -run 'TestSessionCleanupFailure|Test.*RecoveryFailureIdentity|Test.*BootstrapFailure' -count=1)
git diff --check
```

Add assertions to actual Resume, service projection and WS response paths; a
constructor-only test does not establish this acceptance. Ensure every new
regression name is selected by these commands. Add SQLite-backed service reload
coverage using the existing fixture; no database dialect query changes are intended.

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/resume_cleanup_failure.go` (new)
- `apps/backend/internal/orchestrator/executor/executor_resume_cleanup_failure_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/bootstrap_failure.go` and its tests
- `apps/backend/internal/task/models/launch_errors.go` and its tests
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/session_cleanup_failure.go` (new)
- `apps/backend/internal/orchestrator/session_cleanup_failure_test.go` (new)
- `apps/backend/internal/orchestrator/session_launch_test.go` (continuation cleanup fixture)
- `apps/backend/internal/orchestrator/handlers/handlers.go`
- `apps/backend/internal/orchestrator/handlers/session_cleanup_failure.go` (new)
- `apps/backend/internal/orchestrator/handlers/session_cleanup_failure_test.go` (new)

## Dependencies

None. Read the plan's incident evidence and the design's cleanup failure section.
Reuse `executor_resume_test.go` live-resume fixtures and
`executor_resume_terminal_rollback_test.go` state-ownership coverage. Do not
append new tests to an already oversized test file.

## Risks

Early cleanup precedes startup-attempt persistence. Guard durable error writes
against the current host attempt, observed state and error stamp; never call the
STARTING rollback without its ownership. Preserve genuine live-agent protection.
Do not call the carrier's raw `Error()` to build user details.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/agent-resume-runtime-recovery.md), requirements 001 and 002.
- [Design](../../specs/agents/system-design/agent-resume-runtime-recovery.md#runtime-cleanup-failure-feedback).
- Existing task launch error projection and session recovery presentation contracts linked there.

## Results

Implemented and verified on October 10.

- Both cleanup boundaries stop before replacement launch on failure. Existing
  successful-cleanup controls retain the native conversation token.
- `ResumeCleanupFailure` carries fixed safe cause evidence and the observed
  session revision. The orchestrator uses the existing snapshot CAS and the
  current host resume attempt, preserving lifecycle state and newer errors.
- Both actual WebSocket handlers return the safe kind, causes and owned error
  identity. The raw wrapped cause remains available to backend error handling.
- Executor, SQLite projection and actual handler regressions passed. Projection
  cases also passed under `go test -race`, including stale state/error and
  nonterminal fallback cases. Existing retained-cleanup lifecycle checks passed.
- Changed Go packages: pinned golangci-lint v2.9.0, zero issues. The first cold
  five-minute package load timed out; the longer run completed successfully.
- CI exposed a continuation test fake that returned a deletion error when no
  runtime existed. Cleanup now treats that absence as success, matching the
  lifecycle manager's idempotent contract. The existing generation-commit
  admission test reproduced the failure and passed after this fixture correction
  with `go test -race ./internal/orchestrator -run '^TestRecoverSession_ContextContinuationCommitsBeforePromptAdmission$' -count=1`.
- The complete orchestrator package then passed
  `go test -race -json ./internal/orchestrator -count=1` in an isolated process
  after the browser suites, with no failed tests.
- After integrating main `135630f9b`, the combined cleanup, Resume, continuation
  admission and native restart recovery selection passed under race in the
  orchestrator, executor and handlers packages. The retired instruction/batch
  handler regression also passed independently.
