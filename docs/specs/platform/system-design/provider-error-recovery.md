---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
created: 2026-08-08
updated: 2026-08-31
owners:
  - Kandev
---
# Provider Error Recovery System Design

## Purpose and boundaries

This design preserves the technical source detail for `REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001` | [Migrated source detail](#migrated-source-detail), [Cursor normal-completion failure projection](#cursor-normal-completion-failure-projection), [Cursor retry-safety semantics](#cursor-retry-safety-semantics), [Interactive transient retry notice lifecycle](#interactive-transient-retry-notice-lifecycle) |

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

#### Cursor normal-completion failure projection

`cursor-agent` can report an upstream HTTP/2 stream reset as an ordinary
`agent_message_chunk`. It can later return a successful `session/prompt`
response. The ACP transport therefore owns a Cursor-specific evidence
projection that mirrors the existing Codex capacity projection. It does not add
generic content scanning to orchestration.

The observer runs in the ordered ACP notification worker and applies these
checks in order:

1. The adapter identity is `cursor-acp`.
2. The normalized event is a non-empty assistant message chunk for a non-zero
   prompt generation that matches the active turn.
3. After trimming leading and trailing whitespace, the chunk begins with
   `Error: RetriableError:`.
4. A case-insensitive bounded match finds `RetriableError` and either
   `http/2 stream closed` or `CANCEL (0x8)`.

The identity, event-type, and prefix checks precede the full fingerprint. The
prefix check uses `strings.HasPrefix(strings.TrimSpace(text), ...)`, so ordinary
per-token traffic does not allocate a normalized copy. Prose that mentions the
error after other text, a partial `RetriableError`, a stale generation, and the
same text from another adapter do not match.

A match sets pending evidence on the active `promptTurnState` under its existing
evidence mutex. The observer suppresses the control chunk. A later non-empty
assistant or thought chunk clears the marker for the same generation. A new
tool call also clears it. This activity proves that Cursor resumed its own
attempt. A later matching control chunk sets the marker again. Updates for an
in-flight tool do not clear it. Such updates can arrive during error settlement
without proving renewed provider generation.

After `session/prompt` returns, `sendPrompt` drains the notification queue. Then
it examines the turn. If the Cursor marker is pending, the adapter cancels the
async completion owner. It emits exactly one `EventTypeError` instead of the
ordinary complete event. The error carries this bounded constant diagnostic:
`Error: RetriableError: HTTP/2 stream closed with error code CANCEL (0x8)`.
The valid `ProviderError` uses source `cursor_acp`, provider ID `cursor-acp`, and
a UTC occurrence time. The adapter does not copy raw assistant text into the
structured diagnostic.

The deterministic catalogue adds the dedicated rule
`cursor.retriable_stream_reset.v1` before the generic transport-loss rule. The
rule requires the complete normalized Cursor diagnostic
`Error: RetriableError: HTTP/2 stream closed with error code CANCEL (0x8)`
(with the existing bracketed `canceled` decoration allowed). It maps to
`agent_transport_lost` with high confidence and class `transient`.
It retains the existing `AutoRetryable` invariant. The rule applies the shared
context-cancellation veto. Cursor's `[canceled]` token does not satisfy that
veto. It lacks `context canceled`, `context deadline exceeded`, or
`cancel escalated`. The deliberately narrow `transportLostRe` remains
unchanged.

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

#### Cursor retry-safety semantics

The Cursor label `RetriableError` is evidence about the upstream transport. It
is not permission for Kandev to repeat a turn. The following rules define the
Cursor recovery choices. They preserve the provider-neutral safety boundary in
[ADR-2026-08-08-provider-neutral-agent-error-recovery](../../../decisions/2026-08-08-provider-neutral-agent-error-recovery.md):

1. **Safe point.** A terminal marker is automatically replayable only when its
   prompt-generation-correlated evidence is known and records neither assistant
   output nor tool activity. Thoughts, message output, a pending or completed
   tool call, and missing evidence all fail closed. The observed incident had
   thoughts and a `Read File` call in flight, so it enters manual recovery even
   though its classification is transient.
2. **Continuation mode.** An eligible concrete-profile retry retains the
   selected execution profile. It uses the existing provider-native resume
   identity before it sends the cached original prompt again. It does not create
   a fresh provider route or switch providers. This replay is allowed only at
   the safe point above. An unsafe turn exposes the existing manual Resume and
   Start fresh choices. The user, not the classifier, chooses continuation.
3. **Cursor-owned retry.** Kandev does not schedule while the original
   `session/prompt` RPC remains open. Provider progress after a marker clears
   the pending marker. Only a later terminal marker can re-arm it. The prompt
   barrier then proves that Cursor's internal retry has either resumed or
   finished before Kandev chooses recovery. This prevents overlapping Cursor
   and Kandev retry loops.
4. **Budget and delay.** Eligible concrete-profile recovery reuses the single
   orchestrator-owned retry entry, `transientMaxAttempts`, and
   `transientRetryDelayFor`. There is no Cursor-specific nested counter, timer,
   or backoff. Exhaustion uses the existing manual recovery path.

The orchestrator records replay evidence for every interactive prompt, not only
dynamic route attempts. The evidence remains scoped by session, execution, and
prompt generation. Lifecycle snapshots the current prompt's evidence before it
marks a terminal completion as activity and carries that immutable snapshot on
`agent.failed`. This prevents separate NATS subscriptions for `agent.stream.*`
and `agent.failed` from changing the replay decision based on delivery order.
The concrete-profile `handleTransientFailure` path requires the same known,
no-output, no-tool condition before scheduling. A missing record is unsafe.
Dynamic profiles retain `dynamicPreResultSafe` and their configured policy
owner. A model-switch restart reserves the cached prompt and replay identity
before `StartAgentProcess` can dispatch the replacement prompt, then binds the
identity to the replacement execution. `agent_transport_lost` remains
same-provider recovery evidence and does not gain permission to switch
candidates merely because Cursor supplied the new fingerprint. No orchestration
branch inspects `cursor-acp`, `cursor_acp`, or the raw diagnostic.

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

### Interactive transient retry notice lifecycle

Concrete-profile task chat persists one status message for each scheduled
retry attempt. The message metadata contains `retrying: true`; its visible
content includes the safe reason, provider, attempt ordinal, absolute deadline,
and Cancel action. The backend timer owns the retry. The persisted message is a
transcript projection of that ownership, not an independent retry state.

The orchestrator attempts to resolve every outstanding transient-retry status
message for a session whenever retry ownership ends through success, exhaustion,
a terminal event, coordinator stop, explicit cancellation, an ordinary session
stop, or shutdown. It lists the session transcript through the task service,
selects every message whose metadata has the boolean value `retrying: true`, and
deletes each selected message through the task service. Task-service deletion
remains the single mutation path and publishes `session.message.deleted`;
connected clients remove each successfully deleted message from their stores,
while reload and task switching read a transcript that no longer contains any
notice whose cleanup succeeded. Listing or deletion failures are logged and
swallowed, leaving a later authorized cleanup to retry remaining notices.

Normal successful turns perform this durable lookup only when the orchestrator
still owns an in-memory retry entry. Explicit stop, terminal, and Cancel paths
force the lookup so a persisted notice can still be retired after its timer or
process has already disappeared.

Deletion is intentional. Changing only `retrying` to `false` would make the
current task-chat renderer treat the old retry message as a generic settled
warning, preserving stale retry text and its Cancel action. The retry attempts
remain observable through agent output and recovery history without retaining
an actionable status projection after ownership ends.

Advancing from attempt N to attempt N+1 only cancels the superseded in-memory
timer. It does not run the retry-ending reset and therefore does not retire the
current attempt's notice prematurely. Durable message operations do not run
while the orchestrator's runtime-state mutex is held.

`CancelTransientRetry` authorizes the task-session pair before reading or
mutating retry state. After authorization it always retires outstanding retry
notices. If a retry loop is active, cancellation also disarms its timer and
creates the existing manual recovery message with Resume and Start fresh
actions. If no loop remains, cancellation is an idempotent cleanup: it reports
no active cancellation and creates no recovery message. Denied and foreign
pairs remain indistinguishable.

Transcript listing or individual deletion failures are logged and swallowed.
The resolver continues after an individual deletion failure so one corrupt or
racing message cannot prevent other notices from being retired. This cleanup
does not turn an otherwise successful agent transition into a failure.

Desktop and mobile use the same inline retry card and Cancel action. This
change does not add a separate mobile composition; both layouts consume the
same deletion event and must stop rendering the notice durably.

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
- If transient-retry transcript cleanup cannot list or delete a message, the
  orchestrator logs the failure without failing the successful, terminal, or
  cancellation transition. A later authorized cleanup can retry remaining
  notices.

## Persistence guarantees

- Classification, catalogue version, policy snapshot, retry count, deadline,
  and pending outcome survive reload and backend restart.
- Profile edits affect the next failure decision. They do not mutate a pending
  timer or historical attempt unless the user cancels and retries explicitly.
- Absolute deadlines are stored in UTC. Browser clocks affect countdown display
  only.
- A restart never repeats a dispatch whose completion is ambiguous.
- A completed, exhausted, stopped, terminal, or cancelled interactive retry has
  no outstanding `retrying: true` transcript message after successful cleanup.
  Each successful deletion is broadcast to all viewers through the task-service
  event bus. A failed cleanup is best effort and remains eligible for a later
  authorized cleanup.

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
- **GIVEN** an interactive transient retry succeeds, **WHEN** its session later
  becomes idle or its task is reloaded, **THEN** no retry notice is rendered.
- **GIVEN** an authorized user selects Cancel after the retry loop has already
  ended, **WHEN** a stale retry notice remains, **THEN** the notice is retired
  without creating a manual recovery message.
- **GIVEN** an authorized user selects Cancel while a retry loop is active,
  **WHEN** cancellation completes, **THEN** the retry notice is retired and the
  manual Resume and Start fresh recovery message is rendered.

## Out of scope

- Inferring failure from inactivity alone.
- User-authored regular expressions or arbitrary remapping of semantic codes.
- Model or API calls that classify error text, stack traces, or timing hints.
- Automatically buying credits, upgrading subscriptions, authenticating an
  account, or changing provider configuration.
- Overriding effect-safety or generation fencing through profile settings.
- Telemetry, cost, or subscription-usage routing. Those remain in the separate
  Dynamic Agent Telemetry Routing package.
