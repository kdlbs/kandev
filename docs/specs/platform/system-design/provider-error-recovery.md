---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
created: 2026-08-08
updated: 2026-08-17
owners:
  - Kandev
---
# Provider Error Recovery System Design

## Purpose and boundaries

This design preserves the technical source detail for `REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

Decisions:

- [ADR-2026-08-08-provider-neutral-agent-error-recovery](../../../decisions/2026-08-08-provider-neutral-agent-error-recovery.md)
- [ADR-2026-08-17-provider-error-classes-and-policies](../../../decisions/2026-08-17-provider-error-classes-and-policies.md)
- [ADR-2026-08-13-dynamic-agent-profile-routing](../../../decisions/2026-08-13-dynamic-agent-profile-routing.md)

Implementation plan:
[Provider Error Policies](../../../plans/provider-error-policies/plan.md).

Dynamic profile configuration and route continuity are specified in
[Dynamic Agent Routing](../../agents/requirements/dynamic-agent-routing.md). This specification
owns the provider-neutral error catalogue, policy schema, and recovery
semantics used by concrete profiles, dynamic profiles, Kanban, utility calls,
and Office.

## Why

Agent CLIs and providers report equivalent failures through different ACP
frames, HTTP metadata, process exits, and diagnostic strings. Capacity,
network, subscription, and quota failures need different recovery behavior,
but orchestration code must not branch on provider names or raw prose.

Users also need explicit control over how a dynamic candidate reacts. A short
outage may justify a few retries, while an exhausted subscription may justify
waiting for a near reset, skipping to the next candidate, or stopping for
manual action. The same classification and policy semantics must apply in
Kanban and Office so a selected dynamic profile does not change meaning between
workspace modes.

## What

### Evidence and deterministic classification

- Agent adapters collect bounded evidence from structured ACP errors, ordered
  ACP updates, managed stderr, process exits, and structured HTTP metadata.
- Evidence is correlated to the active invocation, prompt generation, and
  lifecycle phase before it can authorize recovery.
- A shared deterministic classifier maps evidence to a stable semantic code,
  policy class, confidence, scope, classifier rule ID, and validated timing
  hints.
- Provider-specific signatures live in adapter extractors and a central
  fixture-driven catalogue. Kanban, Office, lifecycle, and UI code do not
  inspect provider names or raw error strings to choose behavior.
- Structured metadata and exact signatures outrank broad text patterns.
  Collision tests enforce deterministic priority.
- Adding a known provider message normally adds a sanitized fixture and
  catalogue rule. It does not add an orchestration or UI branch.
- Classification is deterministic in this version. Calling a model to classify
  an error or extract timing data is deferred.

### Error classes

The policy layer has two configurable classes:

| Class | Meaning | Initial semantic codes and examples |
| --- | --- | --- |
| `transient` | The provider, network, transport, or selected model is temporarily unable to serve the request. | `network_unavailable`, `provider_unavailable`, `provider_overloaded`, `model_capacity`, confirmed short `rate_limited`, and launch-safe `agent_transport_lost` |
| `hard` | The selected account, subscription, credentials, provider configuration, or model cannot continue without a longer reset or user/configuration change. | `quota_limited`, `subscription_required`, `auth_required`, `missing_credentials`, `provider_not_configured`, and `model_unavailable` |

The semantic code remains authoritative diagnostic detail. The class is the
stable policy input. A catalogue revision can add new codes and signatures,
but it must assign every recoverable code to exactly one class.

Task, repository, permission, local runtime, cancellation, and resume-state
errors are not provider errors. They retain their existing owner and cannot
trigger candidate switching through this policy.

Unrecognized, low-confidence, stale, or conflicting evidence has classification
state `unclassified`. It is not a third configurable class. It stops automatic
recovery and surfaces manual recovery so an unknown string cannot silently
repeat work or change providers. Historical attempts retain the semantic code,
class, rule ID, and catalogue version assigned when the attempt occurred.

### Replay and effect-safety gate

Classification does not by itself authorize retry or switching.

- Automatic retry, reset waiting, or fallback requires evidence tied to the
  current invocation and a failure boundary that is known to be pre-result and
  effect-safe.
- A provider-supported resumable retry guarantee can satisfy this gate when it
  identifies the same provider-native session and generation.
- Assistant output, tool activity, partial utility output, ambiguous prompt
  delivery, or stale event ordering fails closed unless a durable continuation
  package makes successor delivery safe under the dynamic-routing contract.
- User configuration cannot override this gate. An unsafe transient or hard
  failure stops for manual recovery even when its class policy requests retry
  or skip.

### Per-class policy

Each dynamic candidate stores one policy for `transient` and one for `hard`.
Each policy contains:

| Field | Meaning |
| --- | --- |
| `retry.enabled` | Whether Kandev retries the same candidate before the final outcome |
| `retry.max_retries` | Maximum additional attempts after the failed attempt |
| `retry.initial_interval_seconds` | Delay before the first retry; later delays double exponentially |
| `wait_for_reset.enabled` | Whether a trustworthy future reset or retry time can replace the next retry delay |
| `wait_for_reset.max_wait_seconds` | Longest future reset interval this policy will wait |
| `on_exhausted` | `skip` to evaluate the next candidate, or `stop` for manual recovery |

This shape makes retry, reset waiting, skip, and stop explicit without leaving
retry exhaustion undefined. Disabling retry and reset waiting applies
`on_exhausted` immediately.

Backend limits are part of the contract: enabled retry accepts 1 through 10
additional attempts, the initial interval accepts 1 second through 1 hour, and
each computed retry delay is capped at 24 hours. Enabled reset waiting accepts
1 second through 7 days. Disabled sections store zero values. The frontend
uses the same limits, while the backend remains authoritative.

Policy evaluation follows this order:

1. Reject stale, unclassified, or effect-unsafe failures.
2. If reset waiting is enabled, has not already been used for this candidate
   and class in the current route cycle, and a validated future `retry_after`
   or `reset_at` is no later than `max_wait_seconds`, persist a wait for that
   deadline.
3. Otherwise, if retries remain, persist a retry of the same candidate. The
   first delay is `initial_interval_seconds`; each later delay doubles from the
   initial interval. Backend safety limits reject overflow and unbounded
   schedules.
4. When the retry budget is exhausted or no wait applies, execute
   `on_exhausted`: exclude the candidate for this route cycle when `skip`, or
   enter manual recovery when `stop`.

Waiting and its one post-wait attempt do not consume the exponential retry
budget. A candidate and class can use reset waiting at most once per route
cycle, so a repeated or revised hint cannot create an indefinite wait loop. A
reset at or before the current time is ignored. A reset beyond the configured
maximum is not shortened; Kandev proceeds to retry or the exhausted outcome.
Only validated structured hints or catalogue extractors may set timing
metadata.

New and legacy candidates default to immediate `skip` for both classes: retry
and reset waiting disabled, `on_exhausted=skip`. This preserves the current
generic `try_next` behavior and avoids introducing hidden delays. Existing
`retry_same`, `try_next`, `stop`, and per-code rules are normalized once into
the versioned class-policy document:

- `try_next` becomes no wait, no retry, then `skip` for both classes.
- `stop` becomes no wait, no retry, then `stop` for both classes.
- `retry_same` becomes one same-candidate retry after 5 seconds, then `stop`,
  unless an existing explicit per-code rule overrides the mapped class.

The backend returns the normalized document after create or update. The UI does
not write legacy rule shapes.

### Durable scheduling and route ownership

- The backend owns retry and reset timers. Browsers render absolute deadlines
  and never dispatch retries from countdown completion.
- A dynamic route persists its candidate, generation, failure code, class,
  catalogue version, policy snapshot, retry ordinal, deadline, and pending
  outcome before work is scheduled.
- Retry and reset waits use the same generation fencing and single transition
  owner as candidate switching. A timer, manual action, and provider event
  cannot advance the route twice.
- Restart reconciliation re-arms only a provably undispatched schedule. An
  ambiguous dispatch stops for manual recovery.
- User actions can retry now, skip now, cancel a pending wait, or stop. Each
  action carries the expected generation and returns the authoritative route
  snapshot.
- Shared resource circuits remain an eligibility input. A per-candidate policy
  controls the current route response; circuit state prevents concurrent
  sessions from creating a retry herd. Expired circuits recover through one
  exclusive probe.

### Use across Kanban, utility calls, and Office

- A dynamic profile has one policy document regardless of caller. Kanban,
  workflows, utility calls, and Office pass the same classified error and
  policy snapshot to the shared evaluator.
- Callers do not copy provider error tables or retry schedules. They provide
  invocation identity, effect evidence, and presentation context.
- Concrete-profile interactive recovery can retain its product-specific
  defaults, but it consumes the same class and timing metadata rather than a
  separate provider allow-list.
- Utility calls use a unique routing invocation ID and may recover only before
  any partial result or effect.
- Office scheduler wake reasons do not modify candidate error policy. Office
  reads the dynamic route state and presents the same retry, wait, skip, and
  stop outcome.

### User interface

- Every dynamic candidate exposes separate Transient errors and Hard errors
  sections.
- Each section explains the class with concrete examples in visible text. The
  primary behavior is not hidden in a hover-only tooltip.
- Users can enable same-candidate retry, set maximum retries, set the initial
  retry interval, enable reset waiting, set the maximum wait, and choose Skip
  candidate or Stop after recovery is exhausted.
- The form shows the exponential schedule implied by the current values and
  explains that reset waiting applies only to trusted future dates within the
  maximum.
- Validation is inline and mirrored by the backend. Counts and durations must
  be finite, non-negative, and within backend-owned limits.
- Desktop uses an expandable policy area inside each candidate row. Phone uses
  the same direct settings route and a single-column disclosure layout with one
  page scroll owner, 44px controls, and no horizontal document overflow.
- Active route surfaces show the class, safe cause, retry ordinal, absolute
  deadline, and next outcome. They do not expose unsanitized provider text.

## Data model

The normalized classification contains:

| Field | Meaning |
| --- | --- |
| `code` | Stable semantic cause |
| `class` | `transient`, `hard`, or internal `unclassified` state |
| `catalogue_version` | Deterministic catalogue version used for the result |
| `confidence` | `high`, `medium`, or `low` |
| `scope` | Provider, account, model, profile, or request scope when known |
| `classifier_rule` | Stable fixture-backed rule ID |
| `provider_id` / `model_id` | Safe identifiers when present |
| `phase` / `occurred_at` | Correlated lifecycle phase and timestamp |
| `retry_after` / `reset_at` | Optional validated timing hints |
| `safe_excerpt` | Bounded sanitized evidence for technical details |

Dynamic profile policy documents are versioned and store the two class-policy
objects on each candidate. Legacy maps remain readable only for migration.

Dynamic route state adds the persisted policy decision fields needed to resume
or reject a pending schedule. Attempt history is append-only and records the
policy snapshot, so later profile edits or catalogue changes do not rewrite why
an earlier decision occurred.

Raw streams, credentials, account identifiers, and unbounded error text are not
stored in policy or route state.

## API surface

- Dynamic profile CRUD accepts and returns the versioned per-class policy
  document. Unsupported versions, unknown classes, unknown outcomes, invalid
  bounds, and incomplete class coverage return field-addressable validation
  errors.
- Route state and event payloads expose stable error code, class, retry ordinal,
  maximum retries, absolute deadline, and pending outcome.
- Manual route actions carry expected generation. Cancellation is idempotent.
- The classifier API is internal. Provider adapters submit evidence envelopes;
  no public API accepts arbitrary user-authored regexes or class assignments.

## State machine

| State | Trigger | Next state |
| --- | --- | --- |
| `active` | Current, classified, effect-safe failure with eligible reset wait | `waiting_for_reset` |
| `active` | Current, classified, effect-safe failure with retry budget | `retry_wait` |
| `active` | Recovery exhausted with `skip` | `switching` |
| `active` | Recovery exhausted with `stop`, or unsafe/unclassified failure | `action_required` |
| `waiting_for_reset` | Deadline reached and generation current | `retrying` |
| `retry_wait` | Deadline reached and generation current | `retrying` |
| `waiting_for_reset` or `retry_wait` | Skip now | `switching` |
| `waiting_for_reset` or `retry_wait` | Cancel or stop | `action_required` |
| `retrying` | Success | `active` |
| `retrying` | Classified failure with budget remaining | `waiting_for_reset` or `retry_wait` |
| `retrying` | Recovery exhausted | `switching` or `action_required` |

## Permissions

- Existing agent-profile management permission controls policy edits.
- Profile selection does not grant access to credentials, raw evidence, or
  classifier internals.
- Office assignment permissions do not grant routing-policy mutation.

## Failure modes

- Unknown evidence stops automatic work and preserves sanitized diagnostic
  context for catalogue growth.
- A timing hint that cannot be parsed, is in the past, or exceeds the policy
  maximum does not schedule a wait.
- Invalid policy data is rejected at write time. Invalid persisted legacy data
  loads as an actionable configuration error and never authorizes work.
- If durable state cannot be written, Kandev does not schedule the timer,
  repeat the prompt, or launch a successor.
- If a timer races a manual action, generation fencing commits one outcome.
- If assistant output, tool activity, or a partial utility result makes replay
  ambiguous, the configured policy is not applied automatically.
- If a new semantic code lacks a class assignment or policy coverage,
  exhaustive registry tests fail and runtime classification is unclassified.
- If many sessions share an open resource circuit, one probe owns recovery and
  the rest continue waiting or evaluate other candidates without stampede.

## Persistence guarantees

- Classification, catalogue version, policy snapshot, retry count, deadline,
  and pending outcome survive reload and backend restart.
- Profile edits affect the next failure decision. They do not mutate a pending
  timer or historical attempt unless the user cancels and retries explicitly.
- Absolute deadlines are stored in UTC. Browser clocks affect countdown display
  only.
- A restart never repeats a dispatch whose completion is ambiguous.

## Scenarios

- **GIVEN** a candidate returns a high-confidence capacity error before output,
  **WHEN** its transient policy allows three retries starting at 5 seconds,
  **THEN** Kandev persists waits of 5, 10, and 20 seconds before applying the
  configured exhausted outcome.
- **GIVEN** a quota error includes a trusted reset one minute from now,
  **WHEN** the hard policy allows reset waits up to five minutes, **THEN** the
  route waits durably until the reset and retries the same candidate once.
- **GIVEN** the same reset is six hours away, **WHEN** the maximum wait is five
  minutes, **THEN** Kandev does not shorten or wait for it and proceeds to the
  configured retry or exhausted outcome.
- **GIVEN** a transient retry budget is exhausted with `on_exhausted=skip`,
  **WHEN** another candidate is eligible, **THEN** the same logical dynamic
  session advances to that candidate.
- **GIVEN** a hard policy has no retry or wait and ends in `stop`, **WHEN** the
  provider reports exhausted credits, **THEN** the route enters manual recovery
  without trying another provider.
- **GIVEN** an agent emits an unknown string error, **WHEN** no deterministic
  catalogue rule matches, **THEN** Kandev stops automatic recovery and retains
  only bounded sanitized evidence for a future fixture.
- **GIVEN** a failure follows tool activity, **WHEN** the candidate policy says
  retry or skip, **THEN** effect safety overrides the policy and Kandev stops for
  manual recovery.
- **GIVEN** the same dynamic profile is selected by Kanban and Office, **WHEN**
  each sees the same classified, effect-safe error, **THEN** both apply the same
  candidate policy and route transition.
- **GIVEN** a phone viewport, **WHEN** a user edits both class policies, **THEN**
  all controls remain usable in one column with no horizontal page overflow.

## Out of scope

- Inferring failure from inactivity alone.
- User-authored regular expressions or arbitrary remapping of semantic codes.
- Model or API calls that classify error text, stack traces, or timing hints.
- Automatically buying credits, upgrading subscriptions, authenticating an
  account, or changing provider configuration.
- Overriding effect-safety or generation fencing through profile settings.
- Telemetry, cost, or subscription-usage routing. Those remain in the separate
  Dynamic Agent Telemetry Routing package.
