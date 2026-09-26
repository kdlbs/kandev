---
id: "01-normalize-action-url"
title: "Normalize OpenCode action URLs"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-stall-recovery.md"
---

# Task 01: Normalize OpenCode Action URLs

## Acceptance

- `ProviderError` has an optional `remediation_url` field with the existing
  bounded/sanitized contract.
- The OpenCode parser extracts only
  `https://opencode.ai/workspace/<safe-workspace-id>/go`; it removes the URL and
  identifiers from `message` and never returns arbitrary URLs.
- Future structured ACP error data may supply `action_url`, but only the same
  validator accepts it; missing, malformed, wrong-host, query-bearing, and
  oversized values produce no URL.
- Existing correlation, prompt races, stderr projection, and generic error
  behavior remain unchanged.

## Verification

```text
cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/api -run 'Test(OpenCode|ProviderError|Prompt.*Error|HandleWSPrompt)' -count=1
```

Use TDD with the observed URL as a fixture and table-driven rejection cases.
Include a structured ACP error fixture without requiring a live OpenCode
process. Assert no raw URL or workspace identifier appears in the sanitized
message or generic stderr projection.

## Files likely touched

- `apps/backend/internal/agentctl/types/streams/provider_error.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/opencode_stderr.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/opencode_stderr_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go`
- `apps/backend/internal/agentctl/server/api/agent.go`
- `apps/backend/internal/agentctl/server/api/agent_test.go`

## Dependencies and risks

None inside Kandev. The live current-version path still depends on OpenCode
emitting `action_url` through ACP or structured stderr. Do not read its private
log directory or widen the parser to arbitrary prose.

## Results

Implemented 2026-08-07.

- `streams.ProviderError.RemediationURL` added (`remediation_url,omitempty`);
  new `ProviderErrorSourceOpenCodeACP` source for the structured ACP path.
- Shared allowlist `NormalizeOpenCodeActionURL` accepts only
  `https://opencode.ai/workspace/<safe-workspace-id>/go` (exact host, HTTPS,
  no userinfo/query/fragment/port/percent-encoding, bounded identifier and
  URL). Stderr parser captures it from a dedicated `action_url` field or from
  the provider error text before sanitization; `ProviderErrorFromError`
  projects a future ACP `RequestError` carrying structured `action_url`, and
  the message runs through the existing sanitizer so no URL or workspace
  identifier leaks.
- Table-driven tests cover the observed fixture URL, the explicit field,
  wrong hosts/schemes/paths, query/fragment/userinfo/port, traversal IDs,
  oversized values, malformed ACP data, and wrapped generic errors.

Verification: `go test ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/api -run 'Test(OpenCode|ProviderError|Prompt.*Error|HandleWSPrompt)' -count=1` — passed (plus the full acp package suite, 787 tests).
