---
status: current
system: agents
updated: 2026-10-28
requirements:
  - REQ-AGENTS-PROVIDER-ERROR-SIGNATURES-001
  - REQ-AGENTS-CODEX-RATE-SIGNATURE-001
  - REQ-AGENTS-PROVIDER-ERROR-SIGNATURES-002
---

# Provider Error Signature Classification System Design

## Purpose and boundaries

The Agents system owns provider-error classification. This design describes
how the provider rule table separates provider limit signatures from agent
prose. It preserves existing rule order, confidence, classification flags,
unrelated provider-neutral rules, and how dynamic routing acts on a classified
failure. The bounded additions below add exact signatures while preserving
the established higher-priority signals and sanitized excerpt contract.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-AGENTS-PROVIDER-ERROR-SIGNATURES-001` | [Rule signatures](#rule-signatures), [Inputs that carry prose](#inputs-that-carry-prose) |
| `REQ-AGENTS-PROVIDER-ERROR-SIGNATURES-002` | [Opt-in unmatched diagnostic prefilter](#opt-in-unmatched-diagnostic-prefilter) |

## Components and responsibilities

- **`internal/agent/runtime/routingerr`** owns the provider rule table in
  `rules.go` and the classification flags applied to each code.
- **`internal/agent/runtime/dynamic`** evaluates the classified code with the
  candidate's saved recovery policy and opens resource circuits.

With the extension enabled, the terminal stream consumer selects known rules by
`AgentType`, retaining the legacy `AgentID` fallback when type is absent. OFF
preserves the current base attribution. Execution identity, generation,
output and effect fences are preserved.

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

Additional bounded signatures:

- OpenCode's unlabelled opening `free usage limit reached` line is an
  OpenCode-scoped quota rule. The label-wrapped form is outside this follow-up
  and remains separate catalogue work.
- OpenCode daily rate-limit wording (`free-models-per-day` and
  `limit_rpd/<model>`) is matched against raw diagnostic text only because the
  shared sanitizer masks its distinguishing token. `RawExcerpt` remains the
  sanitized excerpt, and existing structured, payment, credit, and quota
  precedence is preserved.
  At the shared ACP message projection, recognized daily reasons that would be
  redacted are reduced to fixed public labels before credential redaction.
  Opaque suffixes are discarded; already-safe text and its complete diagnostic
  identity remain unchanged. The JSON envelope carries the sanitized message
  through the existing fields, so the routing reader can retain the specific
  rule without receiving raw error Data or a new wire field.
- OpenCode Zen's opening `Rate limit exceeded. Please try again later` is a
  generic rate limit; it carries no inferred reset period.
- A labeled opening JSON envelope containing the exact field
  `"error_type":"provider_unavailable"` is a shared provider-neutral
  signature, independent of field order. The existing optional Agent encountered
  an error status prefix is preserved. Bare JSON and quoted prose remain
  unmatched by this rule.

Previously matching higher-priority signatures retain their precedence, code,
confidence, and flags. Text that matched only through a topic word falls
through to the next rule, the
provider-neutral rules, or the phase default.

## Observability

The existing `agent failure classified for provider routing` log records the
rule ID. Rule IDs are unchanged.


## Codex terminal throttling

`REQ-AGENTS-CODEX-RATE-SIGNATURE-001` adds a bounded, line-anchored catalogue
signature in `routingerr/rules.go`. Match the complete observed 429 wording,
with established optional error labels. Keep quota rules ahead of the new rule.
Do not match a bare 429 or an arbitrary occurrence of "retry limit".

Use a distinct stable rule ID and sanitized fixture. Cover direct and prefixed
messages, prose collisions, other statuses, other providers, and quota precedence.
The screenshot establishes the displayed text, not a captured ACP envelope.
Do not fabricate recorded protocol provenance or expose raw error Data.

Interactive policy belongs to Platform. Office and dynamic routing consume the
new classification through their existing policies, without a new retry policy.
[Implementation package](../../../plans/quick-chat-draft-and-rate-retry/plan.md).

## Opt-in unmatched diagnostic prefilter

Requirement 002 adds `features.providerKeywordContext`, off in every profile
and restart-required. Configuration/registry/profile/frontend feature bindings
share that identity and `KANDEV_FEATURES_PROVIDER_KEYWORD_CONTEXT`.

Existing injection, typed status, managed-runtime, provider-specific,
provider-neutral and runtime-environment checks retain their order. Only an
unmatched diagnostic reaches `prefilter.go`, before the phase default. Within
the prefilter, structured status precedes keyword groups and context. Typed
statuses already recognized by the existing classifier never get reclassified.
The 400 request/context distinction authorizes no circuit or replay permission.

`keyword_groups.go` owns the semantic vocabulary. Separator normalization and
bounded stem matching count distinct terms, not occurrences. Single terms and
ties retain low confidence. Unresolved ACCOUNT or GONE evidence also retains
ambiguity when a different group wins by term count; one such term cannot
authorize a stop, hold or automatic retry. Error envelopes, source completeness and current
invocation evidence remain separate from semantic confidence. A keyword match
cannot turn a quoted success into a terminal provider failure.

`KeywordContext` is optional trusted input with unknown zero values.
`KeywordDecision` carries group, suspension scope, backoff and default action.
The orchestrator retains the existing diagnostic-correlation rules. New
unmarked assistant text remains output; the new terminal classifier cannot
clear that safety fence. Its existing prompt-attempt record
holds at most sixteen observations. Attempt identities deduplicate results;
successful output/effects clear continuity and the existing evidence lifecycle
owns cleanup. No service, persistent log, fingerprint algorithm or periodic task
is introduced.

Account context comes from the current session's actual executor and runtime
environment, not the profile family or backend login alone. Session-aware
profile loading keeps that binding consistent in the conductor and classifier.
The existing registered-agent billing detector is reused only when a local
launch demonstrably uses the backend's unchanged default credential home.
Different or unknown homes, remote executors, custom launch/provider overrides,
and incomplete bindings stay unknown without consulting that detector. No new
credential-file reader, account registry or identity algorithm is added.
Known same-account profiles still share the existing credential binding.
Candidate `KeywordAccountID`
uses that same identity without changing the ordinary `BindingKey` used by
known-rule failures. Missing bindings remain isolated. Dynamic circuits reuse
the existing registry and probe leases. Path/model keys isolate unrelated
models; a missing model stays candidate-scoped. Proven paid-account limits
use a separate opt-in account key. Reset hints retain their
deadline, and timed capacity/limit backoff progresses through 5, 15 and 30
minutes. A model-qualified structured 404 holds only that model key; changing
the configured model selects a different key. The registry's existing owner
`ReleaseHold` operation can close a held key; no release API is added.
Disabled candidates ignore keyword metadata when choosing defaults and circuit
durations, retaining the legacy circuit behavior.

Saved candidate policy overrides defaults. Unknown failures and empty
completions retain their existing per-candidate unclassified policy and
threshold. No confidence level bypasses replay, continuation, task ownership,
cancellation, settlement or generation fences.

The returned mid-turn action is a classification recommendation only. Existing
continuation admission is unchanged; generic post-output retry or candidate
continuation is not implemented by this extension.

The minimal future catalog seam is the existing deterministic classifier:
known rules stay before `classifyKeywordContext`; loading and export remain
separate work. No loader or new rule DSL is implemented.

Typed process-start and ACP-initialization failures may separate their own
matching lifecycle wrapper after the original classification remains unknown.
Both the complete wrapped diagnostic and the inner diagnostic remain bounded.
The inner message still needs its error envelope and passes quote/negation
guards. Untyped workspace/executor failures cannot use this path, and OFF and
already classified failures keep their previous behavior.

## OpenCode admission refusal

`AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.11` adds
`opencode.capacity.admission_refused.v1` to `routingerr/rules.go`.
Match the complete opening upstream diagnostic and exact bracketed token.
Keep existing structured status precedence and account-limit rules ahead of it.
Use the sanitized input when its distinguishing token survives sanitization.
If sanitization masks that token, use the existing raw-rule mechanism and retain a sanitized `RawExcerpt`.
Do not match a bare `429`, the word `capacity`, or quoted mid-sentence prose.

Extend `routingerr.Classify` and `resethint.go` to derive `ResetHint` only for this matched rule.
Read the relative suffix only from the line that matched the signature.
Unrelated stderr or stdout lines cannot supply a hint.
Use `Input.OccurredAt`; use the existing current-time fallback when it is absent.
Bound decimal seconds before duration conversion.
An invalid suffix does not invalidate the capacity classification.
Explicit structured hints retain their existing precedence.

Native `_meta["opencode/retry"]` describes OpenCode's own active retry.
Its `provider.rate-limit` label and status `429` do not prove that the ACP prompt ended.
A null update clears that presentation state; it does not authorize a host retry.
Never copy its response body into generic diagnostics or a new recovery record.

Platform owns runtime retention and continuation through
[transient turn continuity](../../platform/system-design/transient-turn-runtime-continuity.md).
That design consumes the semantic classification without a provider-name branch in orchestration.
Office and dynamic routing keep their existing consumers and retry policies.
This addition creates no new classification code, public field, or retry owner.

The [implementation package](../../../plans/opencode-capacity-retry/plan.md)
delivers the positive fixture, collision negatives, and reset-hint bounds.
