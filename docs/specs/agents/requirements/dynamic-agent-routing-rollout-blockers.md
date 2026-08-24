---
status: active
system: agents
created: 2026-08-15
owners:
  - cfl
---
# Dynamic Agent Routing Rollout Blockers Requirements

## Overview

This repair spec amends [Dynamic Agent Routing](dynamic-agent-routing.md). It turns the review findings into release gates for the existing implementation plan. The telemetry-routing package remains separate and is not part of this repair.

## Requirements

### REQ-AGENTS-DYNAMIC-AGENT-ROUTING-ROLLOUT-BLOCKERS-001: Dynamic Agent Routing Rollout Blockers

**Intent:** This repair spec amends [Dynamic Agent Routing](dynamic-agent-routing.md). It turns the review findings into release gates for the existing implementation plan. The telemetry-routing package remains separate and is not part of this repair.

#### Acceptance criteria

- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-ROLLOUT-BLOCKERS-001.1:** **GIVEN** a failed provider emitted a tool call, **WHEN** its lifecycle error arrives, **THEN** no replacement provider is launched and the route exposes a recovery action.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-ROLLOUT-BLOCKERS-001.2:** **GIVEN** three eligible candidates, **WHEN** the first two fail before any result, **THEN** the third receives the persisted continuation and becomes the selected execution profile.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-ROLLOUT-BLOCKERS-001.3:** **GIVEN** a waiting route, **WHEN** Retry or Try next is submitted, **THEN** one request performs the complete handoff and a launch failure returns an actionable non-starting state.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-ROLLOUT-BLOCKERS-001.4:** **GIVEN** a utility call attached to task session `S`, **WHEN** two calls start, **THEN** they use different `utility:` route identities and neither consumes task route state.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-ROLLOUT-BLOCKERS-001.5:** **GIVEN** two concrete profiles share one proven credential binding, **WHEN** one opens a quota circuit, **THEN** new selections skip that resource and a single expired probe controls recovery.
- **AC-AGENTS-DYNAMIC-AGENT-ROUTING-ROLLOUT-BLOCKERS-001.6:** **GIVEN** the flag is disabled, **WHEN** a task picker is opened, **THEN** dynamic profiles are absent while existing dynamic settings and sessions are retained.

## Migrated source detail

This repair spec amends [Dynamic Agent Routing](dynamic-agent-routing.md). It
turns the review findings into release gates for the existing implementation
plan. The telemetry-routing package remains separate and is not part of this
repair.

## Safety contract

- A provider replacement is allowed only when the failed attempt has explicit
  pre-result evidence. Any assistant output, thinking output, tool call, tool
  effect, missing evidence, or evidence-read failure fails closed.
- Before a cross-provider launch, the conductor builds and persists a bounded
  continuation package. The downstream adapter delivers that package in the
  replacement prompt and never reuses the predecessor ACP identity.
- Launch fallback walks every eligible candidate once, subject to route
  generation fencing. A failure of one successor does not strand a configured
  third candidate.
- Retry and Try next are one atomic route action. The backend owns selection,
  predecessor shutdown, successor launch, and the final durable state. A launch
  failure leaves recovery controls available and never leaves a route in
  `starting` without an in-flight launch owner.
- Every utility invocation receives a unique transient route identity, even
  when its template is attached to a task session. Utility fallback also
  requires the same explicit pre-result evidence.

## Shared health contract

- Concrete adapter families provide a non-secret credential-binding descriptor.
  Profiles that prove the same binding share the credential/provider circuit;
  unknown bindings use isolated profile scope.
- Qualifying classified failures open the candidate's correctly scoped circuit.
  Selection of an expired open circuit uses one exclusive probe lease, and the
  launch result releases that lease as success or failure.

## Settings and selection contract

- Settings shows all Dynamic profiles in a list and always provides an Add
  profile action. Each profile has a direct editor route.
- A dynamic profile exposes enablement, candidate enablement, ordering, and a
  configured action for provider errors (`retry_same`, `try_next`, or `stop`).
- Profile kind is preserved in flattened picker options. When the feature flag
  is off, dynamic profiles remain editable in settings but are excluded from
  every new-selection picker.
- Dynamic profiles do not offer Duplicate because the backend rejects dynamic
  profile duplication.

## Acceptance scenarios

- **GIVEN** a failed provider emitted a tool call, **WHEN** its lifecycle error
  arrives, **THEN** no replacement provider is launched and the route exposes a
  recovery action.
- **GIVEN** three eligible candidates, **WHEN** the first two fail before any
  result, **THEN** the third receives the persisted continuation and becomes
  the selected execution profile.
- **GIVEN** a waiting route, **WHEN** Retry or Try next is submitted, **THEN**
  one request performs the complete handoff and a launch failure returns an
  actionable non-starting state.
- **GIVEN** a utility call attached to task session `S`, **WHEN** two calls
  start, **THEN** they use different `utility:` route identities and neither
  consumes task route state.
- **GIVEN** two concrete profiles share one proven credential binding, **WHEN**
  one opens a quota circuit, **THEN** new selections skip that resource and a
  single expired probe controls recovery.
- **GIVEN** the flag is disabled, **WHEN** a task picker is opened, **THEN**
  dynamic profiles are absent while existing dynamic settings and sessions are
  retained.

## Scope boundary

This repair covers the existing dynamic-agent-routing backend and settings
plan. It does not implement telemetry routing, cost or subscription signals,
Office handoff, or the dedicated rollout E2E wave.
