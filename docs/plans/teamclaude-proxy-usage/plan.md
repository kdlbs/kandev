---
created: 2026-09-03
status: in_progress
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
system_design: []
legacy_specs: []
---

# Implementation Plan: Gateway Proxy Usage Adapters

## Overview

Add a proxy-usage resolver layer ahead of Kandev's direct provider clients.
Each gateway supplies a small resolver that recognizes an explicitly selected
profile configuration and constructs a gateway-native usage client. The initial
supported gateway set is TeamClaude, OpenRouter, and LiteLLM. TeamClaude is the
first implementation: a Claude ACP profile with `USAGE_PROXY_VENDOR=teamclaude`
and loopback `ANTHROPIC_BASE_URL` reads only the TeamClaude status control
plane. Its result is a pool summary using the least-utilized usable account for
each known quota window; individual identities and credentials are never
retained.

## Scope

### In scope

- An ordered, independently testable proxy-resolver seam.
- A first-class profile proxy configuration (`vendor`, `endpoint`,
  `usage_mode`, and optional credential-secret reference), replacing the
  temporary environment-variable selection before additional vendors ship.
- A bounded, timeout-controlled TeamClaude status client.
- An OpenRouter authenticated-key usage client.
- A LiteLLM configured admin/spend usage client.
- Pool-level 5-hour, 7-day, Sonnet, and Fable utilization windows.
- Explicit `USAGE_PROXY_VENDOR` selection plus loopback-only endpoint derivation
  from `ANTHROPIC_BASE_URL`.

### Out of scope

- Guessing a vendor from an endpoint or agent runtime.
- Forwarding a profile API key to an endpoint without an explicit proxy
  configuration and secret reference.
- Account switching, usage polling, or dynamic-routing admission changes.
- Changing direct Anthropic or OpenAI usage clients.

## Work orders

- [x] [Task 01: Add the proxy usage resolver layer and TeamClaude adapter](task-01-read-teamclaude-pool-utilization.md)
- [ ] [Task 02: Persist profile proxy configuration](task-02-profile-proxy-configuration.md)
- [ ] [Task 03: Add OpenRouter key-usage adapter](task-03-openrouter-key-usage.md)
- [ ] [Task 04: Add LiteLLM configured usage adapter](task-04-litellm-usage.md)

## Follow-on provider/error rollout

1. Anthropic and TeamClaude: validate upstream 429/500/503/529 plus
   TeamClaude `rate_limit_error` and `proxy_error` fixtures; use the TeamClaude
   pool adapter for proactive availability presentation only.
2. OpenAI and Codex: complete `rate_limit_exceeded`, `insufficient_quota`,
   invalid API key, model-unavailable, and subscription-limit fixtures.
3. Gemini and Antigravity: add Google quota/rate-limit, model, authentication,
   and entitlement fixture extractors.
4. OpenAI-compatible gateways: add a resolver only where the gateway has a
   documented local status API; normalize standard OpenAI JSON and HTTP errors
   for OpenRouter, OpenCode-backed local providers, and LiteLLM-style proxies.
5. Copilot and Cursor: complete adapter-specific entitlement, authentication,
   and rate-limit fixtures.

## Verification results

- `cd apps/backend && go test -tags fts5 ./internal/agent/usage` passed.
- `cd apps/backend && go test -tags fts5 ./internal/backendapp -run
  'TestTeamClaudeStatusURL' -count=1` passed.
