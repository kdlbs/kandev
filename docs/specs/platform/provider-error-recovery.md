---
status: draft
created: 2026-08-08
owner: Kandev
---

# Provider Error Recovery

Decisions:

- [ADR-2026-08-08-provider-neutral-agent-error-recovery](../../decisions/2026-08-08-provider-neutral-agent-error-recovery.md)
- [ADR-2026-07-29-agent-stall-user-controlled-recovery](../../decisions/2026-07-29-agent-stall-user-controlled-recovery.md)

## Why

Codex, Claude, OpenCode, and future agent CLIs report equivalent provider
failures through different ACP frames and diagnostic streams. Users need
temporary capacity failures to recover with honest progress. Kanban sessions
need conservative interactive retry, while unattended Office runs need durable
configured fallback and scheduler recovery. Both must classify the provider
failure consistently without sharing the same retry policy.

## What

### Provider-neutral classification

- Kandev correlates supported error evidence with the active agent session,
  foreground prompt, and prompt generation before it can settle a turn.
- Evidence may come from an ACP prompt error, ordered ACP updates and completion,
  managed structured stderr, a process exit, or structured HTTP metadata.
- Agent-specific evidence adapters produce one bounded, sanitized evidence
  envelope. Orchestration, lifecycle, and UI code do not inspect an agent name or
  raw provider message to choose recovery behavior.
- A shared classifier assigns only a stable semantic code, confidence, scope,
  classifier rule ID, and validated timing hints. It does not assign a global
  retry or fallback verdict.
- The classifier distinguishes temporary model capacity from an invalid or
  unavailable model, and short rate throttling from subscription or plan quota
  exhaustion.
- An explicit execution context selects either Kanban interactive recovery or
  Office unattended routing. Only high-confidence, current-prompt evidence can
  authorize Kanban automatic retry or Office post-start fallback. Inactivity
  alone never classifies a provider failure.
- Classifier rules are deterministic and fixture-driven. Adding another known
  error example normally adds a sanitized fixture and registry rule rather than
  an orchestrator, UI, or agent-type branch.

### Kanban interactive recovery

- `network_unavailable`, `provider_overloaded`, `provider_unavailable`,
  `model_capacity`, and confirmed short `rate_limited` failures are eligible for
  automatic retry.
- Before replaying, Kandev verifies that the prompt has produced no assistant
  content or tool activity, unless the adapter provides a resumable retry
  guarantee. A transient failure after potentially effectful progress remains a
  manual recovery.
- An automatic retry preserves the original prompt, selected agent and execution
  profiles, model, permissions, and turn configuration.
- Kandev makes at most five automatic attempts. Nominal backoff is 5, 10, 20,
  40, and 60 seconds with bounded jitter. A trustworthy short provider retry
  hint can lengthen the next delay.
- Kandev never silently changes the selected model, account, agent, or provider,
  and never purchases capacity.
- Exactly one backend schedule owns retry for a session and prompt generation.
  A successful attempt clears recovery state. Exhaustion transitions to the
  existing manual Resume and Start fresh choices.
- Authentication, credentials, subscription or billing action, plan quota, invalid model,
  disabled model, and provider-configuration failures never schedule a Kanban
  automatic retry.
- A known quota reset time or remediation destination is displayed, but does not
  arm a Kanban timer.
- A permission denial, task error, or repository error follows its existing
  terminal recovery behavior.
- An unknown error fails closed to generic manual recovery. Raw unclassified
  evidence is not presented as trusted provider guidance.

### Office unattended routing

- Office consumes the shared code through its own routing policy. It never uses
  Kanban's short prompt timer or retry-attempt budget.
- The short same-route phase applies to every Office run, even when provider
  fallback is disabled. Fallback after exhaustion still requires an enabled,
  explicitly configured alternative.
- Network interruption, temporary provider unavailability/overload, model
  capacity, and a validated rate-limit wait of at most 60 seconds first retry the
  same execution profile after nominal 5, 10, and 20 second delays with jitter.
- During that short phase the failed provider is not degraded or excluded from
  the run's route cycle. Three failed retries promote it to a degraded route;
  Office may then fall through to the next configured execution profile.
- The retry attempt has one owning run, while the short cooldown is scoped to the
  affected workspace/provider/model. Concurrent runs wait for that cooldown and
  do not independently retry or switch providers. Success releases them;
  exhaustion promotes the route to normal degradation/fallback.
- A rate-limit wait longer than 60 seconds, quota exhaustion, auth/subscription,
  and model/configuration errors bypass the short phase and immediately use the
  configured fallback policy.
- A route with auth, inactive-subscription, missing-configuration, or invalid
  model evidence remains `user_action_required`; Office may try another route,
  but does not schedule the blocked route itself.
- Renewable quota exhaustion bypasses the same-route short phase, falls through
  to the next configured route, and leaves the affected route in durable
  provider-health recovery using a validated reset hint or the minute-scale
  backoff when no reset is known.
- Short rate limits, overload, and availability failures carry durable
  provider-health retry state only after the short same-route phase is
  exhausted. A validated reset hint wins; otherwise Office uses its
  minute-scale 2, 5, 10, 20, and 60-minute backoff with jitter.
- When all routes are exhausted, Office durably parks the run as waiting for
  provider capacity or blocked for provider action. The scheduler wakes waiting
  runs, while blocked runs surface settings/inbox remediation.
- Non-provider run failures retain Office's separate generic scheduler retry
  budget and CEO escalation. They are not reclassified as provider capacity.
- A pre-progress short retry may re-drive the exact prompt. After assistant or
  tool progress, Office retries through a fresh same-provider session that first
  inspects durable task and worktree state. Ambiguous prompt delivery is never
  automatically repeated.

### Workspace-specific progress

- A scheduled Kanban retry appears as one localized inline warning in the task
  chat. It shows a safe provider/model label when available, the next retry ordinal,
  maximum attempts, and a live `Retrying in Ns` countdown.
- The backend sends the exact absolute `retry_at`; the browser derives the
  countdown locally and does not write a new message every second.
- When the timer fires, the same surface changes to `Retrying now`. If another
  transient failure occurs, it advances to the next attempt and new `retry_at`.
- The user can cancel the schedule. Sending another prompt, changing model or
  profile, stopping the session, or starting fresh also cancels the old retry
  before the new action proceeds.
- The UI announces scheduled, retrying, cancelled, recovered, and exhausted
  transitions accessibly, but does not announce every countdown tick.
- Desktop and phone surfaces expose the same status and actions. Phone controls
  retain at least a 44px touch target without horizontal page overflow.
- Office task/run detail, agent detail, dashboard, and inbox show route attempts,
  actual provider/model, fallback reason, parked status, and next durable wake.
  During short recovery they explicitly say the same provider will retry and
  show its countdown and ordinal. Actions are Retry now, Try next provider now
  when configured, or provider remediation.

## Data model

The normalized internal classification contains:

| Field | Meaning |
| --- | --- |
| `code` | Stable semantic cause such as `model_capacity` or `quota_limited` |
| `confidence` | `high`, `medium`, or `low` classifier confidence |
| `scope` | Provider, account, model, or request scope when known |
| `classifier_rule` | Stable rule ID for tests, metrics, and diagnostics |
| `provider_id` / `model_id` | Safe identifiers when present |
| `occurred_at` | Time of the correlated failure |
| `retry_after` / `reset_at` | Optional validated short retry hint or quota reset hint |
| `safe_excerpt` | Bounded sanitized evidence for collapsed technical details |

Each consumer adds its own policy result. Existing compatibility fields such as
`AutoRetryable`, `FallbackAllowed`, and `UserAction` become Office-policy
projections rather than classifier-owned universal truths. Kanban stores its
interactive disposition separately.

Persisted retry status contains:

| Field | Meaning |
| --- | --- |
| `prompt_generation` | Turn identity that owns the retry |
| `failure_code` | Stable semantic cause |
| `interactive_disposition` | Kanban policy result |
| `retry_state` | `scheduled`, `dispatching`, `cancelled`, `recovered`, or `exhausted` |
| `retry_attempt` | Upcoming or active automatic retry ordinal; original prompt is not counted |
| `max_attempts` | Central attempt budget used by this schedule |
| `retry_at` | Exact UTC time for a scheduled retry |
| `provider_id` / `model_id` | Optional safe display labels |

The status is Kanban-session scoped. Office keeps its existing durable
`office_provider_health`, `office_run_route_attempts`, and run routing fields.
Raw agent streams and unsanitized provider errors are not added to either model.

Office extends its run-routing overlay with a durable short-retry phase,
same-route retry ordinal, and absolute retry deadline. A transient attempt uses
an outcome distinct from fallback exclusion; only short-budget exhaustion marks
that provider failed for the route cycle.

Office provider health adds a non-degraded `short_retry` cooldown carrying the
affected scope and deadline. Durable ownership is represented by the run's
`current_route_attempt_seq` pointing at its `retry_scheduled` route-attempt row;
the short-retry ordinal is the count of those rows after the run's cycle
baseline. The run's `scheduled_retry_at` mirrors the cooldown deadline. This
mapping coordinates concurrent runs without making the provider eligible for
fallback before the short budget ends, and survives restart without adding an
owner column to provider health.

## API surface

- Existing session state and message payloads carry the structured retry status.
  `retry_at` is an absolute UTC timestamp, never pre-rendered countdown text.
- The existing `session.recover` request accepts `cancel_retry` for the owning
  session. Cancellation is idempotent.
- The backend publishes status changes when a retry is scheduled, begins,
  succeeds, is cancelled, or exhausts its budget. One-second countdown ticks are
  frontend-only presentation.
- Office routing continues to expose its durable route-health and route-attempt
  events. Its policy derives fallback, health state, and scheduler eligibility
  from the shared classification plus Office context.
- Office exposes a workspace-admin run action to skip a pending short wait and
  advance to the next configured provider. It carries the expected route-attempt
  sequence so a timer race cannot advance twice.

## Kanban state machine

| State | Trigger | Next state |
| --- | --- | --- |
| `classified` | High-confidence transient and replay-safe | `scheduled` |
| `classified` | User action, terminal, unknown, or replay-unsafe | manual recovery |
| `scheduled` | `retry_at` reached and generation still current | `dispatching` |
| `scheduled` | Cancel, new prompt, configuration change, stop, or stale generation | `cancelled` |
| `dispatching` | Prompt succeeds | `recovered` |
| `dispatching` | Another eligible transient and budget remains | `scheduled` |
| `dispatching` | Hard/unknown failure or budget exhausted | manual recovery / `exhausted` |

Only the backend advances this state machine. The frontend renders it and sends
explicit user actions. Office retains the provider-route and task-resolution
state machines in [Office Provider Routing](../office/routing.md).

## Office short-retry state machine

| State | Trigger | Next state |
| --- | --- | --- |
| `route_active` | High-confidence short transient | `same_route_short_retry` |
| `same_route_short_retry` | `retry_at` reached, attempt remains | retry same execution profile |
| `same_route_short_retry` | Retry succeeds | `route_active` |
| `same_route_short_retry` | Third retry fails | degrade route, then enabled configured fallback or parked capacity |
| `route_active` | Long-horizon or user-action provider failure | configured fallback or blocked/parked |
| `same_route_short_retry` | User selects Try next provider now | degrade route and configured fallback |

Runs that encounter an existing route-scoped `short_retry` cooldown wait on its
deadline without consuming another retry attempt or considering fallback.

## Failure modes

- If a Codex-style failure is split across ACP metadata, a message, and
  `end_turn`, only the correctly ordered, current-prompt sequence settles as an
  error. A partial or stale sequence is ignored as authoritative evidence.
- If a classifier rule does not recognize a provider message, Kanban does not
  automatically retry it and Office does not perform a post-start fallback. Its
  bounded sanitized evidence can inform a later rule fixture.
- If two rules match, deterministic specificity and priority select one result;
  registry collision tests prevent accidental broad-rule overrides.
- If a new semantic code lacks either a Kanban or Office policy entry, exhaustive
  policy validation fails; no default may silently authorize retry or fallback.
- If a transient failure follows assistant output or tool activity and no safe
  resume guarantee exists, Kandev does not replay it automatically.
- If a timer fires after a new prompt generation or session replacement, the
  generation guard drops it before dispatch.
- If cancellation races the timer, one backend owner decides the outcome. A
  cancelled schedule cannot dispatch afterward.
- If an Office same-route timer races a fallback request, the scheduler commits
  exactly one transition; the original provider cannot launch after fallback
  has selected a successor.
- If persistence of the visible status fails, Kandev cancels the schedule rather
  than running an invisible automatic retry.
- If the backend restarts with a `scheduled` retry, it re-arms only after proving
  the exact prompt/configuration is reconstructable. An ambiguous `dispatching`
  retry becomes manual recovery to avoid duplicate delivery.
- If a browser clock is skewed, reaching zero changes only presentation; the
  backend still owns actual dispatch time and the next state update corrects the
  display.

## Persistence guarantees

- Classification and retry status survive browser reloads through the existing
  session message/state persistence.
- The frontend reconstructs a live countdown from persisted `retry_at` and the
  current clock.
- Prompt generation and retry ordinal are durable enough to reject stale timers
  and ACP frames.
- A backend restart never claims an in-memory timer still exists. It either
  safely re-arms a provably undispatched durable schedule or changes the surface
  to manual recovery.
- Sanitized classifier evidence follows existing diagnostic retention rules; raw
  ACP/stderr streams do not become durable retry metadata.
- Office provider health, route attempts, parked-run status, and scheduler
  deadlines retain their existing database-backed restart guarantees.
- Office's same-route retry phase and deadline are equally durable. Restart only
  re-arms an attempt known not to have reached dispatch.

## Scenarios

- **GIVEN** the active Luna/Codex prompt emits `systemError`, a selected-model
  capacity message, and `end_turn`, **WHEN** the ordered evidence is correlated,
  **THEN** Kandev classifies `model_capacity` and schedules the first safe retry.
- **GIVEN** Claude reports a structured 529 overload before producing content or
  tool activity, **WHEN** it is classified, **THEN** the same retry lifecycle and
  UI apply without a Claude-specific orchestration branch.
- **GIVEN** OpenCode emits a correlated temporary provider-unavailable diagnostic,
  **WHEN** its adapter normalizes the evidence for a Kanban session, **THEN** the
  Kanban policy can schedule the same short recovery.
- **GIVEN** any Kanban agent reports a subscription or plan usage limit, **WHEN**
  it is classified, **THEN** Kandev shows the safe explanation and
  reset/remediation hint but schedules no automatic retry.
- **GIVEN** an Office agent hits a renewable provider quota, **WHEN** automatic
  routing is enabled, **THEN** Office marks that route degraded and immediately
  tries the next configured execution profile.
- **GIVEN** an Office agent encounters a network reset or selected-model capacity
  failure before producing work, **WHEN** routing is enabled, **THEN** Office
  retries the same execution profile after a visible few-second countdown and
  does not select a fallback provider.
- **GIVEN** that same short transient persists through three Office retries,
  **WHEN** the final short attempt fails, **THEN** Office degrades the route and
  tries the next configured execution profile.
- **GIVEN** Office fallback is disabled, **WHEN** the same short budget is
  exhausted, **THEN** the run enters durable provider-capacity recovery and
  never selects another provider.
- **GIVEN** several Office runs encounter the same model-capacity cooldown,
  **WHEN** one run owns the short retry, **THEN** the other runs wait on that
  deadline without consuming attempts, switching providers, or launching a
  retry herd.
- **GIVEN** an Office rate limit carries a validated 10-minute retry hint,
  **WHEN** another configured provider is available, **THEN** Office bypasses
  the short phase and falls through immediately.
- **GIVEN** every Office route is quota-limited, **WHEN** the configured chain is
  exhausted, **THEN** the run is durably parked and wakes at the earliest
  validated reset or Office provider-health backoff.
- **GIVEN** an Office route requires an inactive subscription, **WHEN** another
  configured route is available, **THEN** Office falls through to it while the
  failed route remains blocked for user action and receives no timed retry.
- **GIVEN** an Office run is parked for provider capacity, **WHEN** the user opens
  its task or dashboard on a phone, **THEN** the actual route, fallback reason,
  next durable wake, and Retry now or remediation action remain available with
  touch targets of at least 44px and no horizontal overflow.
- **GIVEN** an Office same-provider retry is pending, **WHEN** the user views the
  run on a phone, **THEN** the provider, attempt ordinal, live countdown, Retry
  now, and Try next provider now remain visible without a new drawer or
  horizontal overflow.
- **GIVEN** a new provider error example maps to an existing semantic cause,
  **WHEN** a developer adds its sanitized fixture and registry rule, **THEN** all
  recovery consumers gain the behavior without lifecycle or UI changes.
- **GIVEN** an automatic retry is scheduled, **WHEN** the user reloads the task,
  **THEN** the same attempt and live countdown render from the absolute
  `retry_at`.
- **GIVEN** the countdown is visible on a phone, **WHEN** the user taps Cancel,
  **THEN** the backend cancels the schedule through a touch target of at least
  44px and no later retry dispatches.
- **GIVEN** a transient failure occurred after a tool call, **WHEN** no resumable
  retry guarantee exists, **THEN** Kandev offers manual recovery instead of
  replaying the prompt.
- **GIVEN** five automatic retries all fail transiently, **WHEN** the budget is
  exhausted, **THEN** the countdown stops and localized Resume and Start fresh
  recovery actions appear.

## Out of scope

- Inferring failure from inactivity alone.
- Automatically buying capacity, upgrading a subscription, authenticating an
  account, or changing provider configuration.
- Silently changing the selected agent, profile, model, account, or provider for
  an interactive task session.
- Automatically scheduling a Kanban prompt at a distant subscription or quota
  reset time.
- Treating arbitrary raw stderr or model-authored text as trusted error evidence.
- Unifying Kanban and Office retry state machines, schedules, or UI surfaces.
