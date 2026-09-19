---
id: "06-logical-session-integration"
title: "Logical session integration"
status: in_progress
wave: 6
depends_on: ["05-acp-conductor"]
plan: "plan.md"
spec: "../../specs/agents/requirements/dynamic-agent-routing.md"
---

# Task 06: Logical session integration

- **Acceptance:** Route task/workflow launches through the resolver, keep one
  logical task session, persist concrete execution and route generation on each
  turn, emit generation-fenced structured route/capability events, and add the
  `session.route_action` request with expected-generation conflict handling.
- **Files likely touched:** `apps/backend/internal/orchestrator/{session_launch,task_operations,event_handlers_streaming,workflow_session_config}*.go`,
  `apps/backend/internal/task/{models,dto,repository/sqlite,service}/**`,
  `apps/backend/internal/gateway/websocket/*`.
- **Dependencies:** Task 05.
- **Parallelism:** sequential.
- **Inputs:** Spec Transparent profile execution, Route and capability events,
  API surface, and route-action scenarios, Task 05, existing WebSocket session
  request and access-control patterns.
- **Output contract:** Report the request and event contracts, authorization and
  conflict behavior, files changed, exact commands and results, blockers,
  risks, and synchronized task and plan status.
- **Verification:** `cd apps/backend && go test -tags fts5 ./internal/orchestrator/... ./internal/task/... ./internal/gateway/websocket/...`
- **Risks:** Existing concrete sessions and session subscriptions must retain their current behavior.

## Results

In progress. Added logical and concrete session fields, route generation and
turn attribution persistence, generation-fenced `session.route_action`, stale
conflict snapshots, recovery-state handling, and resolver-backed task launch
and resume paths. Route actions now also require a settled turn. The
retry-same path preserves the failed candidate and stale route claims fail
closed after restart. The rollout-blocker repair makes the route action own
selection, predecessor shutdown, successor launch, and durable recovery state
as one backend operation, and fails closed when continuation evidence is
ambiguous.

Verification:

- `go test -tags fts5 ./internal/orchestrator/... ./internal/task/... ./internal/backendapp/... -count=1`

The focused backend suite passed, including active-turn rejection and
settled-turn route-action coverage. Full structured route/capability event
coverage, conductor event subscriptions, and dedicated E2E coverage remain
open.
