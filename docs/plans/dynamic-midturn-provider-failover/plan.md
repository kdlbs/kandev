---
created: 2026-10-20
status: done
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-02.md
legacy_specs: []
---

# Implementation Plan: Workspace opt-in mid-turn provider failover

## Overview

A dynamic task attempt that hits a provider rate limit after it already
produced output or effects stops in manual recovery, because the pre-result
safety contract forbids replacing the provider. This plan adds a per-workspace
setting, off by default, that lets such a classified provider-capacity failure
perform one automatic Try next per failed route generation, using the existing
atomic route action and continuation package. The default behavior is
unchanged.

## Scope

In scope: the workspace setting (model, SQLite column and migration, HTTP and
WebSocket update, DTO, events, boot state), the orchestrator trigger and
detached worker, the resolver's automatic Try next action, the workspace
settings toggle with locale copy, the requirement, safety-contract, and design
updates, and their tests.

Excluded: new provider classification, candidate-order or circuit changes, a
new runtime flag (the behavior already sits behind `features.dynamicAgentRouting`
and only applies to dynamic sessions), Office routing, utility invocations, and
durable persistence of the automatic-handoff chain.

## Technical approach

`routeDynamicAgentFailureWithEvidence` keeps its decline for an effect-unsafe
classified failure; it additionally asks `scheduleMidTurnProviderFailover` to
queue one automatic action when the failure qualifies. The trigger is fenced on
the live prompt attempt (execution plus prompt generation). A process-local
claim admits one handoff per failed generation and execution and stops a chain
that repeats a code on a candidate it already tried. The detached worker reuses
the route-action lock and settled-turn check of `ApplyRouteAction`, re-reads
the session, and calls the same backend route-action handler with `Automatic`
set, so selection, persistence, predecessor shutdown, continuation, and
successor launch stay in one transaction.

## Work orders

- [x] [Task 01: Workspace opt-in mid-turn provider failover](task-01-workspace-midturn-failover.md)

## Changed-path coverage

The work order owns the backend workspace setting plumbing under
`apps/backend/internal/task/`, the boot state field and route-action mapping in
`apps/backend/internal/backendapp/`, the orchestrator trigger in
`apps/backend/internal/orchestrator/`, the resolver action in
`apps/backend/internal/agent/runtime/`, the web workspace settings, store, and
WebSocket plumbing under `apps/web/`, the locale catalogs, the new E2E spec,
`docs/specs/agents/requirements/dynamic-agent-routing.md`,
`docs/specs/agents/requirements/dynamic-agent-routing-rollout-blockers.md`, and
`docs/specs/agents/system-design/dynamic-agent-routing-02.md`.

## Verification

Permanent tests cover the off default, one automatic handoff with the bounded
continuation, duplicate delivery, repeated same-code stop, chain reset after a
completed turn, ineligible codes, unknown evidence, other prompt generations,
stale generations, superseded executions, the resolver action, the backend
request mapping, the workspace storage round trip, the settings card, and the
save handler. A Playwright spec covers the toggle.

## Risks

- An automatic handoff after effects can repeat work. The successor receives
  only durable task state and must inspect it before repeating work; the
  setting is off by default and documented as an opt-in.
- The chain is process-local, so a restart forgets prior automatic handoffs;
  each failed generation still hands off at most once.
