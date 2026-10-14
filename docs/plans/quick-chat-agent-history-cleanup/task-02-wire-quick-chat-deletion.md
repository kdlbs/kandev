---
id: "02-wire-quick-chat-deletion"
title: "Wire cleanup into Quick Chat deletion and expiry"
status: done
wave: 2
depends_on:
  - "01-agent-history-removers"
plan: "plan.md"
requirements:
  - REQ-TASKS-QUICK-CHAT-AGENT-HISTORY-001
  - REQ-TASKS-QUICK-CHAT-AGENT-HISTORY-002
acceptance_criteria:
  - AC-TASKS-QUICK-CHAT-AGENT-HISTORY-001.1
  - AC-TASKS-QUICK-CHAT-AGENT-HISTORY-001.2
  - AC-TASKS-QUICK-CHAT-AGENT-HISTORY-001.4
  - AC-TASKS-QUICK-CHAT-AGENT-HISTORY-001.5
  - AC-TASKS-QUICK-CHAT-AGENT-HISTORY-001.6
  - AC-TASKS-QUICK-CHAT-AGENT-HISTORY-002.1
  - AC-TASKS-QUICK-CHAT-AGENT-HISTORY-002.4
system_design:
  - ../../specs/tasks/system-design/quick-chat-agent-history-cleanup.md
---

# Task 02: Wire cleanup into Quick Chat deletion and expiry

## Summary

In `performTaskCleanup`, capture each eligible Quick Chat session's history identity before its
executor row is deleted, then call `agenthistory.Remove` after `cleanupQuickChatDirs`. Failures
are logged and never change the deletion result.

## In scope

- Eligibility check (Quick Chat task, not managed conversation, not preserved runtime,
  host-local executor, supported agent, safe token).
- Identity capture: agent name from the session's agent profile snapshot, resume token and
  worktree path from `executors_running`, scratch path from `quickChatDir`, config-dir env from
  the resolved agent profile environment. Confirm the launched cwd for multi-repository chats.
- Injectable remover and home dir on `Service` for tests (follow `SetQuickChatDir`).
- Retain native identities in session metadata under the existing executor/token transaction
  guards, then capture all eligible retained identities before task deletion.
- Preserve live history after failed stops, independently from resume-safe dead-row preservation.

## Out of scope

- Archive (does not delete Quick Chats' history).
- Workspace deletion beyond what already routes through `performTaskCleanup`.

## Acceptance

- Service test: create Quick Chat task + session + executor row with a resume token and a fake
  Claude transcript; `DeleteTask` removes only that transcript; a normal task's transcript in the
  same fake root remains after deleting that normal task.
- Service test: `runQuickChatExpiration` with an expired Quick Chat removes its transcript.
- Service test: deleting a Quick Chat with no transcript, no resume token, a non-local executor,
  or a remover error succeeds and leaves the existing deletion outcome unchanged.

## Verification

From `apps/backend`:

```bash
go test -trimpath ./internal/task/service/ -run 'QuickChat' -count=1
go test -trimpath ./internal/task/service/ -count=1
```

From the repository root:

```bash
bash scripts/lint-go-changed $(git diff --name-only main -- '*.go')
```

## Files likely touched

- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/quick_chat_agent_history.go` (new)
- `apps/backend/internal/task/service/quick_chat_agent_history_test.go` (new)
- `apps/backend/internal/backendapp/main.go` (only if the home dir or env needs wiring)

## Dependencies

WO-01.

## Risks

Both runtime cleanup entry points are shared by every task type. The shared Quick Chat
eligibility check must run before capture so normal tasks never reach the remover.

## Inputs

- [Requirements](../../specs/tasks/requirements/quick-chat-agent-history-cleanup.md)
- [System design](../../specs/tasks/system-design/quick-chat-agent-history-cleanup.md)
- `apps/backend/AGENTS.md`

## Results

- Transactional history retention and failed-stop-only history preservation are implemented.
  Tests cover accepted token clear/rotation, stale executions, unrelated metadata,
  executor-row deletion, foreign identities, normal/remote/managed exclusions, archive
  preservation, and concurrent PostgreSQL writers. Broad execution stays hosted.
- Actual full-backend events reproduced confirmed-dead and clear/rotation transcript
  failures. Archive and failed-stop controls passed. The initial transactional fix passed
  hosted source-bound proof 38613208219. Combined-source proof
  [38620076258](https://github.com/kdlbs/kandev/actions/runs/38620076258) verifies
  16 required named regressions with actual RUN/PASS events, including expiry cleanup
  and the idle-clock assertion. Its production source is exactly e23279a748; only the
  temporary focused workflow differs. This proof does not replace full PR acceptance.
- Full-backend artifact 11919818440 exposed history bookkeeping renewing an old session's
  activity timestamp and suppressing expiry. History-only metadata SQL now preserves
  that clock, with SQLite rotation and PostgreSQL concurrent-CAS assertions. The original
  expiry test and cleanup timeout are unchanged. Full PostgreSQL 16/18 and repository
  acceptance are reported by the current PR checks.
- The permission-denied cleanup test skips when the hosted container runs as root;
  its skip is not claimed as a passing execution.

- Identities are captured in `deleteTaskWithManagedClaim` (non-managed deletes only) and carried
  through `taskEnvironmentCleanup.agentHistory` and the cleanup job snapshot `agent_history`.
- Host-local check uses `executors_running.runtime == standalone`; config dirs come from the
  backend process environment.
- Contributor-recorded verification before PR remediation: `go test -trimpath ./internal/task/service/... -count=1`: pass (new service tests fail when the
  removal call is disabled). `go vet`, `gofmt -l`: clean. `golangci-lint` v2.9.0
  `--new-from-rev` on both packages: 0 issues. Run in a `golang:1.26` container; Go is not
  installed on the host.
- Manual test on a dev instance found that closing a Quick Chat tab deletes through
  `HandoffService.DeleteTaskTree` (`cascade_delete`), which prepares its snapshot in
  `PrepareTaskResourceCleanupWithOptions` and skipped the capture. Fixed by capturing there for
  every deleting trigger; regression test `TestDeleteQuickChatTreeRemovesItsAgentHistory`
  covers Claude, Codex and a normal task through the cascade route.
