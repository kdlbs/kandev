---
created: 2026-09-16
status: complete
requirements:
  - REQ-AGENTS-INJECTED-MCP-APPROVAL-001
system_design:
  - ../../specs/agents/system-design/injected-mcp-approval.md
legacy_specs: []
---

# Fix plan: Claude injected MCP approval

## Overview

Resolve [issue #3729](https://github.com/kdlbs/kandev/issues/3729) through its minimum server-wide parity option.
The issue is assigned to `carlosflorencio`.
One sequential work order implements the complete permission path and its regression coverage.
Implementation is complete through the single work order below.

## Evidence and root cause

Investigated commit: `4c6e80091d82b4302e9dfc0a17b80965f1d4c62d`.
The canonical issue has no comments or image attachments at investigation time.

The process manager checks only blanket `AutoApprovePermissions` before it stores a pending request.
There is no injected-server exception. The ACP client also discards the programmatic tool name.
Codex's separate passthrough strategy already emits a server-wide approval override.

A temporary `TestIssue3729Repro` used `Config.NewInstanceConfig(43210, nil)` and the real manager permission handler.
It submitted `mcp__kandev__update_task_plan_kandev`, kind `other`, with allow-once and reject-once choices.
Blanket approval remained false. The handler emitted `permission_request` and waited until context cancellation.

Command run:

```bash
(cd apps/backend && go test ./internal/agentctl/server/process -run '^TestIssue3729Repro$' -count=1 -v)
```

Result: PASS, with the unwanted permission event observed. The temporary test was removed after execution.
This proves the host permission path, not a live Claude or desktop session.

## Scope

### In scope

- Preserve ACP programmatic tool identity and normalize legacy Claude identity.
- Approve the host-injected server before pending-request creation.
- Preserve unrelated permissions and normal tool events.
- Document the server-wide policy and cover fresh and resumed configurations.

### Out of scope

- Per-tool allowlists, new settings, persistence, and Claude terminal passthrough.
- Frontend changes, provider upgrades, and unverified provider dialects.
- Changes to Codex policy or server-side authorization.

## Technical approach

Follow the [system design](../../specs/agents/system-design/injected-mcp-approval.md).
The implementation crosses the existing ACP client, adapter, instance configuration, and process manager.
Use new focused helper and test files instead of expanding large manager files.

## Tests

| Criteria | Permanent regression evidence |
| --- | --- |
| 001.1, 001.3, 001.8 | `client_permission_identity_test.go:TestPermissionProgrammaticName` and `dialect_claude_test.go:TestClaudePermissionIdentity` |
| 001.2, 001.4, 001.7 | `config_test.go:TestInjectedKandevMCPProvenance` and `manager_permission_policy_test.go:TestInjectedKandevPermissionPolicy` |
| 001.5, 001.6 | `manager_permission_policy_test.go:TestInjectedKandevPermissionOptions` |
| 001.1, 001.3, 001.7, 001.8 | `manager_permission_acp_test.go:TestInjectedKandevPermissionACPFlow` |

The first policy regression must fail on current code because the manager emits a pending request.
Tests must include mixed server lists, spoofed names, a Bash title that resembles an MCP name, and reversed option ordering.

## End-to-end evidence

The Go protocol integration test connects the real ACP conversion, adapter, and process manager through an in-memory ACP peer.
It emits a Kandev permission request and verifies the selected wire response without a pending permission event.
It then emits Bash and third-party requests and verifies that both remain pending until explicit resolution.
Run the same sequence with fresh and resumed session identities and with task and Quick Chat instance contexts.
Verify normal tool-call/result events remain present.
This protocol boundary is the deterministic end-to-end evidence. No artificial browser test or UI redraw is required.

## Work orders

- [x] [Task 01: Implement injected server approval](task-01-injected-server-approval.md)

## Verification results

Investigation reproduction passed as described above. Implementation and verification passed.

- `(cd apps/backend && go test -race ./internal/agentctl/server/acp ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/config ./internal/agentctl/server/process ./internal/agent/mcpconfig -count=1)`: passed.
- `node --test scripts/validate-public-docs.test.mjs`: passed.
- `node scripts/validate-public-docs.mjs`: passed.
- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- Catalog discovery found both new agent specification files.
- GitHub assignment readback confirmed `carlosflorencio`.

## Risks

- The minimum policy approves all injected Kandev tools, including destructive and work-spawning tools.
- Legacy Claude title interpretation requires exact provider and kind checks. It must not become a generic title heuristic.
- Installed provider formats can differ. Unsupported shapes retain prompts, and compatibility claims require fixtures.
- No live provider session was run during investigation.
