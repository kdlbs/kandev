---
created: 2026-10-12
status: in_progress
requirements:
  - REQ-TASKS-CHILD-STALL-004
system_design:
  - ../../specs/tasks/system-design/child-turn-stalled-signal.md
legacy_specs: []
---

# Implementation Plan: Child-attention notification queue routing

## Overview

A parent's server-issued `CHILD TASK NEEDS ATTENTION` notices can be left on a
session that later stops being the parent's primary, and a delivered candidate
was treated as terminal so the notice was never reconciled. Stale and duplicate
notices therefore accumulated as dashboard queue rows. This plan keeps a notice
pointed at the parent's current promptable primary and bounds the backlog,
without a new watchdog, collector, or scheduler, and without touching user
messages, questions, answers, or approvals.

Requirement:
[REQ-TASKS-CHILD-STALL-004](../../specs/tasks/requirements/child-turn-stalled-signal.md).
Design:
[Child-turn stalled signal](../../specs/tasks/system-design/child-turn-stalled-signal.md),
section "Current-primary routing and idle-notice reconciliation".

## Root cause

- `markDelivered` persisted the candidate with `resolved=true`
  (`internal/orchestrator/child_stall_delivery.go`), and
  `ListUnresolvedChildStallTurns` excludes resolved turns
  (`internal/task/repository/sqlite/child_stall_turns.go`). A delivered notice
  was therefore never revisited after the parent's primary changed.
- `pruneStaleChildStallAlerts` ran only on the queue-drain path
  (`internal/orchestrator/event_handlers_workflow.go`), so a notice on a session
  that never drained was never pruned.
- Stranded-queue recovery was boot/dispatch-event-driven only
  (`internal/orchestrator/event_handlers_agent.go`), so an idle task parked
  notices indefinitely.
- The dashboard aggregate counts every `queued_messages` row regardless of the
  receiving session state
  (`internal/office/repository/sqlite/aggregate_overview.go`).

## Scope

### In scope

- Reconcile delivered child-attention candidates inside the existing producer
  pass: re-home to the current promptable primary, or withdraw and hold
  (`waiting_parent`) when none exists.
- Withdraw notices when the parent task is archived/terminal or the source turn
  no longer qualifies.
- Collapse equivalent notices for the same child (newer supersedes older) even
  when the session never drains.
- Owning requirement/design update, this plan, and narrow companion unit tests.

### Out of scope

- A second watchdog, collector, scheduler, queue table, or retention framework.
- Changes to durable agent-delivery retention, automation lifecycle, Auto-run,
  merge policy, Workflow, or cancellation.
- Dashboard count masking or any aggregate-rule change.
- Live-DB writes, new hashes/fingerprints/gates/ledgers, broad suites, E2E, or CI.
- Operating-instance recovery/install (Operations) and AIWS canonical prompts.

## Work orders

- [x] [Task 01: Owning contract update](task-01-owning-contract.md)
- [x] [Task 02: Producer notice reconciliation](task-02-producer-reconciliation.md)
- [x] [Task 03: Narrow verification](task-03-verification.md)

## Risks

- Withdrawal must be impossible to apply to a user/question/approval row; gate
  every removal on `messagequeue.IsChildStallAlert`.
- Re-home must preserve FIFO of non-alert entries, so it moves only
  child-attention entries and never reorders the destination queue.
- Reconciliation must be idempotent and bounded by the existing scan window; a
  read failure must leave the candidate retryable.
- A notice older than the scan window is outside this contract's bounded
  retention; do not widen the window or add a second sweep.

## Verification results

Worktree `C:\Users\dream\.kandev\tasks\5mpa4wf3\kandev`, branch
`feature/4bac6e2a-87b3-439e-b-upb` at `origin/main` 51bcc0258a.

| Command | Result |
| --- | --- |
| `go test ./internal/orchestrator/ -run 'TestReconcileDelivered\|TestPruneChildStallAlerts\|TestRunPassReconcilesDeliveredNotice' -count=1` | PASS (6 tests) |
| `go build ./...` | exit 0 |
| `go vet ./internal/orchestrator/...` | exit 0 |
| `golangci-lint run ./internal/orchestrator/... --new-from-rev=51bcc0258a --timeout=5m` | 0 issues |
| `python3 scripts/list-docs.py validate` | 378 decisions, 1567 specifications validated |
| `python3 scripts/lint-spec-files.py --all` | all passed |
| `go test ./internal/orchestrator/ -count=1` | 4 pre-existing failures, none in this change |

The focused tests cover: re-home to the current promptable primary (direct and
through `runPass`), withdraw-and-hold with no promptable primary, withdraw and
resolve when the parent is archived, preservation of a user entry beside a
withdrawn alert, and same-child supersede/coalesce. All assert on the queue
snapshot and the source-turn marker, not log text.

The four full-package failures are unrelated to this change and reproduce on the
base commit: `TestHandleAgentReady_PassthroughAttachmentOnlyOrdinaryMessageUsesAttachmentAwarePrompt`
and `TestHandleAgentReady_PassthroughQueuedAttachmentUsesAttachmentAwarePrompt`
(deterministic Windows PTY attachment-prompt failures), plus
`TestCeilingDispatchAdmissionSerializesRouteMutation` and
`TestCompleteTurnsExceptReportsIterationExhaustion` (flaky; each passed on some
runs of both the base and the change). No child-attention code runs in those
tests.

