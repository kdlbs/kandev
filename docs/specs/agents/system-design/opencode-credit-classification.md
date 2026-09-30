---
status: current
system: agents
requirements:
  - REQ-AGENTS-OPENCODE-CREDIT-001
---

# OpenCode Credit-Exhaustion Classification System Design

## Purpose and boundaries

The Agents system owns provider-error classification. This design describes
how it classifies OpenCode credit-exhaustion and billing diagnostics. It does
not change route order, credential circuits, or reset-time parsing.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-AGENTS-OPENCODE-CREDIT-001` | [Classification](#classification), [Recovery](#recovery) |

## Components and responsibilities

- **`internal/agent/runtime/routingerr`** owns sanitized provider diagnostics,
  structured status precedence, the provider rule table, and classification
  flags.
- **`internal/agent/runtime/dynamic`** evaluates the classified code with the
  candidate's saved recovery policy.
- **`internal/orchestrator`** owns task-session attribution, route persistence,
  and `session.state_changed` publication.
- **The frontend session store** owns the local route-status projection and
  updates it from the existing WebSocket event.

## Classification

The classifier processes structured HTTP status before provider-specific
rules. The OpenCode rules then run in this order: period usage limits, explicit
credit exhaustion, payment requirement, generic quota, rate limit, and auth.

The `opencode.stderr.credit.v1` rule recognizes `credit limit reached`, `out of
credit(s)`, `insufficient credit(s)`, and `insufficient balance`. The bounded
phrases do not match an unrelated mention of credits. A match yields
high-confidence `quota_limited` with fallback allowed.

The `opencode.stderr.subscription.v1` rule recognizes `payment required`. It
runs after the credit rule, so a message that states both payment requirement
and credit exhaustion keeps the quota result. A payment-only message yields
high-confidence `subscription_required` with user action and no automatic
retry. Structured HTTP status retains priority over either text rule.

The shared sanitizer removes renewal URLs and dates before provider rules run.
Neither rule needs those values or derives a reset time from them.

## Recovery

The dynamic runtime classifies failures from the existing OpenCode ACP error
path. It applies the candidate's stored policy to the resulting error code and
class. Quota exhaustion follows the hard-error policy. The default outcome
skips the exhausted candidate and selects the next eligible route.

Payment-only errors use the subscription category. The category marks the
route for user action and disables automatic retry. Dynamic routing may still
select another candidate when the saved policy and replay-safety evidence
permit it. Classification does not bypass the existing generation, prompt,
output, or tool-effect safety checks.

The orchestrator stores route state and attempt history in
`dynamic_route_states` and `dynamic_route_attempts`. It projects the route state
onto the task session and publishes `session.state_changed`. The frontend
session store applies that update through its existing WebSocket handler. The
existing `session.route_action` request lets the user manage a waiting or
action-required route. This change adds no request or event fields.

## Observability

The existing `agent failure classified for provider routing` log records the
classification code, confidence, rule ID, fallback flag, retry flag, and user
action flag. The new rule IDs distinguish quota exhaustion from a billing
condition.
