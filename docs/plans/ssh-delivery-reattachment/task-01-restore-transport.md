---
id: "01-restore-transport"
title: "Restore owned SSH transport before delivery proof"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.4
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.16
system_design:
  - ../../specs/platform/system-design/interrupted-prompt-recovery.md
---

# Task 01: Restore owned SSH transport before delivery proof

## Summary

Connect the live Retry path to verified same-owner SSH transport replacement. Support the saved remote-owner path without consulting a local journal for a remote execution.

## In scope

Executor-owned lost-state replacement, lifecycle routing, current-client/stream fencing, existing retained replay and terminal settlement, integrated regression tests, and independent live fault acceptance.

## Out of scope

Installed app updates/restarts, changes to existing tasks, new UI, new conversations, forced termination, SQL overrides, prompt replay, and other-provider changes.

## Acceptance

1. A surviving SSH controller is reattached before delivery proof, preserving all durable/native identities and a single synthetic tool execution.
2. Duplicate/stale recovery cannot replace a healthy owner or alter its projection; all missing/conflicting proof remains blocked and retryable.
3. New isolated fault acceptance passes without modifying the original task/session set, states, or live backend PID.

## Verification

From the repository root, after adding focused regression tests:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestSSH(Recovery|LiveDeliveryRecovery)' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run 'Test.*(RetrySessionDelivery|SSHDeliveryRecovery)' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/ssh-delivery-reattachment
```

Proposed test prefix `TestSSHLiveDeliveryRecovery` must be created and nonzero test selection verified; a passing empty selection is not acceptance. Run the live procedure in the plan against a separate backend and new task, recording IDs, marker counts, cursor settlement, original-session comparison, and cleanup. Read backend test guidance before permanent test changes.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/delivery_recovery_without_execution.go`
- `apps/backend/internal/agent/runtime/lifecycle/delivery_reconciliation.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_recovery.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_keepalive.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_recovery_test.go`
- New focused lifecycle and orchestrator recovery tests discovered at implementation time.

## Dependencies

None. Read requirements, design, scoped AGENTS.md and backend test guidance first.

## Risks

Tracked lost-state collision, stale client ownership, credential hydration, local/remote identity confusion, and accidental new-start cleanup of a live remote owner.

## Parallelism

sequential

## Inputs

Linked requirements and proposed SSH design section; incident report; existing SSH recovery tests and lifecycle client-replacement ownership code.

## Results

Completed on 2026-10-31. Production changes and six focused regressions were verified in an independent checkout. The failing transport and manager tests reproduced missing reconnect capability and a refused old forward before their fixes. The saved-owner guard test failed before runtime-aware routing was added.

- Exact lifecycle work-order command passed, 1.674 s.
- Exact orchestrator work-order command passed, 1.729 s.
- Focused lifecycle tests with `-race` passed, 3.346 s; orchestrator with `-race` passed, 3.540 s.
- `go vet -trimpath -tags fts5 ./internal/agent/runtime/lifecycle` passed.
- `scripts/lint-go-changed` with golangci-lint v2.9.0 passed with zero issues. The tool and its caches are isolated; the comparison ref exists only in this temporary checkout.
- Specification catalog validated 399 decisions and 1718 specifications. Linter self-tests passed 36 tests. Full specification lint and diff whitespace checks passed.
- Real macOS SSH acceptance passed through the existing WebSocket actions, with a separate backend/home/database/port and a new task. An abrupt disconnect and 15-second refusal preserved the exact controller, native conversation, incarnation and stream. One BEGIN and one END remained after recovery, repeated Retry and a distinct follow-up. The saved submission completed and its unknown-outcome block resolved. The agent recalled a phrase kept only in the conversation.
- Existing task/session identities, states and executor configuration matched the baseline. The original backend process stayed ready. Both test task controllers, the relay and the independent backend exited; test files remain.

A response timing limitation remains: the first Retry returned blocked while terminal projection settled the same recovery; a subsequent state-only Retry returned settled. This did not resend work or prevent a new follow-up. Browser rendering acceptance was not run. The installed binary was not replaced. Task 02 addresses the first-response timing limitation.

Review follow-up reproduced executor-wide lock contention while a workspace callback pinned the old client. The shared remote refresh publisher now obtains the source write lease before entering executor commit; the matching session and client still publish atomically. Losing commits restore the prior URL fallback. Focused SSH, Kubernetes refresh, callback fencing and URL regressions cover the shared helper.
