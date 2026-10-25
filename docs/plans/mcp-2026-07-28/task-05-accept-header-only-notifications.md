---
id: "05-accept-header-only-notifications"
title: "Accept header-only modern notifications"
status: done
wave: 4
depends_on: ["01-serve-dual-era-mcp"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-PROTOCOL-001
acceptance_criteria:
  - AC-AGENTS-MCP-PROTOCOL-001.9
system_design:
  - ../../specs/agents/system-design/mcp-protocol-compatibility.md
---

# Task 05: Accept Header-Only Modern Notifications

## Inputs

- `AC-AGENTS-MCP-PROTOCOL-001.9`.
- `docs/specs/agents/system-design/mcp-protocol-compatibility.md` section
  `Header-only notifications`.
- The notification compatibility amendment in
  [the dual-era decision](../../decisions/2026-08-30-dual-era-mcp-protocol.md).
- The shared route registrations in `apps/backend/internal/mcp/server/server.go`.

## Change

1. Route both `/mcp` registrations through a narrow HTTP adapter.
2. For a POST with a supported modern protocol header, supply
   `params._meta` protocol version metadata only on a valid notification
   with no `id` member and no `params._meta`.
3. Read the body lazily, when the SDK first reads it, so the SDK's transport
   checks run before the body is consumed.
4. Leave requests, explicit metadata, batches, invalid JSON, legacy versions,
   and body read errors on the SDK path.
5. Preserve duplicate envelope and parameter members and Unicode case variants
   verbatim, preventing supplementation from changing SDK validation.
6. Bypass the adapter for declared ordinary request methods, preserving their
   original bodies and avoiding an additional full-body buffer.

## Acceptance

- A header-only `notifications/roots/list_changed` returns HTTP 202 with no
  body through the adapter.
- Requests with an `id` member and explicit metadata keep SDK validation.
- A request rejected by SDK DNS-rebinding protection does not read the body.
- An unauthenticated external notification does not read the body; an otherwise
  identical request with a valid personal access token is accepted.
- Both registered routes accept a notification, then list and call a tool.
- Body read and close errors retain the original error identity.
- Declared ordinary requests reach the SDK with their original body, while a
  notification without `Mcp-Method` still receives compatibility handling.

## Verification

```bash
cd apps/backend
go test -trimpath -race ./internal/mcp/server/ ./internal/backendapp/ -count=1
go vet ./internal/mcp/server/
```

## Files likely touched

- `apps/backend/internal/mcp/server/notification_protocol_compatibility.go`
- `apps/backend/internal/mcp/server/notification_protocol_compatibility_test.go`
- `apps/backend/internal/mcp/server/notification_protocol_preservation_test.go`
- `apps/backend/internal/backendapp/mcp_notification_auth_test.go`
- `apps/backend/internal/mcp/server/server.go`

## Dependencies

Task 01. The adapter wraps the shared dual-era server.

## Result

- Added the lazy notification adapter and wired both `/mcp` registrations to it.
- Focused tests cover acceptance, preserved SDK validation, untouched message
  shapes, legacy and unsupported versions, body read errors, and body ordering
  behind DNS-rebinding protection.
- The author recorded `go test ./internal/mcp/server/ -count=1` and `go vet`
  as preparation checks for the original implementation.
- PR remediation preserves duplicate envelope and parameter members and Unicode
  case variants, and leaves declared ordinary request bodies with the SDK.
- [Hosted race proof 39061689634](https://github.com/kdlbs/kandev/actions/runs/39061689634)
  passed all 14 selected top-level notification tests in the MCP server and
  backend application packages, including both routes, subsequent tool use,
  authentication and DNS checks before reads, read/close errors, numeric
  precision, and ordinary request body preservation. The command used
  `go test -trimpath -race -count=1 -timeout=3m -json` with an explicit
  notification-cohort `-run` selector; the workflow retains the exact command
  and JSON output. No retry was used.
- Before the fixes, hosted assertions demonstrated SDK 400 versus adapter 202
  for conflicting duplicate metadata and replacement of the original bodies of
  valid ordinary requests. A non-pointer assertion fixture was corrected and
  rerun before the ordinary-method guard was implemented.
