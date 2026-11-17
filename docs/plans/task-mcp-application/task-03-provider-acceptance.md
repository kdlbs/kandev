---
id: "03-provider-acceptance"
title: "Prove provider resume and catalog acceptance"
status: done
wave: 5
depends_on:
  - 02-safe-replacement
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-APPLY-001
  - REQ-AGENTS-MCP-APPLY-002
acceptance_criteria:
  - AC-AGENTS-MCP-APPLY-001.5
  - AC-AGENTS-MCP-APPLY-001.6
  - AC-AGENTS-MCP-APPLY-002.4
  - AC-AGENTS-MCP-APPLY-002.5
system_design:
  - ../../specs/agents/system-design/task-mcp-application.md
---

# Task 03: Prove provider resume and catalog acceptance

## Summary

Add adapter conformance gates and authoritative application acceptance. Test the replacement across the real binding seam.

## In scope

Define no-prompt native-resume and catalog-completion capabilities. Gate each provider/transport independently.
Require fresh profile guidance, matching process/binding observations, and removal of stale dispatch authority.
Prove same helper/execution identity and surviving workspace resources for each enabled provider/executor combination.
Use the provider matrix in the design. Record tested provider versions and executor evidence.

## Out of scope

Inferring support from a generic resume flag, silently creating a fresh conversation, or claiming mock tests prove a real provider.

## Acceptance

1. New provider-scope registration in the retained helper, attachment, and guidance use one target selection and preserve native conversation identity.
2. Partial, stale, wrong-generation, and timed-out catalogs never report Applied. Retired scopes cannot dispatch through old credentials, including tools still selected in the successor.
3. Each enabled adapter passes a native resume/catalog conformance fixture. Untested shapes return Pending with an unsupported reason.

## Verification

Use TDD for changed logic. Run commands from the repository root. New test paths are targets created by this work order.

```bash
(cd apps/backend && go test ./internal/backendapp ./internal/agent/runtime/lifecycle ./internal/agent/agents -run 'Test(BuiltinApplication|ResumeWorkspaceHelperMCP)' -count=1)
bash scripts/lint-go-changed $(git ls-files --modified --others --exclude-standard -- '*.go')
```

## Files likely touched

- `apps/backend/internal/backendapp/restart_mcp_catalog_test.go (existing fixture pattern)`
- `apps/backend/internal/backendapp/builtin_mcp_application_test.go (new)`
- `apps/backend/internal/agent/runtime/lifecycle/builtin_mcp_application_test.go (new)`
- `apps/backend/internal/agent/agents/ (provider definitions)`
- `apps/backend/internal/agentctl/ (adapter attachment evidence)`

Run the first native conformance fixture for managed Codex app-server 0.166.0, based on the prior disposable evidence.
This does not enable Codex ACP or another version/transport. Those require independent fixtures.
Include an established PTY, a running workspace server, an existing preview, and a failed provider-resume case.
Each fixture records native identity and user-turn count without logging tokens.

## Dependencies

02-safe-replacement

## Risks

Real-provider conformance remains a release gate for each enabled adapter. If no adapter passes, report that the feature cannot deliver automatic application yet.

## Parallelism

Sequential. No delegation is authorized.

## Inputs

- [Requirements](../../specs/agents/requirements/task-mcp-application.md).
- [System design](../../specs/agents/system-design/task-mcp-application.md).
- [Plan and test mapping](plan.md).
- Scoped backend/web guidance and existing builtin MCP tests.

## Results

The Codex app-server adapter exposes the replacement capability only for managed version 0.166.0 on Linux. Source admission requires persisted settled history, known terminal child states, fresh background evidence, a complete connected catalog, no pending permissions, and settled durable delivery.

`TestProviderReplacementManagedCodexConformance` passed against the actual managed binary with race detection. An isolated provider home and local model fixture create one completed turn. The retained helper then resumes the same native conversation, adds Configuration, removes Diagnostics, and verifies the complete live catalog. Model calls remain one. The retained PTY executes a command, the same preview URL returns the same content, and the original editor subprocess connection echoes data.

Other provider versions and transports remain pending unsupported. Mock fixtures do not enable their capability. The native test needs `KANDEV_CODEX_MCP_CONFORMANCE_BIN`; it uses no live user conversation or external model endpoint. Final commands and browser limits are recorded in the plan.
