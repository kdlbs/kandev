---
id: "03-taskless-permission-e2e"
title: "End-to-end taskless permission regression"
status: done
wave: 3
depends_on: ["02-office-taskless-permission-policy"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-TASKLESS-001
acceptance_criteria:
  - AC-OFFICE-TASKLESS-001.1
  - AC-OFFICE-TASKLESS-001.9
system_design:
  - ../../specs/office/system-design/taskless-run-sessions.md
---

# Task 03: End-to-end taskless permission regression

## Summary

Prove with the real backend and mock agent that a taskless routine whose agent
requests a permission reaches a terminal run outcome and frees the agent.

## In scope

- A line-anchored mock-agent directive, `e2e:office_scenario(<name>)`, that
  runs a registered scenario from anywhere in an Office prompt. The spec puts
  it in the agent's `AGENTS.md`, which taskless prompts inline.
- `apps/web/e2e/tests/office/taskless-routine-permission.spec.ts`, modelled on
  `taskless-routine-session.spec.ts`.

## Out of scope

- UI changes.

## Acceptance

- Two cases, one per decision path: with the launched profile's `auto_approve`
  off the run records `rejected`; with it on, `approved`. The e2e workspace has
  no provider routing, so the launched profile is the Office agent's own row. Both runs reach
  `finished` and the agent returns to `idle`. With the Office handler
  disabled, both runs stay `claimed`.

## Verification

```bash
(cd apps/backend && go test ./cmd/mock-agent/ -count=1)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/office/taskless-routine-permission.spec.ts)
```

## Files likely touched

- `apps/backend/cmd/mock-agent/handler.go`
- `apps/backend/cmd/mock-agent/office_scenario.go` (new)
- `apps/backend/cmd/mock-agent/office_scenario_test.go` (new)
- `apps/web/e2e/tests/office/taskless-routine-permission.spec.ts` (new)

## Dependencies

Task 02.

## Risks

- The directive must reach the prompt: confirm whether the routine description
  is included in a taskless prompt; if not, place it in the agent's
  instructions.

## Parallelism

`sequential`

## Inputs

- `apps/web/e2e/tests/office/taskless-routine-session.spec.ts`
- `apps/backend/cmd/mock-agent/scenarios.go` (`scenarioPermissionFlow`)

## Results

Done 2026-10-20.

- `go test ./cmd/mock-agent/ -count=1`: pass.
- `pnpm e2e:run --host --shards 1 --project chromium tests/office/taskless-routine-permission.spec.ts`:
  2 passed (`rejected` with the launched profile's auto-approve off, `approved` with it on).
- Manual check on a routed dev workspace (2026-10-21): the agent's own flag had no
  effect; enabling auto-approve on the routed claude-acp profile produced `approved`.
- Red check: with the Office subscription removed, both cases failed with the
  run detail still `claimed`.
