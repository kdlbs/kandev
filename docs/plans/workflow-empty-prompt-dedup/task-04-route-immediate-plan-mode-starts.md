---
id: "04-route-immediate-plan-mode-starts"
title: "Route immediate plan-mode starts"
status: done
wave: 3
depends_on:
  - "03-prove-plan-mode-workflow-flow"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-004
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-004.1
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-004.2
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-004.3
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-004.4
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
---

# Task 04: Route Immediate Plan-Mode Starts

## Summary

Route every immediate agent start to the workflow's first automatic-start
step. Plan mode changes agent execution but does not override this destination.

## In scope

- Give `StartAgent` precedence over `PlanMode` during initial step resolution.
- Retain explicit-step precedence and all destination fallbacks.
- Update backend routing coverage for combined start and plan-mode intent.
- Update desktop and mobile Playwright coverage.
- Update the public task-creation and workflow guidance.

## Out of scope

- Plan-only prepared sessions with `start_agent=false`.
- Task-dialog layout, labels, navigation, and touch behavior.
- Workflow editor behavior and stored workflow definitions.
- Automatic entry actions after later task moves.

## Acceptance

- A request with `start_agent=true` and `plan_mode=true` resolves through
  `ResolveAutoStartStep` unless it supplies an explicit step.
- Desktop and mobile plan-mode starts land in the first automatic-start step.
- The duplicate-prompt E2E retains its original message and turn assertions.

## Verification

```bash
(cd apps/backend && go test ./internal/task/service ./internal/task/handlers -run 'TestResolveWorkflowStep_RoutesByCreateIntent|TestHTTPCreateTask_StartAgentSelectsDestinationStep|TestWSCreateTask_StartAgentSelectsDestinationStep' -count=1)
(cd apps/web && pnpm e2e:run tests/workflow/start-step-vs-auto-start-step.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/workflow/mobile-start-step-vs-auto-start-step.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/service_tasks_step_routing_test.go`
- `apps/backend/internal/task/handlers/task_handlers_start_agent_routing_test.go`
- `apps/web/e2e/tests/workflow/start-step-vs-auto-start-step.spec.ts`
- `apps/web/e2e/tests/workflow/mobile-start-step-vs-auto-start-step.spec.ts`
- `docs/public/tasks-and-workflows.md`
- `docs/public/workflow-tips.md`

## Dependencies

- Task 03 provides the existing duplicate-prompt browser scenario.

## Risks

- Replacing the old placement assertion can remove the only browser proof for
  the duplicate-prompt repair.
- A transport test can pass without exercising the combined request flags.
- The mobile test can select a remembered workflow unless it pins that choice.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-004`.
- The creation destination section in the owning system design.
- Existing service, transport, and Playwright routing tests.
- The shared desktop and mobile task-create submission handler.

## Results

- `StartAgent` now takes precedence over `PlanMode` in initial workflow-step
  resolution. Explicit steps, plan-only prepared sessions, and the existing
  automatic-start fallback chain are unchanged.
- HTTP and WebSocket contract tests preserve the combined `start_agent=true`
  and `plan_mode=true` intent through the task service.
- The desktop browser scenario proves normal and plan-mode starts use the same
  automatic-start destination. Its duplicate-prompt regression now makes Plan
  the first automatic-start destination before moving into the second empty
  automatic-start step.
- The mobile browser scenario uses the phone-only Plan mode button. It checks
  the persisted destination and shows the task in the In Progress phone board.
- Public task-creation and workflow guidance now document the common routing
  rule.
- Focused backend verification passed: 14 tests in 2 packages.
- Desktop Playwright verification passed: 3 tests.
- Mobile Chrome Playwright verification passed: 1 test.
- Public documentation tests passed: 61 tests and 41 published pages validated.
