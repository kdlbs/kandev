---
id: "11-core-routing-observability"
title: "Core routing observability"
status: pending
wave: 9
depends_on: ["04-core-route-engine", "06-logical-session-integration", "10-office-routing-handoff"]
plan: "plan.md"
spec: "../../specs/agents/requirements/dynamic-agent-routing.md"
---

# Task 11: Core routing observability

- **Acceptance:** Publish structured logs and expvars for decisions, retries,
  fallback, waiting, circuits, probes, continuation, restart, stale events, and
  utility outcomes. Preserve `routing_route_attempts_total` and
  `routing_fallback_total`. Replace provider degraded/recovered and parked-run
  metrics with resource-circuit opened/closed and waiting-session metrics.
- **Files likely touched:** `apps/backend/internal/agent/runtime/dynamic/*metrics*.go`,
  `apps/backend/internal/office/scheduler/{routing_metrics,metrics_vars}.go`,
  `AGENTS.md`, `apps/backend/AGENTS.md`.
- **Dependencies:** Tasks 04, 06, and 10.
- **Parallelism:** sequential.
- **Inputs:** Spec Observability, plan Observability migration, Tasks 04, 06,
  and 10, current Office metric variables, structured logs, and root
  observability guidance.
- **Output contract:** Report every preserved, retired, and replacement metric
  and log event, safe labels, files changed, exact commands and results,
  blockers, risks, and synchronized task and plan status.
- **Verification:** `cd apps/backend && go test -tags fts5 ./internal/agent/runtime/dynamic/... ./internal/office/scheduler/...`
- **Risks:** Labels must exclude prompts, credentials, raw account IDs, and unbounded profile names.

## Results

Not started in this implementation slice. Dynamic routing metrics and the
Office observability migration remain deferred to the later implementation
wave.
