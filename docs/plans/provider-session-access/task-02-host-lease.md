---
id: "02-host-lease"
title: "Expose exact Host lease and GitHub credential adapter"
status: pending
wave: 2
depends_on:
  - 01-grant-ledger
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001
acceptance_criteria:
  - AC-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001.3
  - AC-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001.4
  - AC-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001.5
  - AC-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001.6
  - AC-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001.9
system_design:
  - ../../specs/integrations/system-design/provider-session-access.md
---

# Task 02: Expose exact Host lease and GitHub credential adapter

## Summary

Add the optional versioned Host RPC/SDK contract and bind redemption to the
grant, active session, linked PR head and provider connection. Mint and revoke
a distinct one-repository GitHub App token per redeemed lease.

## In scope

- `provider-access/v1` exact wire and SDK methods with H6 capability check.
- Current linked PR/fork/head and run identity validation at issuance and
  redemption; no provider mutation.
- Uncached GitHub App token mint, transient plugin-only delivery, provider
  revoke on release/invalidation, and bounded pending-revocation receipt.
- GitLab PAT/unsupported-provider fail-closure and lifecycle revocation hooks.

## Out of scope

Plugin provider calls and task MCP provider-action tools.

## Acceptance

- The plugin can redeem one token only while exact grant, session, target,
  approval and connection identity remain current; no task agent sees it.
- HTTP fixtures prove one canonical repository and the minimum permissions,
  with no shared cache or PAT fallback.
- Provider revocation success and failure are distinguished; grant/session
  revocation fences new redemption immediately.

## Verification

```bash
(cd apps/backend && go test -race -count=1 ./internal/provideraccess ./internal/plugins ./internal/github ./pkg/pluginsdk)
(cd apps/backend && go test -count=1 ./internal/backendapp ./internal/mcp/server)
```

## Files likely touched

- `apps/backend/proto/kandev/plugin/v1/plugin.proto`
- `apps/backend/pkg/pluginsdk/`
- `apps/backend/internal/plugins/`
- `apps/backend/internal/provideraccess/`
- `apps/backend/internal/github/`
- `apps/backend/internal/backendapp/`

## Dependencies

Task 01 and a reviewed `provider-access/v1` fixture with the plugin owner.

## Risks

GitHub's token cannot be narrowed to a PR/run; Host crashes can leave an
issued token valid until GitHub expiry if revocation was not confirmed.

## Parallelism

`sequential`

## Inputs

- `apps/backend/internal/github/app_token_cache.go`
- `apps/backend/pkg/pluginsdk/host.go`
- `docs/decisions/2026-09-26-plugin-direct-provider-access.md`

## Results

Pending.
