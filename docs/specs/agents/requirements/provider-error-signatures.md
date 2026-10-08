---
status: active
system: agents
created: 2026-10-08
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

## Out of scope

- Changing rule order, confidence levels, or classification flags.
- Changing the provider-neutral rules or the runtime-environment rules.
- Changing how classified failures suspend candidates or select a successor.
