---
status: draft
system: agents
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-ROLLOUT-BLOCKERS-001
created: 2026-08-13
updated: 2026-10-12
owners:
  - cfl
---
# Dynamic Agent Routing System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001` | [Migrated source detail](#migrated-source-detail) |
| `REQ-AGENTS-DYNAMIC-AGENT-ROUTING-ROLLOUT-BLOCKERS-001` | [Failure modes](#failure-modes) |

## Migrated source detail

## Observability

- Structured logs and expvars count route decisions, same-route retries,
  fallbacks, waiting sessions, circuit opens and closes, half-open probes,
  continuation creation, restart reconciliation, stale-generation drops, and
  utility routing outcomes.
- The shared router preserves `routing_route_attempts_total` and
  `routing_fallback_total`. It retires `routing_provider_degraded_total`,
  `routing_provider_recovered_total`, and `routing_parked_runs_total`. Their
  replacements are `routing_resource_circuit_opened_total`,
  `routing_resource_circuit_closed_total`, and
  `routing_waiting_sessions_total`. Structured log names use the same semantic
  mapping. `AGENTS.md` documents the new names.

## Test strategy

- Backend table-driven tests cover profile-kind resolution, fixed ordering,
  class-policy validation and evaluation, reset waits, exponential retry,
  exhausted outcomes, circuit sharing and scoping, generation fencing,
  continuation, restart reconciliation, utility calls, and the
  concrete/dynamic transparent resolver boundary.
- Repository integration tests cover dynamic configuration, logical route
  state, immutable turn and utility attribution, conflict details, and
  preservation of legacy Office routing rows.
- Desktop and mobile Playwright tests cover profile creation and ordering,
  workflow and utility selection, one-tab provider switching, localized route
  rows, immutable provider/model badges, capability replacement, waiting and
  recovery actions, and Office execution-profile selection. Phone editors use
  direct navigation and a single internal scroll owner. Temporary candidate
  choices use the existing inset picker drawer.

## State machine

| State | Trigger | Next state |
| --- | --- | --- |
| `selecting` | route generation and candidate committed | `starting` |
| `selecting` | no eligible candidate, circuit recovery exists | `waiting` |
| `selecting` | no candidate and user action required | `action_required` |
| `starting` | downstream ACP initialized and capabilities captured | `active` |
| `starting` | safe classified failure with reset wait | `waiting_for_reset` |
| `starting` | safe classified failure with retry budget | `retry_wait` |
| `starting` | recovery exhausted | `switching` or `action_required` |
| `active` | safe classified failure with reset wait | `waiting_for_reset` |
| `active` | safe classified failure with retry budget | `retry_wait` |
| `active` | recovery exhausted | `switching` or `action_required` |
| `active` | active candidate disabled or removed | `reroute_pending` |
| `active` | concrete-profile process restart | `starting` with same route |
| `reroute_pending` | active turn settles | `switching` |
| `switching` | successor decision committed | `starting` with generation + 1 |
| `switching` | no successor is eligible | `waiting` or `action_required` |
| `retry_wait` or `waiting_for_reset` | deadline due and generation current | `retrying` |
| `retrying` | candidate succeeds | `active` |
| `retrying` | safe classified failure | class policy evaluates again |
| `retry_wait` or `waiting_for_reset` | Skip now | `switching` |
| `retry_wait` or `waiting_for_reset` | Cancel wait or Stop | `action_required` or `stopped` |
| `waiting` | reset, probe success, or Retry now | `selecting` |
| `action_required` | settings or credentials change, or explicit retry | `selecting` |
| any nonterminal state | user stops session | `stopped` |

Only one transition owner can commit a new route generation. Candidate
selection is not an exclusive lease across sessions. Late ACP frames and user
actions are ignored when their generation no longer matches. Probe results also
require the current exclusive probe lease.

## Permissions

- Users who can manage agent profiles can create and edit dynamic profiles.
- Users who can select an agent profile can select a dynamic profile but cannot
  thereby view candidate credentials.
- Office agents cannot edit routing policy unless a separate existing profile
  management permission authorizes it. Assigning tasks does not grant that
  permission.

## Failure modes

- Before adding transcript-derived user rows to a new continuation or choosing
  a durable relaunch prompt, the orchestrator reads the complete session queue
  snapshot and, when supported, ordinary pending dispatch claims under the same
  session admission gate. A concurrent restore cannot move a claim into the
  queue between these ownership reads. Owned entries
  and same-session unaccepted claims exclude existing rows by their explicit
  transcript ID and the existing deterministic queued-message ID. The recorded
  marker may predate the writer in durable claims and is not required for an
  exact row-ID match. An absent transcript row has no context to exclude. Read errors
  stop reconstruction before launch. These reads do not ACK, restore, delete,
  or dispatch queued work. Saved continuation data and cached retry prompts
  keep their existing precedence.
- Invalid or deleted candidate references make that candidate ineligible and
  surface a profile configuration error. They never fall through to a workspace
  default profile.
- If route-state persistence fails, Kandev does not launch the successor. This
  prevents an invisible provider switch.
- If a provider fails after assistant or tool effects, the continuation package
  marks delivery as potentially effectful and requires the successor to inspect
  durable state before repeating work.
- If capability discovery for the new provider fails, the route does not become
  `active`. The UI retains the prior controls as disabled until another route is
  selected or the failure is resolved.
- If multiple tasks fail the same resource concurrently, the first committed
  circuit transition wins. Other failures extend evidence without starting
  duplicate probes.
- If every route is open, tasks wait under the same logical profile and expose
  the earliest known recovery time and remediation actions.
- If a route action carries a stale generation, Kandev does not change route
  state and returns the authoritative route snapshot.
- If a route action targets a session whose selected profile is not dynamic,
  the orchestrator returns a typed sentinel error and the WebSocket handler maps
  it to a validation error with that reason. No route or session state changes.
- A failure is fenced to the route generation and candidate that produced it.
  The route records a successor before the predecessor's process is replaced,
  and a deferred successor launch leaves the predecessor serving the session.
  The orchestrator therefore drops a failure whose execution ran a concrete
  profile other than the session's current execution profile, and the engine
  rejects a failure whose generation or candidate differs from the route state
  it holds or loads with a stale-generation error before it opens any circuit.
  Such a failure neither opens the current candidate's circuit nor advances the
  route. A session without known route state keeps the existing behavior.
- Lifecycle stream events capture the execution's concrete profile separately
  from its logical profile or Office identity. The stream-error handler passes
  that concrete profile to the same failure guard as terminal lifecycle events.
  Legacy stream events without a concrete profile retain their existing behavior.
- If an error is unclassified, Kandev enters manual recovery except for the
  [repeated-failure extension](dynamic-unclassified-fallback.md). Stale,
  conflicting, and effect-unsafe evidence never authorizes that extension.
- If an ACP diagnostic is followed by a matching high-confidence terminal
  provider error before any other output or tool activity, the transcript keeps
  the diagnostic but the shared recovery evidence treats the attempt as
  pre-result. A mismatch or later progress remains effect-unsafe and enters
  manual recovery.
- If a trusted reset is beyond the configured maximum wait, Kandev does not
  wait for or shorten it. It proceeds to retry or the exhausted outcome.
- If dynamic routing is disabled after a session is persisted, Kandev keeps its
  route state and does not launch or resume downstream ACP until the feature is
  enabled again.
- If a resumed or prepared runtime settles to WAITING_FOR_INPUT before the
  process-start callback runs, the callback still marks the claimed route
  generation active for that execution, and the resume-ready path does the same
  for a route it re-claimed as `starting` or `retrying`. A route left
  `starting` would make every later explicit recovery report
  `dynamic_route_pending`.
- A resume-ready durable activation failure returns to the actual resume caller
  without stopping or replacing the healthy runtime. A later resume admission
  may reconcile the same starting generation after storage recovery, provided
  the live prompt-ready execution matches the session's claimed execution and
  the session is not cancelled or terminal. The generation/status-fenced write
  remains authoritative; a predecessor runtime cannot activate a successor.
- If an explicit recovery action finds the route `starting` and no runtime is
  live for the session, it re-claims the same persisted candidate and resumes.
  A live launch owner, a passive resume, and other non-active states keep the
  `dynamic_route_pending` guard. An explicit recovery that is still blocked
  returns that error to its caller instead of a success response with no
  execution; a passive session open keeps its waiting response.
- At startup the orphan-route sweep treats a session parked in
  WAITING_FOR_INPUT whose route is still `starting` and which has no
  `executors_running` row as a stranded launch owner, and marks it
  `action_required`: no launch reached `MarkActive`, the ordinary session UI
  cannot release the route while no runtime backs it, and no other pass
  recovers it. A live executor row, an unreadable row, a stale generation, and
  any other parked state keep the existing exclusion.
- If the backend restarts during `switching`, it reconciles the persisted route
  generation and downstream ownership before launching. An ambiguous launch is
  not duplicated.

## Persistence guarantees

- Dynamic profile configuration and reference protection survive backend
  restart and workspace reload.
- Logical task-session identity, route generation, active concrete profile,
  provider-native session ownership, classification and policy snapshots,
  retry ordinals, waiting deadlines, pending outcomes, and shared circuits are
  durable.
- Provider-native resume data is usable only with the concrete profile that
  created it.
- A backend restart resumes the same downstream session when supported and safe,
  otherwise it starts a fresh session on the same route with a continuation
  package. It does not silently re-run route selection unless the persisted
  route is no longer eligible.
- Route attempt history is append-only for audit. Profile edits affect future
  selections and do not rewrite recorded decisions.
- Turn-level execution profile, provider, model, and route-generation badges are
  immutable and survive reload even when the current route or profile changes.

## Scenarios

- **GIVEN** a Kanban task assigned to a dynamic Frontier profile with Fable then
  Codex, **WHEN** Fable reports a high-confidence quota failure, **THEN** the
  same task tab shows a route-change row, starts a fresh Codex ACP session with a
  continuation package, and replaces the model/options/commands controls.
- **GIVEN** an Office CEO that selects the same dynamic profile, **WHEN** the
  same failure occurs, **THEN** the shared router performs the same transition
  while the CEO identity, instructions, skills, permissions, budget, task, and
  worktree remain unchanged.
- **GIVEN** a task, workflow, Office agent, or utility caller selects a dynamic
  profile ID, **WHEN** it starts execution, **THEN** the caller uses its normal
  profile path and the shared resolver starts the conductor without requiring a
  concrete execution profile from that caller.
- **GIVEN** a utility agent selects a dynamic profile, **WHEN** its first
  candidate fails with a classified quota error before returning a result,
  **THEN** the invocation tries the next configured candidate and records both
  the logical and successful concrete profile IDs without creating a task tab.
- **GIVEN** two tasks use profiles with the same proven Fable credential
  binding, **WHEN** one opens its quota circuit, **THEN** the second skips Fable
  without making a failing launch.
- **GIVEN** another profile uses a different or unknown Fable credential
  binding, **WHEN** its resource key is evaluated, **THEN** the first binding's
  quota circuit does not disable it.
- **GIVEN** a healthy session on Fable, **WHEN** its dynamic profile is reordered
  or receives an error-action edit, **THEN** it stays on Fable until the next
  route decision and that decision uses the new profile version.
- **GIVEN** the active Fable candidate is removed or disabled, **WHEN** the
  current turn finishes, **THEN** the session reroutes at that safe boundary even
  when no provider error occurred.
- **GIVEN** a Dynamic Frontier chat switches from Fable to Codex, **WHEN** the
  user reloads it, **THEN** the tab and message authors still show Dynamic
  Frontier, each assistant turn shows its original provider/model badge, the
  durable switch row remains in sequence, and controls describe Codex.
- **GIVEN** an open route reaches its retry time while many tasks are waiting,
  **WHEN** recovery begins, **THEN** one half-open probe runs and the other tasks
  do not stampede the provider.
- **GIVEN** an earlier candidate recovers while a task is healthy on fallback,
  **WHEN** the circuit closes, **THEN** that task stays on fallback and future
  route selections use the configured candidate order.
- **GIVEN** the backend restarts with a dynamic session active, **WHEN** runtime
  reconciliation completes, **THEN** the same Kandev session tab returns and the
  backend resumes only the provider-native session owned by the persisted
  concrete profile.
- **GIVEN** an existing database contains legacy Office workspace routing rows,
  **WHEN** it upgrades to dynamic routing, **THEN** those rows remain unchanged
  but no Office settings or launch path reads or displays them.
- **GIVEN** a waiting route at generation 7, **WHEN** the user sends Skip now
  for generation 7, **THEN** Kandev excludes the current candidate for one
  decision and returns the resulting authoritative route snapshot.
- **GIVEN** a route has advanced to generation 8, **WHEN** a delayed Retry now
  for generation 7 arrives, **THEN** Kandev rejects it and returns generation
  8 without changing route state.
- **GIVEN** a transient policy allows three retries starting at five seconds,
  **WHEN** the candidate keeps returning an effect-safe capacity error,
  **THEN** the route persists 5, 10, and 20 second waits before applying its
  configured skip or stop outcome.
- **GIVEN** the active candidate emits `Repeated 529 Overloaded errors` as an
  ACP diagnostic and immediately settles with a matching structured provider
  error, **WHEN** no assistant output or tool activity follows and the
  transient policy is exhausted with `skip`, **THEN** the same logical dynamic
  session advances to the next eligible candidate. **GIVEN** later output,
  tool activity, or a mismatched terminal error, **THEN** it does not switch.
- **GIVEN** a hard quota error includes a trusted reset in one minute, **WHEN**
  the policy permits reset waits up to five minutes, **THEN** the route waits
  durably and retries the same candidate after the reset.
- **GIVEN** a new dynamic profile is being created, **WHEN** its editor opens,
  **THEN** it contains one draft profile and no Add profile action.
- **GIVEN** Office is enabled and dynamic routing is disabled, **WHEN** one
  Office identity selects a concrete profile and another selects a dynamic
  profile, **THEN** the concrete identity can launch and the dynamic identity
  enters an actionable feature-disabled state.
- **GIVEN** a phone viewport, **WHEN** the user creates or inspects a dynamic
  profile, **THEN** candidates and class policies use a single-column touch
  layout. Routing status and recovery actions remain available without
  horizontal page overflow.

## Out of scope

- Lossless transfer of hidden provider-native conversation state.
- Nested dynamic profiles or arbitrary routing graphs in the first version.
- Session-cost polling, subscription-usage polling, telemetry-backed routing,
  cost preference, allowance thresholds, and `interrupt_turn`. These behaviors
  belong to the deferred Dynamic Agent Telemetry Routing feature, which has no
  active specification.
- Automatic purchase, plan upgrade, or credential refresh.
- Model-based classification or extraction of provider error details.
- Moving a healthy active task back to an earlier candidate immediately after
  recovery.
- Letting workflows or Office workspaces duplicate provider order, error
  actions, or health policy outside a dynamic profile.
- Automatically converting legacy Office routing rows into dynamic profiles or
  deleting those rows during this feature's rollout.

Activation reconciliation also verifies the execution record's concrete profile against the current route candidate. Matching the session's execution ID alone is insufficient while a detached successor has been recorded but its predecessor still runs. The shared cancellation/settlement guard serializes observation and activation with successor launch, and a guarded session read revalidates the observed generation, candidate, execution, and nonterminal state.

After durable activation, reconciliation reads back the same-generation active route and the persisted session projection. A failed projection mirror keeps admission guarded until a later attempt repairs it; the admitted session copies the persisted route state, reason, and timestamp. Candidate, execution, and nonterminal session checks remain under the shared cancellation guard.
