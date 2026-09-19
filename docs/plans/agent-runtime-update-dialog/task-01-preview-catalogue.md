---
id: "01-preview-catalogue"
title: "Restore the preview install catalogue"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/runtime-updates.md"
---

# Task 01: Restore the preview install catalogue

## Acceptance

- PR previews enable `mock-agent` while retaining the built-in real-agent
  catalogue.
- Settings can derive unavailable built-in agents and their install scripts
  from the preview registry.
- E2E-only `KANDEV_MOCK_AGENT=only` semantics remain unchanged elsewhere.

## Verification

- RED/GREEN:
  `cd apps/backend && go test -run TestBuildExtractScript ./cmd/preview`
- Supporting:
  `cd apps/backend && go test -run TestProvide_MockAgentModes ./internal/agent/registry`

## Files likely touched

- `apps/backend/cmd/preview/sprite_ops.go`
- `apps/backend/cmd/preview/sprite_ops_test.go`

## Dependencies

None.

## Parallelism

Sequential by default. It is file-disjoint from Task 02 and may run in
parallel only with explicit user authorization.

## Inputs

- Spec agent-catalogue and Settings scenarios
- `registry.Provide` mock-mode contract
- Confirmed preview script root cause in `plan.md`

## Output contract

Report RED and GREEN results, changed files, preview-mode risk, and update this
task plus `plan.md` status in the primary conversation.
