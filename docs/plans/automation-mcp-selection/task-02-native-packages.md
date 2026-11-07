---
id: "02-native-packages"
title: "Bind and enforce native packages"
status: done
wave: 2
depends_on:
  - "01-selection-contract"
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATION-MCP-002
  - REQ-OFFICE-AUTOMATION-MCP-003
acceptance_criteria:
  - AC-OFFICE-AUTOMATION-MCP-002.1
  - AC-OFFICE-AUTOMATION-MCP-002.2
  - AC-OFFICE-AUTOMATION-MCP-002.3
  - AC-OFFICE-AUTOMATION-MCP-002.4
  - AC-OFFICE-AUTOMATION-MCP-002.5
  - AC-OFFICE-AUTOMATION-MCP-002.6
  - AC-OFFICE-AUTOMATION-MCP-003.1
  - AC-OFFICE-AUTOMATION-MCP-003.2
  - AC-OFFICE-AUTOMATION-MCP-003.4
system_design:
  - ../../specs/office/system-design/automation-mcp-selection.md
---

# Task 02: Bind and enforce native packages

## Summary

Make every selected native package usable by private automation processes.
Preserve the Automation principal while authorizing the bound package actions.

## In scope

Update Automation descriptors, v2 manifest validation, Core assembly, optional builders and handlers, and raw dispatch checks.
Update Configuration resource/grant guards, launch preparation, runtime carriers, helper negotiation, recovery, and generated guidance.

## Out of scope

Workspace server additions, normal-task templates, and rendered editor work.

## Acceptance

- Each optional package passes discovery and an actual authorized invocation. Unselected/raw/cross-package calls fail before effects.
- Configuration cannot change any package grant or automation/plugin approval through dedicated, generic, nested, or creation paths.
- Fresh starts use current choices. Continuation/adoption preserve accepted choices. Old helpers reject explicit bindings before start.

## Verification

Run from the repository root. Use `/tdd` for meaningful logic tests.
New test paths named by this work order must exist before its verification completes.

```bash
(cd apps/backend && go test -tags fts5 -trimpath ./internal/mcp/profile ./internal/mcp/server ./internal/mcp/handlers ./internal/mcp/scope -run 'AutomationMCP|Automation|Builtin' -count=1)
(cd apps/backend && go test -tags fts5 -trimpath ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle ./internal/agentctl/server/... ./internal/sysprompt/... -run 'AutomationMCP|Builtin' -count=1)
git diff --check
```

Run `bash scripts/lint-go-changed` with the changed Go file paths. Record the actual expanded command and result.

## Files likely touched

- `apps/backend/internal/mcp/profile/builtin_packages.go`
- `apps/backend/internal/mcp/profile/builtin_descriptors.go`
- `apps/backend/internal/mcp/server/builtin_registration.go`
- `apps/backend/internal/mcp/server/builtin_actions.go`
- `apps/backend/internal/mcp/handlers/automation_authorization.go`
- `apps/backend/internal/mcp/scope/`
- `apps/backend/internal/orchestrator/executor/executor_builtin_mcp.go`
- `apps/backend/internal/orchestrator/executor/builtin_mcp_context.go`
- `apps/backend/internal/agent/runtime/`
- `apps/backend/internal/agentctl/`
- `apps/backend/internal/sysprompt/`
- `apps/backend/internal/mcp/server/automation_packages_test.go (new)`
- `apps/backend/internal/mcp/handlers/automation_packages_test.go (new)`
- `apps/backend/internal/orchestrator/executor/automation_mcp_selection_test.go (new)`

## Dependencies

01-selection-contract.

## Risks

Never implement Configuration by changing the surface or bypassing the entire automation guard. Verify operation-specific resource access.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/office/requirements/automation-mcp-selection.md).
- [System design](../../specs/office/system-design/automation-mcp-selection.md).
- [Authority decision](../../decisions/2026-11-04-automation-mcp-selection.md).
- Existing plugin-tool save tests, built-in package binding tests, and workspace resolver tests.

## Results

Version 2 Automation manifests, package catalogs and actual handlers, principal checks, raw dispatch authorization, helper capability checks, and launch binding are implemented.

- The combined backend command in [the plan](plan.md#verification-results) passed all discovered package, handler, lifecycle, helper, and prompt checks. No cases were skipped.
- `TestAutomationMCPPackageInvocation` executes one handler for each optional package. `TestAutomationMCPRawDispatch` checks accepted and unbound actions plus foreign workspace denial.
- The browser private run invoked selected native tools and checked the accepted package list and observed package servers.
- A save during a live private process preserved its accepted packages. The next process on the reused conversation accepted the new packages.
- Existing generic binding/recovery and surface-exclusion checks ran with the `Builtin` filter. Shared Configuration service guards remain in force.
- Changed-Go lint is recorded centrally in the plan.

### PR remediation

PR remediation verifies canonical resource keys, workspace-owned profile targets, preparation-time helper negotiation, Core question/permission publication, and automation-surface mismatch rejection. The raw-dispatch test uses the guarded dispatcher. The expanded backend command and results are in the plan.
