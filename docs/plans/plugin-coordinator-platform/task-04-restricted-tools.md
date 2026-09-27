---
id: "04-restricted-tools"
title: "Enforced managed agent tool policy"
status: done
wave: 4
depends_on: ["01-exact-host-foundation", "03-managed-lifetime"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-TOOLS-001
  - REQ-AGENTS-MANAGED-TOOLS-002
acceptance_criteria:
  - AC-AGENTS-MANAGED-TOOLS-001.1
  - AC-AGENTS-MANAGED-TOOLS-001.2
  - AC-AGENTS-MANAGED-TOOLS-001.3
  - AC-AGENTS-MANAGED-TOOLS-002.1
  - AC-AGENTS-MANAGED-TOOLS-002.2
  - AC-AGENTS-MANAGED-TOOLS-002.3
system_design:
  - ../../specs/agents/system-design/managed-tool-policy.md
---

# Task 04: Enforced managed agent tool policy

## Summary

Add provider-enforced restricted turns and transport-derived provenance. Advertise support only after the supported adapter passes the full launch/resume matrix.

## In scope

- Add managed-conversation tool applicability and per-instance allowlists within manifest declarations. Keep the single Kandev MCP broker.
- Carry validated policy through runtime, agentctl, launch, resume, and recovery; fence current execution on every tool effect.
- Prove denied shell/file/network/ambient and foreign-plugin attempts, stale generations, revocation, and unsupported adapters. Document the initial supported provider set from actual adapter evidence.

## Out of scope

- Work owned by other orders, publishing external repositories, and unapproved production rollout.
- Coordinator-specific host roles, private API shortcuts, and changes to unrelated v1 behavior.

## Acceptance

- Restricted turns expose only declared approved tools and authenticated calls cannot forge workspace or execution provenance.
- Resume and recovery enforce the same policy; unsupported adapters fail before launch.
- Disable/revoke cancels execution and denies further effects, including calls already queued at the plugin boundary.

## Verification

Add the named tests before implementation using TDD. These commands run after the work order is implemented; they are not results from planning.

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/... ./internal/agentctl/server/process/... ./internal/mcp/handlers/... -run 'TestManagedToolPolicy|TestManagedToolProvenance' -count=1)
(cd apps/backend && go test ./internal/plugins/manifest/... ./pkg/pluginsdk/...)
```

Evidence to create:

- `apps/backend/internal/agent/runtime/managed_tool_policy_test.go`: `TestManagedToolPolicyLifecycle`.
- `apps/backend/internal/agentctl/server/process/managed_tool_policy_test.go`: `TestManagedToolPolicyAdapter`.
- `apps/backend/internal/mcp/handlers/plugin_tools_managed_test.go`: `TestManagedToolProvenance`.

Follow the [plan verification rules](plan.md#tests), including expected test presence, generated contracts, and public documentation checks.

## Files likely touched

- `apps/backend/internal/agent/runtime/`
- `apps/backend/internal/agentctl/server/process/`
- `apps/backend/internal/agentctl/server/adapter/`
- `apps/backend/internal/mcp/handlers/plugin_tools.go`
- `apps/backend/internal/plugins/manifest/`
- `apps/backend/internal/plugins/host.go`

New test paths above are deliverables. Existing directories indicate the owning boundary; inspect scoped AGENTS.md before editing.

## Dependencies

- [Task 01](task-01-exact-host-foundation.md)
- [Task 03](task-03-managed-lifetime.md)

## Risks

Provider support is a release gate, not an assumed property. Record a real supported-adapter smoke in Results before advertising capability; no permissive fallback.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/managed-tool-policy.md) and [design](../../specs/agents/system-design/managed-tool-policy.md).
- [Ownership decision](../../decisions/2026-09-25-plugin-coordination-platform.md).
- [Source baseline and fork mapping](plan.md#source-baseline-and-fork-mapping).

## Results

Implemented manifest-scoped managed tool declarations, runtime policy propagation,
MCP provenance checks, and invalidation/cancellation when approval or declarations
change. Caller metadata cannot create authority, and unsupported adapters fail
closed before launch. Audit records omit arguments and execution handles.

The supported managed-conversation provider set is **empty**. No provider adapter
has completed the required real launch/resume smoke matrix, so the host does not
advertise a supported provider.

Validation passed:

- Required managed policy/provenance race suite:
  `go test -race ./internal/agent/runtime/... ./internal/agentctl/server/process/... ./internal/mcp/handlers/... -run 'TestManagedToolPolicy|TestManagedToolProvenance' -count=1`.
- Manifest and SDK tests: `go test ./internal/plugins/manifest/... ./pkg/pluginsdk/...`.
- Focused plugin approval/invalidation, managed lifecycle, process adapter,
  provenance, task service, SDK, and executor tests.
- `make build` from `apps/backend`.
- Public documentation validation (62 tests and 47 pages) and `git diff --check`.

### Review remediation (2026-09-27)

Released the approval admission mutex before calling managed plugin tools, while
keeping Host effects behind current capability/provenance checks. A regression
uses the remote tool callback to call the real Host capability, task read, and
exact task write paths. It passed under `go test -race ./internal/plugins`.
