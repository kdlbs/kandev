---
created: 2026-10-20
status: done
requirements:
  - REQ-AGENTS-SESSION-CEILING-001
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
legacy_specs: []
---

# Implementation Plan: Agent origin ceiling admission

## Overview

A task that an agent creates through Kandev MCP with `start_agent` was started
without `AutoStart`, so the session ceiling classified it as a manual launch and
admitted it through the manual override. Agents creating tasks in a loop could
therefore run the instance well above the configured ceiling.

## Scope

In scope: `LaunchSessionRequest.Automatic`, origin selection in
`session_launch.go`, `task_create_prompt.go`, `startCreatedSession` and the
`start_created` replay payload, and the MCP create-task auto-start requests,
with permanent tests.

Excluded: `spawn_session` and message follow-up resumes, whose MCP responses do
not yet report a queued launch, server recovery paths, and any change to the
manual override for user actions.

## Technical approach

Reuse the existing automatic origin and its `startTaskOptions.Origin` override
(the same path Office-routed starts use). Add the matching option to
`startCreatedSession` and round-trip it through the replay payload.

## Work orders

- [x] [Task 01: Admit agent-requested starts as automatic](task-01-agent-origin-ceiling-admission.md)

## Changed-path coverage

The work order owns the orchestrator and MCP handler files above, their
permanent tests, the new criterion in the session concurrency ceiling
requirement, and the matching paragraph in its system design.
