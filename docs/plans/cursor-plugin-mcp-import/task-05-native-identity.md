---
id: "05-native-identity"
title: "Preserve Cursor credential identity and reject unproven cache imports"
status: complete
wave: 5
depends_on: ['03-launch-materialization']
plan: "plan.md"
requirements:
  - REQ-AGENTS-CURSOR-PLUGIN-MCP-001
  - REQ-AGENTS-CURSOR-PLUGIN-MCP-003
acceptance_criteria:
  - AC-AGENTS-CURSOR-PLUGIN-MCP-001.1
  - AC-AGENTS-CURSOR-PLUGIN-MCP-001.6
  - AC-AGENTS-CURSOR-PLUGIN-MCP-001.10
  - AC-AGENTS-CURSOR-PLUGIN-MCP-003.5
system_design:
  - ../../specs/agents/system-design/cursor-plugin-mcp-import.md
---

# Task 05: Native plugin identity repair

## Scope

Authorized follow-up to the live-task investigation, after the review-remediation commit.
Emit native plugin identifiers so the existing workspace credential aggregator remains
usable. Keep logical-name policy checks and profile/project precedence. Skip cache
and marketplace roots without readable enablement metadata, including malformed
registry data. Preserve source credentials, approval/disable stores and user-owned
configuration.

No UI changes. No login, token refresh, blanket approval, live-task mutation or
Cursor service integration. The native enabled-plugin inventory is remote; importing
that inventory into ACP remains unsupported by this filesystem-only repair.

## Regression evidence

- RED: unverified cached Figma was imported; emitted Atlassian name did not match
  its shared OAuth key; raw project name, global native identity and denied native profile name failed to
  suppress the native import.
- GREEN: those regressions passed after the repair.
- Existing ownership regression: replace unchanged owned bare Atlassian with its
  native identifier and remove stale owned Figma on the next preparation.
- Existing precedence, policy, credential opt-out and reconciliation tests remain
  part of the focused verification.

## Validation

Run from `apps/backend`:

- `go test -race ./internal/agent/mcpconfig ./internal/agent/runtime/lifecycle -run 'Cursor|ProjectMCP' -count=1`
- `golangci-lint run ./internal/agent/mcpconfig/... ./internal/agent/runtime/lifecycle/...`

Run specification and public documentation validators from the repository root.
Record actual results in the package manifest. Live authenticated provider behavior
is not established by fixtures.

## Results

Focused Cursor/ProjectMCP tests passed with the race detector. Scoped Go lint
passed with writable temporary Go/linter caches. Specification validation,
specification lint, all 62 public-doc validator tests, public-doc validation
and whitespace checks passed. Production provider login was not exercised;
the live task and personal credentials were not modified.
