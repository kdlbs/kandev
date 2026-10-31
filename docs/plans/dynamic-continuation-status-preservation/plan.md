---
created: 2026-10-20
status: done
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-01.md
legacy_specs: []
---

# Implementation Plan: Preserve dynamic continuation across route status changes

## Overview

The conductor saves a continuation package straight to the repository, but the
engine keeps its own cached route state whose continuation stays empty. The
next status transition the engine writes for the same generation, such as
marking the successor active, persists the cached state and erases the saved
handoff. The successor then starts without the continuation it was promised.

## Scope

In scope: `apps/backend/internal/agent/runtime/dynamic/conductor.go`, the new
`engine_continuation.go`, the policy-failure and cancel transitions in
`engine.go`, and SQLite regression tests through the public repository APIs.

Excluded: schema, SQL adapters, configuration, providers and UI.

## Technical approach

Route continuation saves through the engine when one is attached. The engine
holds its existing status-transition mutex, refuses a stale generation, saves
durably, and only then refreshes its cached continuation. A failed save leaves
the cache unchanged. Every same-generation status transition holds that mutex
across its route read, durable write and cache update, so a save cannot land
between a transition's read and write and be overwritten by its stale snapshot.

## Work orders

- [x] [Task 01: Keep the saved continuation through status transitions](task-01-continuation-status-preservation.md)

## Changed-path coverage

The work order owns the dynamic runtime files above, the regression test, and
`AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.25`.
