---
status: current
system: agents
requirements:
  - REQ-AGENTS-PROVIDER-ERROR-SIGNATURES-001
---

# Provider Error Signature Classification System Design

## Purpose and boundaries

The Agents system owns provider-error classification. This design describes
how the provider rule table separates provider limit signatures from agent
prose. It does not change rule order, confidence, classification flags, the
provider-neutral rules, or how dynamic routing acts on a classified failure.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-AGENTS-PROVIDER-ERROR-SIGNATURES-001` | [Rule signatures](#rule-signatures), [Inputs that carry prose](#inputs-that-carry-prose) |

## Components and responsibilities

- **`internal/agent/runtime/routingerr`** owns the provider rule table in
  `rules.go` and the classification flags applied to each code.
- **`internal/agent/runtime/dynamic`** evaluates the classified code with the
  candidate's saved recovery policy and opens resource circuits.

## Inputs that carry prose

Every channel the provider rules read can carry agent text. A prompt that ends
in error reports the agent's final message with the error, and assistant
message chunks are classified to mark provider diagnostic candidates. Agents
that coordinate other sessions quote those sessions' errors verbatim. The rules
therefore match error signatures and never a bare topic word.

## Rule signatures

Shared rate-limit signature (`opencode-acp`, `copilot-acp`, `amp-acp`):
"rate limit(ed) exceeded/reached", `rate_limit_exceeded`, `rate_limit_error`,
and "too many requests". These are the forms emitted by the AI-SDK based
adapters, OpenRouter-style gateways, and plain HTTP 429 reasons.

Shared quota signature (`opencode-acp`, `amp-acp`): `insufficient_quota`,
"quota exceeded/exhausted/reached", "exceeded your current quota", "out of
quota", and `RESOURCE_EXHAUSTED`.

Claude rate-limit signature: `API Error: 429` (including `Request rejected
(429)`), `rate_limit_error`, `429 Too Many Requests`, "you've hit your rate
limit", and "rate limit exceeded/reached" only when it opens a line, optionally
after `Internal error:` or `API Error:`. A Claude prompt error can carry the
agent's final prose, and prose quotes other providers' "Rate limit exceeded"
mid-sentence, so the bare phrase is accepted only at the start of a line.

Claude quota signature: `anthropic_quota_exceeded`, "credit balance is too
low", and "insufficient credits". Claude subscription signature: "requires an
active (paid) Claude [Pro|Max|Team|Enterprise] subscription", "subscription
expired / is required / is inactive / not active", and "no active
subscription".

Rule order and confidence are unchanged, so a provider signature that matched
before still yields the same code, confidence, and flags. Text that matched
only through a topic word now falls through to the next rule, the
provider-neutral rules, or the phase default.

## Observability

The existing `agent failure classified for provider routing` log records the
rule ID. Rule IDs are unchanged.
