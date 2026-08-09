---
id: "05-prove-autopilot-e2e"
title: "Prove the autopilot lifecycle end to end"
status: done
wave: 4
depends_on:
  - "03-build-parent-question-lifecycle"
  - "04-build-autopilot-ui"
plan: "plan.md"
spec: "../../specs/tasks/autopilot-mode.md"
---

# Task 05: Prove the Autopilot Lifecycle End to End

## Acceptance

- A deterministic test creates an autopilot child, observes its prompt/tool
  contract, opens one parent question, waits visibly, accepts one correlated parent
  reply, and resumes the child exactly once.
- Desktop and mobile Playwright coverage verifies the create switch, yellow chip,
  secondary autopilot icon, primary question indicator, cleared wait state, and
  absence of horizontal overflow.
- Restart or integration coverage proves a pending question reconstructs without
  duplicating the parent prompt, and normal task clarification remains unchanged.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --project chromium tests/task/autopilot-mode.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-autopilot-mode.spec.ts)
(cd apps/backend && go test ./internal/integration/... ./internal/mcp/handlers/... ./internal/task/statussummary/...)
```

## Files likely touched

- `apps/backend/cmd/mock-agent/scenarios.go`
- `apps/backend/cmd/mock-agent/script.go`
- `apps/backend/cmd/mock-agent/mock_agent_test.go`
- `apps/backend/internal/integration/mcp_integration_test.go`
- `apps/web/e2e/tests/task/autopilot-mode.spec.ts`
- `apps/web/e2e/tests/task/mobile-autopilot-mode.spec.ts`
- `apps/web/e2e/fixtures/backend.ts`

## Dependencies

- Task 03 completes the backend lifecycle.
- Task 04 completes the shared desktop/mobile UI.

## Parallelism

Runs after all behavior and UI tasks. Keep E2E fixture changes scoped to deterministic
test controls; do not duplicate product state machines in the mock agent.

## Inputs

- All spec acceptance scenarios.
- Existing task creation, MCP integration, mobile sidebar, and task-status E2E
  exemplars.
- The repository E2E README and containers-project boundary.

## Output contract

Report each scenario and project run, mock directive/API added, task/session states
observed, parent and child message counts, restart evidence, mobile viewport and
overflow measurement, exact commands/results, and any scenario left to a lower test
layer with justification.

## Results

Done. The mock agent invokes the real parent-question tool, waits for the parent
answer, and replies through the real correlated message path. Chromium and mobile
Playwright scenarios pass, including creation controls, prompt waiting, identity and
question icons, chat chip, answer/resume, and horizontal-overflow checks. Backend
message/repository tests cover durable question reconstruction and idempotent
answer state. A full process-restart Playwright scenario was not run; that remains
an explicit follow-up for the fixture rather than an unverified claim.
