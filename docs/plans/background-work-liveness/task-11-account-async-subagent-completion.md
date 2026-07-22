---
id: "11-account-async-subagent-completion"
title: "Account for async-subagent completion"
status: completed
wave: 11
depends_on: ["10-preserve-prompt-cycle-identity"]
plan: "plan.md"
spec: "../../specs/fine-grained-background-running-status-indicator/spec.md"
---

# Task 11: Account for async-subagent completion

- **Acceptance:** Identified completion retires the exact async registration;
  duplicates are harmless; uncorrelated completion uses a documented safe
  fallback; execution stop/failure/cancellation/teardown removes every remaining
  registration and publishes the resulting session/task activity transition.
- **Verification:** Focused ACP/lifecycle/orchestrator tests under `-race`, then
  `cd apps/backend && go test -race ./internal/agentctl/... ./internal/agent/runtime/lifecycle/... ./internal/orchestrator/...`.
- **Files likely touched:** ACP detached-work event conversion/types,
  lifecycle forwarding, orchestrator background registration/completion and
  execution teardown paths, plus focused tests.
- **Dependencies:** Task 10 prompt-cycle ownership.
- **Inputs:** Spec accountable-completion requirement; diagnosis of ID-less
  arbitrary retirement and missing execution-teardown cleanup.
- **Output contract:** Report provider identity available, fallback semantics,
  one/many/missing/duplicate/out-of-order tests, teardown publication, exact
  results, risks, and update only this task status.
