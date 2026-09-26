---
id: "04-dynamic-conductor-policy-integration"
title: "Dynamic conductor policy integration"
status: done
wave: 4
depends_on: ["03-durable-policy-evaluator"]
plan: "plan.md"
spec: "../../specs/platform/requirements/provider-error-recovery.md"
---

# Task 04: Dynamic conductor policy integration

- **Acceptance:** Route launch and settled-turn dynamic failures, circuit
  state, continuation delivery, successor selection, restart recovery, and
  manual route actions through the shared evaluator without weakening
  effect-safety or generation fencing.
- **Files likely touched:**
  `apps/backend/internal/agent/runtime/{dynamic_resolver.go,dynamic/**}`,
  `apps/backend/internal/orchestrator/{dynamic_launch,event_handlers_transient,event_handlers_agent}.go`,
  task-session route action handlers, and focused integration tests.
- **Dependencies:** Task 03.
- **Parallelism:** parallel-safe with Task 06 after Task 03. This task owns
  backend conductor and orchestrator files.
- **Inputs:** Dynamic Agent Routing Route selection, One logical chat, Route
  events, and failure boundaries; Provider Error Recovery effect-safety and
  route ownership.
- **Output contract:** Report launch and mid-turn boundaries, continuation
  evidence, route-action contract, circuit interaction, stale-event tests,
  files changed, exact commands/results, risks, and synchronized task/plan
  status.
- **Verification:** `cd apps/backend && go test -tags fts5 ./internal/agent/runtime/... ./internal/orchestrator/... ./internal/backendapp/...`
- **Risks:** A retry or skip after ambiguous delivery can duplicate tools.
  Policy must never bypass continuation persistence or pre-result evidence.

## Results

Completed. Dynamic launch and settled-turn failures now use the shared policy
evaluator, retain pre-result effect-safety gates, and persist continuation
packages before successor launches. Launch fallback continues through all
eligible candidates within the configured candidate bound. Manual route
actions use one backend operation for selection, predecessor shutdown,
successor launch, and durable result; stale generations are rejected. Durable
recovery resumes only due pending states.

Verification:

- `go test -tags fts5 ./internal/orchestrator/... ./internal/backendapp/...` — 3,085 passed in 11 packages.
- `go test -tags fts5 ./internal/agent/runtime/dynamic/...` — included in the 1,106-package run for Task 03.
