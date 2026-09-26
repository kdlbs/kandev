---
id: "06-utility-policy-integration"
title: "Utility policy integration"
status: done
wave: 6
depends_on: ["04-dynamic-conductor-policy-integration", "05-kanban-recovery-convergence"]
plan: "plan.md"
spec: "../../specs/platform/requirements/provider-error-recovery.md"
---

# Task 06: Utility policy integration

- **Acceptance:** Apply dynamic class policy to each uniquely identified utility
  invocation, persist concrete attribution, and reject retry, reset waiting, or
  candidate skipping after any partial result or ambiguous effect.
- **Files likely touched:** `apps/backend/internal/utility/{service,handlers}/**`,
  dynamic resolver utility seams, utility call repository fields, and focused
  service/handler tests.
- **Dependencies:** Tasks 04 and 05.
- **Parallelism:** sequential.
- **Inputs:** Provider Error Recovery utility contract and effect-safety;
  Dynamic Agent Routing utility attribution; current unique invocation ID and
  pre-result fallback implementation.
- **Output contract:** Report invocation identity, result/effect evidence,
  durable attribution, wait behavior for one-shot calls, files changed, exact
  commands/results, risks, and synchronized task/plan status.
- **Verification:** `cd apps/backend && go test -tags fts5 ./internal/utility/... ./internal/agent/runtime/... ./internal/task/repository/sqlite/...`
- **Risks:** A non-empty partial response must never be combined with another
  provider's output. Utility waits need cancellation and bounded lifetime.

## Results

Completed. Utility profile resolution creates a unique `utility:<uuid>` route
identity for every invocation, including calls associated with a task session.
The shared evaluator is applied to classified pre-result failures; partial
responses and ambiguous effects fail closed, so utility output is never
combined with a successor provider. Pending retry/reset decisions remain
durable and are returned to the caller without an unsafe synchronous fallback.

Verification:

- `go test -tags fts5 ./internal/utility/... ./internal/agent/runtime/... ./internal/task/repository/sqlite/...` — covered by the 1,673 Office/utility and 1,106 routing/settings/SQLite results.
