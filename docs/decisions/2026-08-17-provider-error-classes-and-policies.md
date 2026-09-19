# ADR-2026-08-17-provider-error-classes-and-policies: Classify Provider Errors Before Applying Candidate Policy

**Status:** accepted
**Date:** 2026-08-17
**Area:** backend, frontend, protocol, workflow

## Context

Kandev already normalizes provider failures into semantic codes, but recovery
policy is split across classifier-owned booleans, Kanban retry tables, Office
routing tables, and dynamic candidate maps. The dynamic map currently stores a
generic `retry_same`, `try_next`, or `stop` action by semantic code. It cannot
express a retry budget, exponential interval, bounded waiting for a provider
reset, or the outcome after retries are exhausted.

The same selected dynamic profile can run in Kanban, a utility invocation, or
Office. If each caller maps provider strings or codes independently, the
profile's meaning changes with the workspace mode and the error catalogue
drifts. Provider and agent CLIs also report many failures as unstructured text,
so the catalogue must grow as real examples are observed without making broad
unknown matches automatically actionable.

Automatic replay and fallback can duplicate side effects. A policy setting
must not turn ambiguous prompt delivery, assistant output, tool effects, or a
partial utility result into permission to repeat work.

## Decision

Kandev uses one provider-neutral classification and policy pipeline:

1. Agent adapters produce bounded, sanitized, invocation-correlated evidence.
2. A deterministic fixture-driven catalogue assigns a stable semantic code,
   one policy class, confidence, scope, rule ID, catalogue version, and
   validated timing hints.
3. A shared effect-safety gate decides whether automatic recovery is allowed.
4. A shared policy evaluator applies the selected candidate's class policy and
   returns a durable route transition.
5. Kanban, utility, workflow, and Office owners present or schedule that
   transition without reclassifying the error.

### Two configurable classes

Recoverable provider errors belong to one of two user-configurable classes:

- `transient`: temporary network, transport, provider availability, overload,
  selected-model capacity, and confirmed short throttling failures.
- `hard`: account, subscription, quota, credentials, provider configuration,
  or selected-model failures that need a longer reset or user/configuration
  change.

Semantic codes remain stable diagnostic detail and can grow independently.
Task, repository, permission, local runtime, cancellation, and resume-state
failures are outside provider policy.

Unknown, low-confidence, stale, or conflicting evidence is `unclassified`.
This is an internal classification state, not a third configurable class. It
always fails closed to manual recovery. A new semantic code without an explicit
class assignment also classifies as unclassified until exhaustive tests and
the catalogue are updated.

### Policy is a pipeline, not one action

Each dynamic candidate stores a complete policy for each class:

- optional reset waiting with a maximum accepted wait,
- optional same-candidate retry with maximum additional retries and an initial
  interval that doubles exponentially, and
- an explicit exhausted outcome of `skip` or `stop`.

Evaluation first applies a trusted future `retry_after` or `reset_at` when it is
within the configured maximum. Each candidate and class can use that reset wait
once per route cycle. If no eligible wait applies, the evaluator uses the next
retry interval while budget remains. When recovery is exhausted, it skips the
candidate or stops according to the configured outcome. The reset wait and its
one post-wait attempt do not consume the exponential retry budget.

This policy shape avoids an implicit answer to "what happens after retry?" and
supports immediate skip or stop by disabling both wait and retry.

### Safety and durable ownership override policy

The policy evaluator can run only for current, effect-safe evidence. User
configuration cannot override generation fencing, continuation requirements,
or replay safety. Ambiguous work stops even when the policy requests retry or
skip.

Backend-owned route state persists the classification, policy snapshot, retry
ordinal, absolute deadline, and pending exhausted outcome before scheduling.
Timers, manual actions, and provider events use one generation-fenced
transition owner. Restart reconciliation re-arms only work proven not to have
dispatched.

### One catalogue and one evaluator across modes

`internal/agent/runtime/routingerr` owns the evidence and deterministic
catalogue contract. A provider-neutral runtime policy package owns class-policy
validation and evaluation. Dynamic routing consumes it directly. Kanban,
utility calls, and Office supply invocation and effect context, but do not own
provider allow-lists or duplicate policy schedules.

Concrete-profile interactive recovery may keep different product defaults from
dynamic profiles. It still consumes the shared class and timing metadata.

### Compatibility

The versioned dynamic candidate document replaces generic action maps. Existing
rules are normalized when read or updated:

- `try_next` becomes immediate `skip` for both classes.
- `stop` becomes immediate `stop` for both classes.
- `retry_same` becomes one retry after 5 seconds followed by `stop`.
- Explicit per-code rules are mapped to the class assigned to that code, with a
  validation error when conflicting code rules would produce two policies for
  one class.

New candidates use immediate `skip` for both classes. This preserves current
fallback behavior and does not introduce hidden retries or waits.

### Catalogue growth and future classifiers

Catalogue rules have stable IDs, explicit priority, bounded input, and
sanitized positive, negative, correlation, and redaction fixtures. Historical
attempts keep the code, class, rule ID, and catalogue version selected at the
time.

A future model-based classifier can be considered as another evidence-to-
classification producer with provenance and confidence. This decision does
not authorize network calls, model-authored policy, or automatic recovery from
model-only classification. That work requires a separate decision and rollout.

## Consequences

### Positive

- A dynamic profile has the same recovery meaning in Kanban and Office.
- Users can configure bounded retry, near-reset waiting, skip, and stop without
  per-provider rules.
- Unknown strings are safe by default while the catalogue can grow through
  fixtures.
- Retry exhaustion, timer ownership, and restart behavior are explicit.
- Provider adapters and UIs do not duplicate recovery tables.

### Costs

- Dynamic policy persistence and route state need a versioned migration.
- The settings editor becomes denser and needs progressive disclosure on
  desktop and phone.
- Existing Kanban and Office policy tables must converge on the shared class
  contract without changing effect-safety boundaries.
- Durable retry and reset waiting require scheduler reconciliation and race
  tests.

## Alternatives Considered

### Keep rules keyed only by semantic code

Rejected. It exposes catalogue churn to users, creates repetitive settings,
and still leaves retry schedules and exhaustion undefined.

### Keep a single action per class

Rejected. `retry` cannot say what follows exhaustion, and `wait` cannot say
what happens when a reset is absent or beyond the maximum.

### Let Kanban and Office keep separate policy tables

Rejected. The same dynamic profile would behave differently by caller and new
error codes would require synchronized allow-lists.

### Treat unknown errors as transient

Rejected. Broad automatic replay or provider switching can duplicate effects
and hides classifier gaps.

### Use a model to classify unknown errors now

Deferred. It adds latency, cost, privacy, availability, prompt-injection, and
confidence-calibration concerns. Deterministic classification and a growing
fixture catalogue are required first.

## Relationship to Prior Decisions

This decision amends
[Separate Agent Error Evidence From Recovery Policy](2026-08-08-provider-neutral-agent-error-recovery.md).
It retains the evidence and semantic-code boundary, replaces workspace-owned
provider allow-lists with shared classes, and keeps product-specific defaults
outside the classifier.

It amends
[Unify Provider Routing Behind Dynamic Agent Profiles](2026-08-13-dynamic-agent-profile-routing.md)
by replacing generic candidate error actions with versioned per-class policy.

It supersedes the fixed transient attempt schedule in
[Retry transient provider errors with backend-owned exponential backoff](0011-transient-provider-error-retry.md)
for dynamic profiles. Concrete-profile interactive retry can retain that
decision's defaults while consuming the shared classification contract.

Product behavior is specified in
[Provider Error Recovery](../specs/platform/requirements/provider-error-recovery.md) and
[Dynamic Agent Routing](../specs/agents/requirements/dynamic-agent-routing.md).
