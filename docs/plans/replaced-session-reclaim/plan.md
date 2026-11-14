---
created: 2026-10-20
status: done
requirements:
  - REQ-TASKS-PRIMARY-SESSION-OWNERSHIP-001
system_design:
  - ../../specs/tasks/system-design/primary-session-ownership.md
legacy_specs: []
---

# Implementation Plan: Replaced session reclaim

## Overview

When the user makes another session primary, the previous primary's agent
process keeps running until workspace idle suspension or a manual stop, so it
holds a full agent process tree for the idle window. Behind
`features.replacedSessionReclaim`, off in every shipped profile, Kandev now
stops that execution once it settles, without changing the session state.

## Scope

In scope: `apps/backend/internal/orchestrator` (`replaced_session_reclaim.go`,
`PromotePrimarySession` and its `session.set_primary` handler, the
`SetPrimarySession` write lock, the idle-reaper tick, and `ServiceConfig`), the runtime flag (`FeaturesConfig`, `runtimeflags/registry.go`,
`profiles.yaml`, `backendapp/orchestrator.go`, frontend `defaultFeatureFlags`),
and `docs/public/configuration.md` and `agent-communication.md`.

Excluded: automatic promotion, session deletion, workflow profile switches and
their queue transfer (the switch already parks or completes its source), idle
suspension policy, Office and coordinator lifecycles, and queue routing.

## Technical approach

Register the exact execution the replaced session held at the explicit
promotion, then stop that execution with `StopExecution` on an idle-reaper tick
once the session has no known idle work, reusing the idle reaper's eligibility
checks. Registrations that can never be stopped are dropped.

## Work orders

- [x] [Task 01: Stop settled replaced session runtimes](task-01-replaced-session-reclaim.md)

## Changed-path coverage

The work order owns the orchestrator files above, their permanent tests, the
flag plumbing, `AC-TASKS-PRIMARY-SESSION-OWNERSHIP-001.6`, the replaced session
reclaim section of the primary session ownership design, and the public docs.
