---
id: "11-journal-shutdown"
title: "Reject journal operations after shutdown"
status: done
wave: 7
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.2
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
---

# Task 11: Reject journal operations after shutdown

## Summary

Repair the agentctl crash captured by PR #4368's queue-navigation test.
Replay ran after journal closure and dereferenced a nil bbolt database.
The shared agentctl process exited, which released the capacity-holder session and removed the expected queue state.

## Scope

Guard read and write transactions while holding the existing journal lifetime lock.
Return bbolt's existing closed-database error when shutdown has removed the database.
Preserve journal closure, compaction, retained records, and all browser assertions.
Do not change queue admission, retries, timeouts, or test selection.

## Inputs

- [Delivery requirements](../../specs/platform/requirements/durable-agent-delivery.md).
- [Delivery design](../../specs/platform/system-design/durable-agent-delivery.md), producer and shutdown contract.
- [Failing shard](https://github.com/kdlbs/kandev/actions/runs/37929885371/job/113822881324), attempt 1.
- Its backend log records `Journal.Replay` calling `bbolt.DB.beginTx(0x0)` during source-session shutdown.

## Files

- `apps/backend/internal/agentctl/journal/`: transaction guards and public-operation regression tests.

## Acceptance

All valid late journal reads and writes return a closed-database error without panic.
Normal journal operations retain their existing test coverage and race checks.
The queue-navigation browser test still requires a real capacity holder and visible queue state.

## Verification

```bash
(cd apps/backend && go test -trimpath -race ./internal/agentctl/journal -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/api -run 'Test.*(Journal|Durable|AgentStream)' -count=1)
pnpm --dir apps/web e2e:run --host --project chromium tests/workflow/queue-limit-navigation.spec.ts tests/workflow/queued-session-ownership.spec.ts --retries=0
```

Run Go lint against the current PR base and the existing CI helper/workflow checks after merging main.
Hosted verification remains pending until the remediation commit's checks finish.

## Results

The replay regression reproduced the exact nil database panic before the fix.
All 14 public-operation closure cases and the full journal suite pass with `-race` after the fix.
All three queue-navigation and session-ownership browser cases pass without retries after a fresh build.
Focused agent-stream integration tests pass under the race detector.
Go lint against the merged PR base, 48 workflow/gate tests, Actionlint, harness validation, and documentation checks pass.
Hosted verification awaits the remediation push.
