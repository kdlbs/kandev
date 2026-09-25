---
id: "01-injected-server-approval"
title: "Implement injected server approval"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-INJECTED-MCP-APPROVAL-001
acceptance_criteria:
  - AC-AGENTS-INJECTED-MCP-APPROVAL-001.1
  - AC-AGENTS-INJECTED-MCP-APPROVAL-001.2
  - AC-AGENTS-INJECTED-MCP-APPROVAL-001.3
  - AC-AGENTS-INJECTED-MCP-APPROVAL-001.4
  - AC-AGENTS-INJECTED-MCP-APPROVAL-001.5
  - AC-AGENTS-INJECTED-MCP-APPROVAL-001.6
  - AC-AGENTS-INJECTED-MCP-APPROVAL-001.7
  - AC-AGENTS-INJECTED-MCP-APPROVAL-001.8
system_design:
  - ../../specs/agents/system-design/injected-mcp-approval.md
---

# Task 01: Implement injected server approval

## Summary

Add a permission decision for the host-injected Kandev server without enabling blanket approval.
Preserve identity through the ACP conversion and use strict provider normalization.

## In scope

- Start with a failing manager regression for an injected Kandev plan update with blanket approval disabled.
- Add identity fields, Claude dialect normalization, injection provenance, and offered-option selection through the documented path.
- Add the unit and protocol integration tests named in the plan.
- Update the permission sections in `docs/public/agents-and-profiles.md` and scoped agentctl guidance.
- Mark the ADR accepted only after explicit implementation authorization for this package.

## Out of scope

- New permission settings, storage changes, UI controls, and unrelated approval cleanup.
- Changes to blanket approval's existing first-option fallback.
- Removing Codex's existing launch override.

## Acceptance

1. All listed criteria pass through real production seams, including the protocol integration test.
2. Unknown identity, forged title, unrelated server, missing provenance, and missing allow choices never receive approval through the new policy.
3. Public reference text explains the server-wide exception, its destructive-tool scope, and its independence from blanket approval.

## Test matrix

- Names: programmatic qualified name, Claude metadata, legacy Claude title, empty name, malformed metadata, conflicting fields, wrong server, suffix-only name.
- Operations: plan creation/update, title update, task deletion/creation, Bash, file edit, third-party MCP.
- Injection: actual constructor, zero port, user-supplied reserved name, wrong port/path/transport, mixed server inventory, recreated instance.
- Options: reject first, allow-always first with allow-once later, allow-always only, reject only, empty, unknown kinds.
- Lifecycle: fresh/resumed session, task/Quick Chat context, cancellation of a remaining prompt, blanket approval on/off.
- Streams: normal tool-call/result events, no automatic-path permission card, normal explicit resolution for unrelated tools.

Use channel synchronization and cleanup registration before assertions.
Do not use arbitrary sleeps or a real user instance.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race ./internal/agentctl/server/acp ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/config ./internal/agentctl/server/process ./internal/agent/mcpconfig -count=1)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Record the initial failing regression and each final command result.

## Files likely touched

Paths below are repository-relative. New files are marked `(new)`.

- `apps/backend/internal/agentctl/types/types.go`
- `apps/backend/internal/agentctl/types/permission_identity.go` (new)
- `apps/backend/internal/agentctl/types/permission_identity_test.go` (new)
- `apps/backend/internal/agentctl/server/acp/client.go`
- `apps/backend/internal/agentctl/server/acp/client_permission_identity_test.go` (new)
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_claude.go` (new)
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_claude_test.go` (new)
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_permissions.go`
- `apps/backend/internal/agentctl/server/config/config.go`
- `apps/backend/internal/agentctl/server/config/config_test.go`
- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/manager_permission_policy.go` (new)
- `apps/backend/internal/agentctl/server/process/manager_permission_policy_test.go` (new)
- `apps/backend/internal/agentctl/server/process/manager_permission_acp_test.go` (new)
- `apps/backend/internal/agentctl/AGENTS.md`
- `docs/public/agents-and-profiles.md`
- `docs/decisions/2026-09-16-injected-mcp-approval.md`

## Dependencies

None.

## Risks

Server-wide approval includes task deletion and creation.
The implementation must not infer trust from raw input or a display title from an unknown provider.
Public permission snapshots must not expose the newly preserved provider metadata.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/injected-mcp-approval.md)
- [System design](../../specs/agents/system-design/injected-mcp-approval.md)
- [Plan and evidence](plan.md)
- Existing `client_test.go` permission conversion tests.
- Existing `manager_permission_test.go` pending-request and cancellation tests.
- Existing `config_test.go` injected HTTP/SSE ordering tests.
- Repository `/tdd` skill and its backend test reference.

## Results

Implemented the Claude ACP injected-server permission path. ACP conversion now
preserves internal programmatic identity and metadata, the Claude dialect
normalizes only verified identity forms, and instance construction records
host-injection provenance. The process manager approves every qualified tool
on the exact injected Kandev HTTP or SSE server, preferring allow-once and then
allow-always, while blanket approval and all unrelated pending flows remain
unchanged. Flattened names with delimiter collisions fail closed to the
pending flow. Permanent unit and in-memory ACP protocol tests cover fresh and
resumed task and Quick Chat contexts, destructive and planning tools, spoofed
identities, malformed metadata, option ordering, normal tool/result events, and
explicit unrelated-tool resolution.

The public permissions reference and scoped agentctl guidance document the
server-wide exception and its independent authorization boundaries. The ADR is
accepted, and the paired requirement and system design are active/current.

Verification passed:

- `(cd apps/backend && go test -race ./internal/agentctl/server/acp ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/config ./internal/agentctl/server/process ./internal/agent/mcpconfig -count=1)`
- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.test.py`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`
