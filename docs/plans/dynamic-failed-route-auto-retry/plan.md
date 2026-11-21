---
created: 2026-10-20
status: done
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-02.md
legacy_specs: []
---

# Implementation Plan: Workspace opt-in automatic retry of a failed dynamic route

## Overview

When a provider-capacity failure leaves a dynamic task route in
`action_required`, or in `waiting` because no candidate is eligible, the
session waits until a person presses Retry. This plan adds a per-workspace
setting, off by default and independent of mid-turn provider failover, that
submits that same Retry automatically once the candidates' circuits recover,
up to a workspace limit of 1 to 3 retries between successful turns. The default
behavior is unchanged.

## Scope

In scope: the two workspace settings (model, SQLite columns and migrations,
validation, HTTP and WebSocket update, DTO, events, boot state), the durable
retry chain in the route policy state, the scheduling, restart re-arm, and
consumption on the shared dynamic recovery timer, the resolver's automatic
retry action and the Retry reselection of a waiting route, the workspace
settings controls with locale copy, the requirement, safety-contract, and
design updates, and their tests.

Excluded: FAILED sessions (an automatic relaunch keeps refusing terminal
sessions), launch failures (`route_action_launch_failed`), account-block holds,
unclassified and non-capacity codes, Office sessions, utility invocations, a
new runtime flag (the behavior sits behind `features.dynamicAgentRouting`), and
a recovery-banner indicator for a pending automatic retry.

## Technical approach

`routeDynamicAgentFailureWithEvidence` and the mid-turn failover worker hand a
qualifying failure to `scheduleFailedRouteAutoRetry` after the route reaches
its final state. The engine records one pending retry per generation in
`PolicyState.FailedRouteRetry` with a status- and snapshot-fenced write, due at
the later of one minute and the earliest usable candidate circuit expiry. The
chain's attempts and retried failures are carried to every new generation and
cleared on a successful turn. `armDynamicRecoveryTimer` is the timer that policy
waits already use; startup re-arms pending retries from
`ListRecoveryRouteStates`. The run reuses `applyAutomaticRouteAction`: the
route-action lock, the settled-turn check, an admission that consumes the
pending retry, and the backend route-action transaction with `Automatic` set,
which maps to the resolver's `auto_retry` action and its
`auto_failed_route_retry` reason.

## Work orders

- [x] [Task 01: Workspace opt-in automatic failed-route retry](task-01-workspace-failed-route-auto-retry.md)

## Changed-path coverage

The work order owns the backend workspace setting plumbing under
`apps/backend/internal/task/`, the boot state field and route-action mapping in
`apps/backend/internal/backendapp/`, the orchestrator scheduler and hooks in
`apps/backend/internal/orchestrator/`, the resolver action and the engine chain
under `apps/backend/internal/agent/runtime/`, the web workspace settings,
store, and WebSocket plumbing under `apps/web/`, the locale catalogs, the new
E2E spec, `docs/specs/agents/requirements/dynamic-agent-routing.md`,
`docs/specs/agents/requirements/dynamic-agent-routing-rollout-blockers.md`, and
`docs/specs/agents/system-design/dynamic-agent-routing-02.md`.

## Verification

Permanent tests cover the off default, one retry at the earliest circuit expiry
with the injectable clock, early firing, the limit, the repeated-failure stop, a
duplicate failure, a stale generation, a superseded execution, an ineligible
code, restart re-arm and a consumed retry after restart, the setting turned off
before the retry, chain reset after a completed turn, the mid-turn interaction,
the engine chain and due-time rules, the resolver actions, the backend request
mapping, the workspace storage round trip and validation, the settings card,
and the save handler. A Playwright spec covers the toggle and the limit.

## Risks

- An automatic retry repeats provider work after a pre-result failure, or after
  a mid-turn handoff when mid-turn failover is also on. The successor receives
  only durable task state; the setting is off by default and capped.
- The retry is at most once: a crash between consuming the schedule and the
  route action leaves the route in its recovery state for a person.
