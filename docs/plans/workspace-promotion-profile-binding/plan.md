---
created: 2026-11-08
status: done
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-02.md
legacy_specs: []
---

# Implementation Plan: Workspace promotion profile binding

## Overview

A workspace-only execution can be created with a logical profile before a
concrete dynamic candidate is selected. When it is promoted to an agent
execution, command construction uses the concrete profile but the execution's
profile ID could remain the earlier logical ID, so startup, persistence, and
restart read the wrong profile. After promotion preparation succeeds, bind the
execution to the resolved profile and persist it even when no Office profile
was supplied.

## Scope

In scope: `promoteWorkspaceExecution` in
`apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`.

Out of scope: inherited-workspace policy, archived-dependency handling, route
selection, and the separate Office profile field.

## Requirement and design references

| Work order | Acceptance criteria | System design |
| --- | --- | --- |
| Task 01 | `AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.27` | [Dynamic agent routing part 2](../../specs/agents/system-design/dynamic-agent-routing-02.md#persistence-guarantees) |

## Work orders

- [x] [Task 01: Bind the promoted execution to the resolved profile](task-01-bind-promoted-execution-profile.md)

## Risks

- The executor-running row is now written when only the concrete profile
  changes. The write is the existing persistence path and runs only after the
  final agent-launch admission succeeds.
