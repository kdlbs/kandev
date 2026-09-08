---
id: "07-utility-profile-integration"
title: "Utility profile integration"
status: in_progress
wave: 7
depends_on: ["06-logical-session-integration"]
plan: "plan.md"
spec: "../../specs/agents/requirements/dynamic-agent-routing.md"
---

# Task 07: Utility profile integration

- **Acceptance:** Make dynamic profiles eligible for all utility bindings, route
  calls through the shared resolver, persist logical and concrete attribution,
  and allow only pre-result classified fallback.
- **Files likely touched:** `apps/backend/internal/utility/{profilebinding,service,handlers,store}/**`,
  `apps/backend/internal/agent/hostutility/**`,
  `apps/backend/internal/plugins/host_utility*.go`.
- **Dependencies:** Task 06.
- **Parallelism:** parallel-safe with Task 09. Owned backend utility and plugin
  files do not overlap Task 09 frontend session and chat files.
- **Inputs:** Spec Use by utility agents and utility scenarios, Task 06 shared
  resolver, the existing utility profile binding and call-record contracts.
- **Output contract:** Report utility fallback boundaries and attribution,
  files changed, exact commands and results, blockers, risks, and synchronized
  task and plan status.
- **Verification:** `cd apps/backend && go test -tags fts5 ./internal/utility/... ./internal/agent/hostutility/... ./internal/plugins/...`
- **Risks:** Ambiguous partial/effectful utility attempts must fail closed rather than combine provider output.

## Results

In progress. Utility bindings now retain the logical profile, persist the
concrete execution profile, pass the concrete ID to session and host executors,
expose that attribution in call DTOs, and advance only classified pre-result
failures. Each utility invocation has an isolated transient route identity, and
partial responses are rejected before fallback. Ambiguous or post-start
runtime failures remain fail-closed.

Verification:

- `go test -tags fts5 ./internal/utility/... ./internal/agent/hostutility/... -count=1`

The utility suite passed, including a concrete-profile fallback test. Plugin
host utility routing and caller-selection E2E coverage remain open.
