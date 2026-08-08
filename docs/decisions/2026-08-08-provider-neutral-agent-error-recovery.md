# ADR-2026-08-08-provider-neutral-agent-error-recovery: Separate Agent Error Evidence From Recovery Policy

**Status:** accepted
**Date:** 2026-08-08
**Area:** backend, frontend, protocol

## Context

Agent CLIs expose equivalent provider failures in incompatible ways. An ACP
prompt may return a structured error, a process may emit a correlated diagnostic
on managed stderr, or an agent may split one failure across several apparently
successful ACP frames. In the observed Codex capacity incident, the agent sent a
`systemError` session-info update, followed it with "Selected model is at
capacity", and then returned `end_turn`. Kandev treated the successful RPC return
as normal completion even though the sequence described a transient provider
failure.

The existing retry path recognizes a single Anthropic `529 Overloaded` message
inside orchestration code. Extending that pattern for every Codex, Claude,
OpenCode, and future agent message would couple lifecycle policy to agent prose.
It would also make it too easy to retry hard limits such as inactive
subscriptions, exhausted quotas, missing credentials, or invalid model
configuration.

Automatic prompt replay carries a second risk: a provider may fail after the
agent has already emitted content or invoked a tool. Retrying such a turn can
duplicate side effects even when the provider failure itself is transient.

Kandev also has two materially different execution contexts. Kanban sessions
are interactive and normally supervised by the user who selected one concrete
agent profile. Office runs are commonly unattended, may have an explicitly
configured provider fallback chain, and already persist provider health and
parked-run deadlines for scheduler recovery. A single global `AutoRetryable` or
`UserAction` verdict cannot correctly represent both policies.

The current split policy is already vulnerable to drift: `routingerr` marks
`provider_overloaded` auto-retryable, while Office's separate resolver
allow-list omits that code; the same allow-list treats low-confidence
`unknown_provider_error` as scheduler-retryable. Classification and Office
policy therefore need separate but exhaustive, single-owner tables.

## Decision

Kandev separates provider-failure handling into four layers:

1. **Agent evidence adapters** collect bounded signals from ACP responses,
   ordered ACP updates, managed structured stderr, process exits, and HTTP status
   metadata. They correlate evidence with the active agent session and prompt
   generation, sanitize it, and emit a provider-neutral evidence envelope.
2. **A shared classifier registry** maps that evidence to a stable semantic
   error code and confidence. Agent- or provider-specific signatures live in
   registry rules and adapter extractors, never in the orchestrator or UI.
3. **An explicit execution context** selects a consumer policy. Kanban's
   interactive recovery policy and Office's unattended routing policy consume
   the same classification but make different retry, fallback, and scheduling
   decisions. Adapters do not independently authorize automatic work.
4. **Workspace-specific recovery owners** apply the selected policy. Kanban owns
   a short same-profile retry loop and inline chat feedback. Office owns durable
   provider health, configured fallback, parked runs, and scheduler/inbox state.

The evidence envelope is prompt-generation scoped and can contain multiple
ordered signals. This allows a Codex `systemError` metadata update plus its next
bounded message and `end_turn` result to classify as one failure without treating
an isolated metadata update or arbitrary model text as authoritative.

### Stable taxonomy and context-specific policy

The initial policies are:

| Semantic cause | Kanban interactive policy | Office unattended policy |
| --- | --- | --- |
| `network_unavailable`, `provider_overloaded`, `provider_unavailable`, `model_capacity` | Short same-profile retry when high-confidence and replay-safe | Retry the same route for a few seconds; only degrade and fall through after the short budget is exhausted |
| `rate_limited` | Retry only a confirmed short throttle; honor a bounded retry hint | Retry the same route when a validated wait is at most 60 seconds; otherwise treat it as a long-horizon route block and fall through |
| `quota_limited` | Show reset/remediation information; no automatic prompt replay | Fall through to the next configured provider; if all routes are exhausted, park until a validated reset or the Office quota backoff |
| `auth_required`, `missing_credentials`, `subscription_required`, `model_unavailable`, `provider_not_configured` | User action; no automatic retry or route change | Mark the failed route user-action-required and try another configured route; if none works, block and surface inbox/settings remediation without a timed retry for that route |
| `permission_denied_by_user`, `task_error`, `repo_error` | Existing terminal recovery | Do not treat as provider unavailability or change routes |
| `unknown_provider_error`, `agent_runtime_error`, unclassified evidence | Manual recovery | Pre-start fallback may be allowed conservatively; post-start ambiguous failures do not change providers |

`quota_limited` means a renewable account or plan allowance has been exhausted.
A short provider throttle is `rate_limited`, while an inactive or insufficient
subscription or billing state is `subscription_required`. Classifiers must not
collapse these causes merely because they may share an HTTP status. A separate
`billing_required` code is intentionally not part of the initial taxonomy;
Kanban only displays a quota reset hint, and Office may use a validated reset
hint to wake an unattended parked run.

Only high-confidence evidence correlated with the active foreground prompt can
authorize Kanban automatic retry or Office post-start fallback. Unknown,
malformed, stale, background-request, or uncorrelated evidence fails closed in
Kanban and cannot trigger a post-start Office route change. Mere inactivity is
never provider-failure evidence.

Classifier rules have stable IDs, deterministic priority, bounded input, and
table-driven fixtures containing positive, negative, correlation, and redaction
examples. Common structured and exact-message signatures are declarative.
Agent-specific code is limited to extracting or assembling evidence that cannot
be represented by a single signal. Adding support for another observed error is
normally a new fixture plus a registry rule that maps it to an existing semantic
code; it does not add an agent-type branch to lifecycle or presentation code.

### Kanban interactive retry

A transient classification makes a failure eligible for automatic retry; it is
not by itself permission to replay. Interactive retry also requires one of:

- the failure occurred before assistant content or tool activity for the prompt;
- the adapter provides a provider-supported resume/retry guarantee that does not
  replay completed effects.

If neither condition holds, Kandev presents the transient failure with manual
recovery actions instead of automatically resending the prompt.

Eligible interactive retries preserve the selected agent profile, execution
profile, model, turn configuration, and original prompt. Kandev never purchases
capacity or silently changes models, accounts, agents, or providers. The default
schedule allows five automatic attempts with nominal delays of 5, 10, 20, 40,
and 60 seconds. Each delay has bounded jitter, and a trustworthy short
provider-supplied retry hint is a lower bound. The backend publishes the exact
scheduled UTC `retry_at`; the UI does not attempt to reproduce the backoff
formula.

One backend owner exists per session and prompt generation. A new user prompt,
model or profile change, session stop, start-fresh action, successful retry,
explicit cancellation, or a later prompt generation disarms the old schedule.
A stale timer or delayed ACP frame cannot retry a successor turn. After the
attempt budget is exhausted, the session uses the existing manual Resume and
Start fresh recovery path.

### Office unattended routing and scheduling

Office does not reuse the Kanban timer or prompt-replay state. Every Office run
first separates short-horizon failures from long-horizon route blocks, regardless
of whether automatic provider fallback is enabled.

High-confidence network failures, temporary provider unavailability or
overload, model capacity, and short rate throttles enter a
`same_route_short_retry` phase. One run owns the retry attempt, while a
workspace/provider/model-scoped short cooldown makes concurrent runs wait rather
than switch providers or create a retry stampede. Office retries the same execution profile after
nominal delays of 5, 10, and 20 seconds with bounded jitter. No provider-health
degradation or fallback exclusion occurs during this phase. A successful retry
clears the phase without changing the resolved route. If all three retries fail,
the condition is promoted to provider degradation and Office may fall through
to the next configured execution profile when automatic routing is enabled. If
fallback is disabled or no alternative exists, the run enters durable provider
capacity recovery instead.

A validated rate-limit delay longer than 60 seconds, renewable quota exhaustion,
auth or subscription failure, and model/configuration unavailability bypass the
short phase. They make the current route ineligible immediately and fall through
to the next configured profile when routing is enabled. This is not a silent
policy invention: the workspace's provider order and tier mappings are the
user's prior authorization for that fallback.

For a short transient before assistant output or tool activity, Office can
re-drive the original prompt. After effectful progress, a same-provider retry
starts a fresh provider-native session with the existing Office continuation
instruction to inspect durable task and worktree state; it does not blindly
replay the original prompt. An ambiguous dispatch acknowledgement is never
automatically resent.

Office preserves its durable provider-health schedule. Transient and renewable
quota routes use the existing minute-scale 2, 5, 10, 20, and 60 minute ladder
with bounded jitter when no validated reset is available; a validated reset
time takes precedence. Auth, inactive-subscription, missing-configuration, and
invalid-model routes remain `user_action_required` without a timed retry, while
other configured routes may still be attempted. If every route is exhausted,
the run is durably parked as either `waiting_for_provider_capacity` or
`blocked_provider_action_required` and survives backend restarts.

Office's separate generic run-failure scheduler remains responsible for
non-provider infrastructure failures, using its own retry budget and CEO
escalation. A classified provider-routing failure must be consumed by routing
before it can fall into that generic retry loop.

The Office short-retry phase is scheduler-owned and durable despite its
second-scale delays. It records the cooldown scope, owning run, route attempt,
execution profile, retry ordinal, and absolute `retry_at`. The original failed attempt does not join
the route-cycle provider exclusion set until the short budget is exhausted. A
backend restart can therefore re-arm a provably undispatched short retry; an
ambiguous dispatching attempt follows normal Office reconciliation and is not
duplicated.

### Visible backend-owned progress

Kanban exposes structured recovery state including semantic code, interactive
disposition, prompt generation, scheduled retry ordinal, maximum attempts,
`retry_at`, and safe provider/model labels. The task chat renders one localized
inline warning with the exact next retry and a live countdown plus a Cancel
action. The client derives remaining time from `retry_at`, so it does not require
a persisted message update every second. Assistive technology is notified when
recovery changes state, not on every countdown tick.

Office continues to expose route attempts, resolved provider/model, provider
health, fallback reason, parked status, and the durable next scheduler wake in
task/run detail, agent detail, dashboard, and inbox surfaces. Its `retry_at` is a
same-route retry, route-health, or parked-run deadline. During the short phase it
shows, for example, "Model at capacity — retrying Codex in 8s (2/3)" and does not
claim that fallback has started. Office actions include Retry now and, when an
alternative is configured, Try next provider now; reconnect/configure and route
history remain available for long-horizon blocks.

Within each workspace type, desktop and mobile expose the same explanation,
progress, next retry or wake, and available actions. Kanban mobile keeps the
existing chat composition; Office mobile keeps the existing task/dashboard
navigation. Both provide touch targets of at least 44px, with no desktop-only
recovery control or unnecessary new drawer.

Kanban scheduled state survives browser reloads. If the backend restarts, it may
re-arm a durable `scheduled` retry only when it can reconstruct the exact prompt
and configuration and prove the retry was not dispatched. A retry found in an
ambiguous `dispatching` state becomes manual recovery rather than risking a
duplicate prompt. The UI must never continue displaying a countdown for a retry
the backend no longer owns.

Office does not need that reconstruction rule: its provider health, route
attempts, parked status, and scheduler deadlines are already durable, and a
post-restart dispatch starts from durable task/worktree state under the selected
execution profile.

## Consequences

Provider-specific knowledge becomes independently extensible and testable while
each execution context retains the policy appropriate to its supervision model.
Codex, Claude, OpenCode, and future agents receive consistent classification
even when their wire evidence differs. Kanban does not silently route or wait on
hard limits; Office can keep unattended work moving through explicitly
configured fallbacks and durable scheduler state without switching providers for
a momentary network or capacity blip.

The classifier registry becomes a safety boundary. Rules need collision tests,
sanitized evidence fixtures, and metrics keyed by semantic code, execution
context, policy result, agent type, provider, and classifier rule ID. Both retry
owners need injectable clock and jitter sources, but their state machines,
budgets, persistence, and user surfaces remain independent.

The current `applyInvariants`, Office `autoRetryableCodes`, scheduling, and inbox
action switches must converge on one exhaustive Office policy function. Kanban
must have its own exhaustive policy function. Tests enumerate every semantic
code against both contexts so adding a classifier code cannot silently inherit
or miss automatic behavior.

This decision supersedes
[ADR 0011](0011-transient-provider-error-retry.md). Its visible retry behavior is
retained for Kanban, while its Anthropic-specific classifier and fixed 5/15/30
schedule are replaced by the provider-neutral taxonomy and Kanban policy
described here.

## Alternatives Considered

- **Add agent-message checks to each lifecycle handler.** Rejected because
  provider prose, agent dialects, retry policy, and UI behavior would diverge.
- **Retry every ACP `systemError`, HTTP 429/5xx, or failed prompt.** Rejected
  because those shapes also include auth, quota, subscription, configuration,
  ambiguous-delivery, and task failures.
- **Let each agent adapter decide whether to retry.** Rejected because adapters
  should translate evidence, not independently define product policy.
- **Keep one global `AutoRetryable` / `UserAction` verdict on classification.**
  Rejected because interactive prompt replay and unattended provider routing
  have different authorization, persistence, timing, and escalation semantics.
- **Let the browser own the timer and send the retry.** Rejected because closing
  or reloading a page would alter backend behavior and multiple clients could
  dispatch the same retry.
- **Automatically switch model or provider on capacity errors.** Rejected for
  interactive sessions because it changes user-selected cost and behavior.
  Office fallback remains available only when the workspace explicitly enabled
  and configured it.
- **Apply Kanban's no-hard-limit-retry rule to Office.** Rejected because Office
  is intentionally unattended and its configured fallback chain plus durable
  scheduler are designed to route around renewable provider limits. Inactive
  subscriptions still require action; only renewable quota/reset evidence can
  arm an Office capacity wake.
- **Fall through to the next Office provider on the first transient failure.**
  Rejected because momentary network, overload, and model-capacity failures often
  clear within seconds; switching CLIs loses provider-native continuity and is
  only warranted after a short same-route budget or clear long-horizon evidence.
- **Treat inactivity as a transient provider error.** Rejected because quiet
  tools and long-running model work are valid; inactivity remains advisory.
