---
id: "03-backend-toggle-delivery"
title: "Backend toggle and delivery"
status: done
wave: 3
depends_on:
  - "01-agentctl-credential-enforcement"
  - "02-agentctl-credentialed-entries"
plan: "plan.md"
requirements:
  - REQ-AGENTS-BUILTIN-MCP-CALLER-AUTH-002
  - REQ-AGENTS-BUILTIN-MCP-CALLER-AUTH-003
acceptance_criteria:
  - AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-002.1
  - AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-002.2
  - AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-003.1
  - AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-003.2
  - AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-003.4
system_design:
  - ../../specs/agents/system-design/builtin-mcp-caller-auth.md
---

# Task 03: Backend toggle and delivery

## Summary

The backend adds the `builtinMcpCallerAuth` release toggle and sends its value
on every instance creation. It reads each instance's credential on demand from
that instance and uses it on every backend-built delivery path, including after
a restart that adopts a surviving instance.

## In scope

- Toggle: `profiles.yaml` (prod and dev `"false"`, e2e `"true"`),
  `FeaturesConfig.BuiltinMCPCallerAuth`, and one `internal/runtimeflags`
  registration, following `/runtime-feature-flags`.
- `internal/agent/runtime/agentctl`: the startup setter and
  `ControlClient.CreateInstance` send `builtin_mcp_auth_enforced`.
  `Client.BuiltinMCPCredential` reads the credential from the instance route.
- Lifecycle reads the credential per artifact build through the execution's
  agentctl client, so adoption needs no extra state.
- Credentialed entries in `builtin_mcp_passthrough.go`, `builtin_mcp_cursor.go`,
  `cursor_plugin_mcp_reconcile.go`, `project_mcp_files.go`, and
  `passthrough_remote_mcp.go`.
- The mock-agent TUI client forwards `headers` from `--mcp-config`.
- Redact headers in backend logs of MCP entries and passthrough argv.

## Out of scope

- Production promotion of the toggle.
- Admin UI beyond what the runtime-flag registry provides generically.

## Acceptance

- With the toggle on, every passthrough strategy output and Cursor or Pi project
  file contains the instance credential. The create request carries
  `builtin_mcp_auth_enforced: true`.
- A backend restart that adopts a running instance rebuilds artifacts with the
  same credential. Without a credential, it fails the relaunch instead of
  writing header-less entries.
- With the toggle off, instances do not enforce, and entries still carry the
  credential.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/runtimeflags/... ./internal/common/config/... ./internal/profiles/... ./internal/agent/runtime/agentctl/... ./internal/agent/runtime/lifecycle/... ./internal/agent/mcpconfig/... ./cmd/mock-agent/... -count=1)
(cd apps/backend && make lint)
```

## Files likely touched

- `profiles.yaml`
- `apps/backend/internal/common/config/config.go`
- `apps/backend/internal/runtimeflags/registry.go`
- `apps/backend/internal/agent/runtime/agentctl/control.go`
- `apps/backend/internal/agent/runtime/lifecycle/builtin_mcp_passthrough.go`
- `apps/backend/internal/agent/runtime/lifecycle/builtin_mcp_cursor.go`
- `apps/backend/internal/agent/runtime/lifecycle/cursor_plugin_mcp_reconcile.go`
- `apps/backend/internal/agent/runtime/lifecycle/project_mcp_files.go`
- `apps/backend/internal/agent/runtime/lifecycle/passthrough_remote_mcp.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough.go`
- `apps/backend/cmd/mock-agent/tui_mcp.go`
- Frontend flag key only if the registry requires a matching web key

## Dependencies

Task 01, Task 02

## Risks

- The Cursor fingerprint changes on each instance. Confirm that
  `cursor-agent mcp enable` re-approves without user action. Otherwise stop and
  report.
- Executor-specific create paths (Docker, SSH, Kubernetes, Sprites, plugins)
  must all pass the toggle value. Grep every `CreateInstanceRequest`
  construction.

## Parallelism

`sequential`

## Inputs

- System design: Delivery paths, Rollout control, Failure and recovery, Persistence.
- `/runtime-feature-flags` checklist.

## Results

Done 2026-11-12. Targeted verification passed; see the plan's Verification results.
