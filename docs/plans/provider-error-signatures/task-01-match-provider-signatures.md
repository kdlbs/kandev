---
id: "01-match-provider-signatures"
title: "Match provider limit signatures instead of topic words"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROVIDER-ERROR-SIGNATURES-001
acceptance_criteria:
  - AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.1
  - AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.2
  - AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.3
  - AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.4
  - AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.5
system_design:
  - ../../specs/agents/system-design/provider-error-signatures.md
---

# Task 01: Match provider limit signatures instead of topic words

## Summary

Replace the topic-word rate, quota, and subscription patterns in the provider
rule table with provider error signatures, keeping rule order, confidence, and
rule IDs.

## Changes

- `claude.stderr.rate.v1`: Anthropic 429 API errors, `rate_limit_error`,
  `429 Too Many Requests`, "you've hit your rate limit", and a line-opening
  "rate limit exceeded/reached".
- `claude.stderr.quota.v1`: word-bounded `anthropic_quota_exceeded`, "credit
  balance is too low", "insufficient credits".
- `claude.stderr.subscription.v1`: missing, inactive, expired, or required
  subscription wording.
- `opencode`, `copilot`, `amp` rate rules: shared rate-limit signature.
- `opencode`, `amp` quota rules: shared quota signature.

## Acceptance

1. Agent prose about rate limits, quotas, credit balances, and subscriptions
   classifies as none of `rate_limited`, `quota_limited`, or
   `subscription_required` for `claude-acp`, `opencode-acp`, `copilot-acp`, and
   `amp-acp`, in the prompt-send and streaming phases.
2. Claude prose that quotes another provider's "Rate limit exceeded"
   mid-sentence is not a Claude rate limit.
3. The listed provider signatures still classify as before, and the existing
   classifier tests pass unchanged.

## Verification

- `go test ./internal/agent/runtime/routingerr/ -run 'TestClassify_AgentProseAboutLimitsIsNotAProviderLimit|TestClassify_ProviderLimitSignaturesStillClassify'`
- `go test ./internal/agent/runtime/routingerr/`
- `make -C apps/backend build`
