---
spec: docs/specs/agents/requirements/dynamic-agent-routing.md
created: 2026-08-14
status: in_progress
---

# Implementation Plan: Dynamic Agent Routing

## Overview

Ship the guarded virtual profile family, transparent execution resolver,
fixed-order and error-based routing, durable conductor, shared health, and
Kanban, utility, and Office surfaces. Plugin cost and subscription telemetry
belong to a separate deferred package and are not implementation tasks here.

The follow-up [Provider Error Policies](../provider-error-policies/plan.md)
package replaces the generic candidate action map with shared transient and
hard policies, durable reset waits, bounded exponential retry, and explicit
skip/stop exhaustion. This package retains the compatibility action path until
that migration lands.

---

## Backend

### Rollout boundary

- Add `features.dynamicAgentRouting` / `KANDEV_FEATURES_DYNAMIC_AGENT_ROUTING`
  through `profiles.yaml`, `internal/common/config`, and `internal/runtimeflags`.
- Register it as mutable, experimental, high risk, and restart-required.
  Default every shipped profile to false. Gate profile mutation, dynamic
  execution, route actions, background probes, and capability advertisement at
  backend composition boundaries.
- Preserve stored dynamic configuration while disabled. Persisted dynamic
  sessions enter `action_required` without launch or resume. Concrete Kanban,
  utility, and Office execution remains unchanged. Add the explicit
  `office=on` and `dynamic=off` test cell.

### Virtual family and profile persistence

- Add a non-launchable `dynamic` built-in agent implementation under
  `internal/agent/agents` and register it unconditionally in
  `internal/agent/registry`. Keep it out of `ListInferenceAgentsWithContext`
  while keeping its DB parent row safe from orphan reconciliation.
- Extend `settings/models.AgentProfile` and DTOs with computed profile kind.
  Persist one-to-one configuration in `dynamic_agent_profiles` and ordered
  candidates in `dynamic_agent_routes` through
  `internal/agent/settings/store/sqlite.go`, do not add a duplicate kind column.
- Extend profile CRUD and typed dependency conflicts for dynamic configuration,
  candidate references, optimistic versioning, and confirmed stale-reference
  semantics.
- Treat profile kind as execution-family identity. Validate only launchable
  concrete profiles as candidates. Exclude rich Office identities,
  `AutoFallback=true` profiles, and dynamic profiles. Allow explicit
  `FallbackModel` as intra-profile policy.

### Core route engine

- Create `internal/agent/runtime/dynamic` for route selection, route state,
  fixed order, compatibility error actions, shared resource circuits, probe leases,
  and generation fencing. Reuse `runtime/routingerr` classification and migrate
  reusable Office backoff behavior without importing `internal/office`.
- Add a concrete-adapter `CredentialBindingDescriptor` contract after launch
  environment resolution. Add `CredentialBindingResolver` to canonicalize the
  descriptor and derive an HMAC-SHA-256 fingerprint with a persistent Kandev
  installation key. Use provider, binding, model, or profile scopes. Unknown
  bindings fall back to profile scope.
- Add shared SQLite repositories for route attempts, logical route state, and
  resource health under `internal/task/repository/sqlite/base_schema.go`,
  `base_migrations.go`, and focused repository files. The first committed route
  generation owns successor launch, stale writers fail closed. Candidate
  selection uses generation compare-and-swap, not an exclusive candidate lease.

### ACP conductor and continuation

- Add a profile-execution resolver above `runtime.Runtime`. Concrete selections
  delegate unchanged, dynamic selections create a logical conductor that owns
  downstream `Runtime.Launch`, `Resume`, `Stop`, and event subscriptions.
- Persist downstream ACP identity with its concrete profile. Same-profile resume
  can reuse it, cross-profile fallback always creates a fresh ACP session.
- Build bounded continuation from durable task, conversation, plan, tool, and
  repository state. Fence potentially effectful attempts before a successor.

### Task, workflow, and utility integration

- Keep `TaskSession.AgentProfileID` logical and
  `TaskSession.ExecutionProfileID` concrete. Add route generation and concrete
  attribution to turn snapshots and structured route-reason events.
- Route task/workflow launch through the shared resolver without caller kind
  branches. Replace live model, mode, configuration-option, and custom-command
  capabilities by route generation.
- Add the generation-fenced `session.route_action` WebSocket request. `retry`
  evaluates the full policy. `try_next` excludes the current candidate for one
  decision. Both return the authoritative route snapshot on stale input.
- Allow utility bindings to resolve dynamic profiles. Add
  `utility_agent_calls.execution_profile_id`, pre-result classified failures can
  select another candidate, while ambiguous effectful results fail closed.

### Office handoff

- Add `agent_profiles.execution_agent_profile_id` as a validated soft reference
  for Office identities and route Office launches through the shared resolver.
- Stop registering or calling Office routing mutation endpoints, onboarding
  seeding, scheduler router, and Office-specific routing UI. Preserve
  `office_workspace_routing`, `office_run_route_attempts`, and
  `office_provider_health` tables and rows unchanged and unread.
- Remove the Office `StartTaskWithRoute` and `RouteOverride` chain through the
  scheduler, orchestrator, executor, backend adapters, and lifecycle manager.
  Remove the legacy model, flag, and environment overlay helpers and tests.
  Keep `KANDEV_MOCK_PROVIDERS` as generic E2E infrastructure. Remove the
  Office-specific production provider catalogue.
- Retain Office wake-reason selection only as a request for another concrete or
  dynamic execution profile. Provider policy stays inside the selected dynamic
  profile.

### Observability migration

- Move `routing_route_attempts_total` and `routing_fallback_total` to the shared
  router with their current meaning.
- Retire `routing_provider_degraded_total` and
  `routing_provider_recovered_total`. Replace them with
  `routing_resource_circuit_opened_total` and
  `routing_resource_circuit_closed_total`.
- Retire `routing_parked_runs_total`. Replace it with
  `routing_waiting_sessions_total`.
- Apply the same mapping to `routing.metric.*` structured log events. Update
  root and backend `AGENTS.md` observability guidance. Do not emit raw account
  identifiers, credentials, prompts, or unbounded profile names.

## Frontend

### Profile settings

- Extend agent profile types/API normalization with `kind`, dynamic candidates,
  version, compatibility provider-error actions, and typed dependency details.
- Add the Dynamic family section and dedicated profile route. Desktop uses the
  settings-page editor, phone uses direct navigation with one internal scroll
  owner. Candidate selection uses the existing inset mobile picker drawer.
  Reorder actions remain visible and at least 44px.

### Routed chat

- Store route generation, current concrete profile, route state, and
  authoritative capabilities in the session slice. Discard stale generations.
- Keep the logical tab/author identity, render immutable concrete provider/model
  badges per turn, localize reason enums into durable route rows, and replace
  provider controls without remounting the composer.
- Render waiting and action-required states with generation-current Retry and
  Try next actions. The phone chat keeps these actions inline, uses the existing
  scroll owner, and does not replace or hide the composer.

### Utility and Office selectors

- Include eligible dynamic profiles in utility and Office execution-profile
  pickers without changing their saved logical ID shape.
- Remove legacy Office routing navigation and editors while leaving unrelated
  Office surfaces intact. Show shared route status through task/run read models.

### Mobile design contract

- Agent settings uses its existing tappable profile row and direct profile
  route. The dense editor remains a dedicated page with one scroll owner.
- Temporary candidate choices use the inset `MobilePickerSheet` pattern. Move
  controls remain visible, touch targets are at least 44px, and the page clears
  the safe area without horizontal document overflow.
- Desktop and mobile share state, validation, ordering, and actions. Only the
  profile editor and temporary picker composition differ.

## Public documentation

- Update `docs/public/agents-and-profiles.md` with dynamic profile creation,
  candidate behavior, and recovery actions.
- Update `docs/public/configuration.md` and `docs/public/feature-status.md` with
  the runtime flag, defaults, restart requirement, and experimental status.
- Update `docs/public/tasks-and-workflows.md` with dynamic workflow selection
  and one-logical-session behavior.

## Tests

- **Profile identity:** registry, reconciler, SQLite migration, DTO, CRUD, and
  dependency tests prove the virtual family survives boot and cannot launch.
- **Core routing:** table-driven engine tests cover ordering, error actions,
  binding derivation, provider/binding/model/profile circuit scope, generation
  claims, exclusive probe leases, waiting, actions, and recovery.
- **Conductor:** integration tests cover same-route resume, cross-route fresh
  ACP sessions, continuation, restart reconciliation, and stale frame fencing.
- **Callers:** orchestrator, workflow, utility, plugin task-launch, and Office
  tests prove callers submit one logical profile ID and receive concrete
  attribution behind the resolver.
- **Persistence:** real SQLite tests prove route/turn/utility attribution and
  preservation of all legacy Office routing rows.

## E2E Tests

- `apps/web/e2e/tests/settings/dynamic-agent-profile.spec.ts` covers desktop
  create/edit/reorder/dependency behavior.
- `apps/web/e2e/tests/settings/mobile-dynamic-agent-profile.spec.ts` covers the
  direct phone route, picker drawer, touch reorder actions, internal scrolling,
  safe-area clearance, and no document overflow.
- Add isolated `dynamic-routing` and `dynamic-routing-mobile` Playwright
  projects for specs that restart with `KANDEV_MOCK_PROVIDERS`. Keep settings
  specs in the normal desktop and mobile projects.
- Match these desktop files only in `dynamic-routing`:
  `tests/task/dynamic-agent-routing.spec.ts`,
  `tests/task/dynamic-agent-routing-rollout.spec.ts`,
  `tests/workflow/dynamic-agent-profile.spec.ts`,
  `tests/settings/dynamic-utility-profile.spec.ts`, and
  `tests/office/dynamic-agent-execution-profile.spec.ts`.
- Match these phone files only in `dynamic-routing-mobile`:
  `tests/task/mobile-dynamic-agent-routing.spec.ts` and
  `tests/office/mobile-dynamic-agent-execution-profile.spec.ts`. Exclude all
  seven files from the default Chromium and mobile projects.
- `apps/web/e2e/tests/task/dynamic-agent-routing.spec.ts` covers one-tab fallback,
  localized route rows, immutable badges, capability replacement, waiting,
  generation-fenced actions, and restart recovery with multi-provider mocks.
- `apps/web/e2e/tests/task/mobile-dynamic-agent-routing.spec.ts` proves the same
  chat outcome without composer loss on the Pixel 5 project.
- Utility, workflow, Office, and flag-matrix specs prove transparent selection,
  retained legacy rows, and fail-closed disabled behavior. Retire the five
  `office-routing-*` specs and `mobile-provider-routing-profiles.spec.ts` with
  the legacy UI.

## Verification Results

Design-package validation on 2026-08-15:

- 16 task files exist and the plan links each file once.
- Every `depends_on` ID resolves to a task in this package.
- Every task has explicit inputs and an output contract.
- Task dependencies are acyclic. The caller, documentation, and Office rollout
  gates complete this package.
- Package files contain no stale task names, conflict markers, trailing spaces,
  Unicode em dashes, or prohibited shorthand found by the documentation scan.

Implementation slice validated on 2026-08-15:

- Task 01 flag tests pass with production, development, and E2E defaults off.
- Tasks 02 and 03 profile-family, persistence, CRUD, dependency, and
  optimistic-update tests pass with the `fts5` build tag.
- Task 04 dynamic engine and SQLite route-state tests pass, including fixed
  ordering, semantic actions, binding fingerprints, circuit/probe contracts,
  generation claims, restart loading, and stale-writer rejection.
- Task 05 and Task 06 resolver, launch-boundary, logical-session, settled-turn
  route-action, turn-attribution, persistence, continuation, and fail-closed
  evidence changes compile and pass the focused orchestrator/backend suite.
  The rollout-blocker repair also makes route actions atomic and delivers
  durable continuation before supported mid-turn fallback. Full lifecycle
  ownership, event-subscription wiring, and restart reconciliation remain
  open.
- Task 07 utility routing passes its focused backend suite. Utility calls keep
  logical attribution, execute the concrete profile, use isolated invocation
  identities, and can advance only classified pre-result failures. Plugin-host
  utility wiring and full caller E2E coverage remain open.
- Task 08 and Task 09 settings, route state, recovery UI, picker filtering,
  typecheck, i18n, and focused frontend tests pass. Full immutable per-turn
  badge presentation and Playwright coverage remain open.
- Task 15 public documentation validation passes after documenting the flag,
  fixed-order behavior, recovery boundary, and restart requirement.
- Tasks 10 through 14 and 16 remain pending because Office handoff,
  observability migration, and dedicated E2E projects were not implemented in
  this slice.

The telemetry-routing and provider-error-policy plans remain separate and are
not implemented by this package. The remaining dynamic-routing work is
explicitly tracked in the task results below rather than represented as a
completed package.

## Implementation Waves And Parallel Candidates

Dynamic agent routing:

Wave 1:

- [x] [Task 01: Runtime rollout flag](task-01-runtime-rollout-flag.md)

Wave 2:

- [x] [Task 02: Virtual profile foundation](task-02-virtual-profile-foundation.md)

Wave 3:

- [x] [Task 03: Dynamic profile management](task-03-dynamic-profile-management.md)

Wave 4 (parallel candidates with user authorization):

- [ ] [Task 04: Core route engine](task-04-core-route-engine.md)
- [x] [Task 08: Dynamic profile settings](task-08-dynamic-profile-settings.md)

Wave 5 (parallel candidates with user authorization):

- [ ] [Task 05: ACP conductor](task-05-acp-conductor.md)
- [ ] [Task 12: Profile settings E2E](task-12-profile-settings-e2e.md)

Wave 6:

- [ ] [Task 06: Logical session integration](task-06-logical-session-integration.md)

Wave 7 (parallel candidates with user authorization):

- [ ] [Task 07: Utility profile integration](task-07-utility-profile-integration.md)
- [ ] [Task 09: Routed chat presentation](task-09-routed-chat-presentation.md)

Wave 8:

- [ ] [Task 10: Office routing handoff](task-10-office-routing-handoff.md)

Wave 9:

- [ ] [Task 11: Core routing observability](task-11-core-routing-observability.md)

Wave 10 (parallel candidates with user authorization):

- [ ] [Task 13: Routed session E2E](task-13-routed-session-e2e.md)
- [ ] [Task 15: Core routing public docs](task-15-core-routing-public-docs.md)

Wave 11:

- [ ] [Task 14: Caller selection E2E](task-14-caller-selection-e2e.md)
- [ ] [Task 16: Office rollout E2E](task-16-office-rollout-e2e.md)

Sequential execution remains the default. The wave labels expose only the two
disjoint frontend/backend or docs/test pairs. Completion of Wave 11 completes
this implementation package.
