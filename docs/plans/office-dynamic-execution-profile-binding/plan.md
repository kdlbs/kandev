---
created: 2026-09-28
status: done
requirements:
  - REQ-OFFICE-AGENTS-001
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
system_design:
  - ../../specs/office/system-design/agents-01.md
  - ../../specs/agents/system-design/dynamic-agent-routing-01.md
legacy_specs: []
---

# Implementation Plan: Office Dynamic Execution Profile Binding

## Overview

An Office agent is a persistent `agent_profiles` row whose ID is the logical
`agent_profile_id` for assignments, instructions, skills, budgets, permissions,
and Office history. When onboarding or configuration selects a dynamic
execution profile, that selection is a routing owner, not a concrete CLI
family. Without a durable binding, launch resolution looked up the Office row
ID as a dynamic profile and failed to load its routes, so automatically
assigned or routine-created work could stay in `SCHEDULING`.

This plan persists the selected execution profile on the stable Office identity
and resolves dynamic candidates through it.

## Scope

### In scope

- Persist `execution_agent_profile_id` on `agent_profiles` rows through the
  shared settings store, with a replayable `ADD COLUMN` migration and empty
  default for existing rows.
- Bind the execution profile on Office onboarding and profile configuration when
  the selected source is the virtual dynamic family.
- Resolve the dynamic candidate set through the bound profile in the shared
  dynamic resolver while keeping the Office ID as the logical session identity.
- Resolve a concrete candidate for a taskless Office run before launch, and
  restore a task to its previous state when dynamic resolution fails, so the
  task stays actionable instead of stranded in `SCHEDULING`.

### Out of scope

- Concrete-profile behavior, which continues to select itself.
- Cross-provider continuation and route telemetry, owned by the base routing
  design.
- Any public API or logical Office identity change.
- Unrelated timezone, routine, or UI work.

## Technical approach

The Office identity and its execution profile share the unified
`agent_profiles` table but are distinct logical objects. Store the optional
binding in a new nullable-with-default column. The shared resolver consults the
binding only for Office rows that set it and otherwise resolves the requested ID
unchanged, so non-Office launches keep `execution_profile_id == agent_profile_id`.

Taskless runs reach the launcher with no workspace route. When the bound Office
agent carries a dynamic execution profile, resolve a concrete candidate through
the shared resolver before launch rather than launching the virtual family,
which the lifecycle rejects. When resolution fails, finish the run session and
return the error. Task-backed launches restore the pre-`SCHEDULING` task state
only while the task is still `SCHEDULING`, so a concurrent transition wins.

## Tests

- `AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.4`: dynamic resolver tests cover Office
  binding source selection, absent-binding fallthrough, and fail-closed behavior
  when the bound profile is missing.
- `AC-OFFICE-AGENTS-001.2`: onboarding and profile-configuration tests assert the
  binding is set for a dynamic source and untouched for a concrete source;
  settings-store tests cover the round trip and legacy default; launcher tests
  cover taskless dynamic resolution; orchestrator tests cover taskless binding
  and `SCHEDULING` restoration.

## Work orders

- [x] [Task 01: Bind the Office execution profile](task-01-bind-execution-profile.md)

## Verification results

```bash
cd apps/backend && go build ./...
cd apps/backend && go test -p 2 ./internal/agent/runtime ./internal/agent/settings/store \
  ./internal/office/agents ./internal/office/onboarding ./internal/backendapp ./internal/orchestrator
```

`go build ./...` passed. The focused changed-behavior tests passed for the
dynamic resolver, settings store, Office agents, Office onboarding, backend-app
launcher, and orchestrator task operations.

## Risks

- The binding must not be treated as a session identity; sessions, budgets, and
  Office history stay keyed by the Office row ID.
- A model, mode, and account for a launch come from the resolved concrete
  candidate, not from the Office row.
- Resolution failure must never launch the virtual family and must leave the
  task/run retryable.
- Blank bindings must preserve current concrete-profile behavior and existing
  rows must survive the migration with an empty binding.
