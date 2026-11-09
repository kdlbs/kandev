---
id: "01-bind-promoted-execution-profile"
title: "Bind the promoted execution to the resolved profile"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.31
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-02.md
---

# Task 01: Bind the promoted execution to the resolved profile

## Outcome

A promoted workspace-only execution carries the concrete profile used to build
its command, and the executor-running row records it.

## In scope

- After promotion preparation succeeds, set the execution's profile ID to the
  resolved profile when it differs.
- Persist the executor-running row when either the concrete profile or the
  acting Office profile changes.

## Exclusions

- The Office profile stays in `OfficeAgentProfileID`.
- No change to inherited-workspace policy or archived-dependency handling.

## Acceptance conditions

1. A promotion that starts with a logical profile ends with the concrete
   profile on the execution, and the Office identity remains separate.
2. With no Office profile supplied, the concrete profile is still written to
   the executor-running row.

## Verification

Disposable promotion and SQLite-backed checks: the promotion case failed on
current main with the logical profile still present, then passed after the
change. Normal commit hooks, including changed-package Go lint, passed.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`

## Dependencies

None.
