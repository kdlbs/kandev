---
status: active
system: agents
created: 2026-10-08
updated: 2026-10-28
owners:
  - kandev
---

# Provider Error Signature Classification Requirements

## Overview

Provider routing suspends a candidate and selects another one when a provider
reports a rate limit, an exhausted quota, or a missing subscription. The text
that the classifier reads can also carry the agent's own words: a prompt error
can end with the agent's final message, and agents routinely quote and explain
errors they read from other sessions. A rule that matches a topic word such as
"rate limit" or "subscription" anywhere in that text suspends a healthy account
because its agent wrote about another one.

## Terminology

- **Provider error signature:** wording that a provider or its adapter emits
  for its own failure, such as an HTTP 429 API error, `rate_limit_error`,
  `insufficient_quota`, or "Rate limit exceeded" opening an error line.
- **Agent prose:** assistant text that mentions limits, quotas, credits, or
  subscriptions while describing other work.

## Requirements

### REQ-AGENTS-PROVIDER-ERROR-SIGNATURES-001: Classify provider limits by signature

**Intent:** A rate-limit, quota, or subscription classification must come from
a provider error signature, not from a topic word in agent prose.

**User story:** As an operator, I want a provider account suspended only when
that provider reports a limit, so that an agent summarizing other sessions'
errors does not take its own healthy account out of rotation.

#### Acceptance criteria

- **AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.1:** For `claude-acp`, a rate limit
  shall be classified only from an Anthropic 429 API error, `rate_limit_error`,
  `429 Too Many Requests`, a "you've hit your rate limit" notice, or a line that
  opens with "rate limit exceeded" or "rate limit reached" (optionally after an
  `Internal error:` or `API Error:` label). A mid-sentence quote of another
  provider's "Rate limit exceeded" shall not be a Claude rate limit.
- **AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.2:** For `claude-acp`, a quota limit
  shall require `anthropic_quota_exceeded`, "credit balance is too low", or
  "insufficient credits", and a subscription requirement shall require wording
  that states a missing, inactive, expired, or required subscription.
- **AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.3:** For `opencode-acp`,
  `copilot-acp`, and `amp-acp`, a rate limit shall require "rate limit
  exceeded/reached", `rate_limit_exceeded`, `rate_limit_error`, or "too many
  requests", and a quota limit shall require `insufficient_quota`, "quota
  exceeded/exhausted/reached", "exceeded your current quota", "out of quota",
  or `RESOURCE_EXHAUSTED`.
- **AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.4:** Agent prose that only mentions
  rate limits, quotas, credit balances, or subscriptions shall not be
  classified as `rate_limited`, `quota_limited`, or `subscription_required` for
  any of these providers.
- **AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.5:** Structured HTTP status
  precedence and the existing provider-specific rules (usage limits, session
  limits, credit exhaustion, payment required, authentication, model, and
  installation rules) shall keep their classification.
- **AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.6:** For `opencode-acp`, an
  opening `free usage limit reached` line without an error-envelope label shall
  classify as a quota limit. This criterion covers only the unlabelled
  provider-specific form.
- **AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.10:** OpenCode diagnostics opening
  with `Rate limit exceeded: free-models-per-day...` or
  `Rate limit exceeded: limit_rpd/<model>` shall classify as rate limited even
  when sanitization masks the cadence token. The recorded excerpt remains
  sanitized, and these signatures shall not outrank existing typed signals or
  earlier payment, credit, and quota rules.
- **AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.8:** An opening labeled JSON error
  envelope containing `error_type` with the value `provider_unavailable`
  shall classify as provider unavailable. Bare JSON examples and quoted
  mid-sentence prose shall not trigger it.
- **AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.9:** OpenCode's opening
  `Rate limit exceeded. Please try again later` line, optionally wrapped by
  its provider error label, shall classify as rate limited without inferring a
  daily reset period.
- **AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.11:** For `opencode-acp`, an opening
  `Upstream request failed: [ttft_admission_refused] the cell is at capacity`
  diagnostic shall classify as high-confidence, retryable `model_capacity`.
  Established error labels can precede the diagnostic.
  Bare capacity prose, quoted examples, other refusal tokens, and unrelated
  providers shall not match this signature.
  Existing structured classification and earlier account-limit rules retain precedence.
  A valid `(after N s)` suffix shall provide a reset hint from the diagnostic's observation time.
  Only integer delays from 1 through 60 seconds qualify.
  Missing, malformed, zero, negative, and larger delays shall provide no hint.
  Classification shall not establish runtime health or authorize another prompt.

Criterion 001.11 was added by the October 28
[OpenCode capacity package](../../../plans/opencode-capacity-retry/plan.md).

## Out of scope

- Changing rule order, confidence levels, or classification flags beyond the
  narrow additions in criteria 001.6, 001.8, 001.9, 001.10, and 001.11.
- Changing unrelated provider-neutral rules or any runtime-environment rules.
- Changing how classified failures suspend candidates or select a successor.


## Codex retry-exhaustion signature

### REQ-AGENTS-CODEX-RATE-SIGNATURE-001: Recognize terminal Codex throttling

- **AC-AGENTS-CODEX-RATE-SIGNATURE-001.1:** A Codex error line beginning with
  `exceeded retry limit, last status: 429 Too Many Requests` shall classify as
  high-confidence `rate_limited`. Established error-label prefixes may precede it.
- **AC-AGENTS-CODEX-RATE-SIGNATURE-001.2:** Mid-sentence quotations, other HTTP
  statuses, unrelated providers, and incomplete signatures shall not match this rule.
- **AC-AGENTS-CODEX-RATE-SIGNATURE-001.3:** Existing structured classification
  precedence and explicit quota, authentication, and usage-limit classifications
  shall remain unchanged. Classification alone shall not authorize prompt replay.

[Design](../system-design/provider-error-signatures.md).
[Implementation package](../../../plans/quick-chat-draft-and-rate-retry/plan.md).

### REQ-AGENTS-PROVIDER-ERROR-SIGNATURES-002: Unmatched diagnostic context

**Intent:** Recognize new provider diagnostics without copying whole-message
patterns, while preserving existing classification and recovery fences. The
earlier out-of-scope list describes requirement 001; this extension owns its
additional behavior.

- **AC-AGENTS-PROVIDER-ERROR-SIGNATURES-002.1:** The extension shall be off in
  every shipped profile. OFF shall preserve existing classification and routing.
  Structured HTTP metadata and known signatures shall retain precedence. Only
  an unmatched bounded provider diagnostic may enter the new prefilter.
- **AC-AGENTS-PROVIDER-ERROR-SIGNATURES-002.2:** The prefilter shall evaluate
  structured status, keyword groups, then invocation context. It shall cover
  account permission, qualified model/endpoint removal, timeout, limits,
  overload, context-size rejection and request compatibility. Existing typed
  mappings, including permission-denied HTTP 403, shall remain unchanged.
- **AC-AGENTS-PROVIDER-ERROR-SIGNATURES-002.3:** LIMIT, CAPACITY, GONE, ACCOUNT
  and REQUEST shall normalize case, separators and stems. Two distinct terms
  shall establish a group; a single term or conflicting groups shall retain
  low confidence. Repeating a term shall not increase confidence. Quoted,
  negated and successful assistant text shall not authorize recovery.
- **AC-AGENTS-PROVIDER-ERROR-SIGNATURES-002.4:** Context shall distinguish
  startup, prompt-send and mid-turn, empty output, repeated current attempts,
  reset hints, account/tier/model/path and matching diagnostics observed across
  paths/models. Missing observations shall stay unknown. A proven free daily
  limit without another reset shall use the next midnight UTC.
- **AC-AGENTS-PROVIDER-ERROR-SIGNATURES-002.5:** Account-wide limit scope shall
  require a known paid-account binding. Other failures shall retain model/path
  scope; corroborated model failures may span paths. Explicit reset hints shall
  control suspension expiry. A structured model 404 shall remain unavailable
  until model change or owner release; prose shall never permanently hold an account.
- **AC-AGENTS-PROVIDER-ERROR-SIGNATURES-002.6:** Defaults shall prefer next
  candidate at startup/prompt-send, a same-session retry recommendation at
  mid-turn before next candidate, and stop only for ACCOUNT. These are
  classification defaults, not authorization to execute post-output continuation.
  Generic post-output continuation is outside this extension. Explicit saved policies, existing
  unclassified admission/thresholds, cancellation, ownership, generation,
  output/effect and continuation guards shall take precedence over defaults.
  Existing empty-turn handling shall be reused without changing thresholds.
- **AC-AGENTS-PROVIDER-ERROR-SIGNATURES-002.7:** Classification, diagnostic
  correlation and routing shall consume consistent evidence, explain ON/OFF
  differences and retain model order. No external catalog loader, inference
  service or periodic learner is introduced.
