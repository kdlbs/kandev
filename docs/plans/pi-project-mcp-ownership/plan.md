---
created: 2026-10-08
status: complete
requirements:
  - REQ-AGENTS-MCP-PREP-005
system_design:
  - ../../specs/agents/system-design/agent-mcp-preparation.md
legacy_specs: []
---

# Implementation Plan: Pi project MCP ownership

## Overview

Repair the shared project MCP lifecycle so a successor Pi execution can publish
its current agentctl endpoint without an older execution deleting it. The work
adds a fenced ownership/generation layer around the existing passthrough
materialization and cleanup path, then persists only non-secret claim metadata
for backend restart and recovery. The existing Pi project path, merge behavior,
`streamable-http` transport, `eager` lifecycle, and managed `pi-acp` runtime
remain unchanged.

The order is deliberately one vertical slice: first pin the failing lifecycle
sequence and ownership rules with deterministic tests, then implement the
resource fence and persistence/recovery wiring, and finally run the focused
race and backend checks.

## Scope

### In scope

- Shared workspace/project MCP materialization and teardown ownership.
- Successor generation transfer during launch and workspace promotion.
- Cleanup protection for pending, active, resumed, recovered, and restarted
  executions.
- Fingerprint-based fail-closed protection for user edits, unknown files,
  malformed files, and unsafe paths.
- Regression and race coverage for Pi project configuration and existing merge
  semantics.

### Out of scope

- Changing Pi ACP or `pi-acp` managed-runtime versions.
- Writing Kandev endpoints to `~/.pi/agent/mcp.json`.
- Changing MCP server schema, profile server selection, or credentials.
- Adding a new user-facing settings or browser flow.
- Reworking Cursor's separate native MCP import ownership protocol.

## Technical approach

Add a lifecycle-owned shared-resource coordinator in
`internal/agent/runtime/lifecycle`. It serializes the complete project MCP
read/merge/write and cleanup decision, tracks per-execution claims and the
current generated fingerprint, and hydrates claims from persistent execution
metadata after recovery. Existing `materializePassthroughFile` callers keep
their strategy contract; the coordinator changes only whether a file is
tracked and whether teardown is allowed to remove it.

Update the launch, promotion, rollback, removal, and persistence boundaries so
claims exist before a successor can be cleaned up, failed unregistered launches
release their claims, and recovered executions rejoin the resource record.
Persist only path/fingerprint ownership records through the existing
session-scoped metadata allowlist. Never copy generated JSON, URLs, headers,
environment values, or credentials into ownership metadata or logs.

Keep `mcpconfig.PiStrategy` as the source of the project file path and generated
entry. Its current tests remain the contract for `<workspace>/.pi/mcp.json`,
`streamable-http`, `eager`, and profile/user merge behavior.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| AC-AGENTS-MCP-PREP-005.1 | Deterministic old/new materialization test in `manager_passthrough_mcpfiles_test.go`; Pi URL/transport/lifecycle assertions in `manager_project_mcp_test.go` and `mcpconfig/passthrough_test.go`. |
| AC-AGENTS-MCP-PREP-005.2 | User-file preservation and non-deletion tests in `manager_passthrough_mcpfiles_test.go` and `manager_passthrough_test.go`. |
| AC-AGENTS-MCP-PREP-005.3 | Last-claim cleanup, successor-claim, rollback, and old-cleanup-after-new-materialization tests with channel barriers. |
| AC-AGENTS-MCP-PREP-005.4 | Concurrent launch/cleanup race tests, fingerprint mismatch tests, restart-shaped metadata tests, and `go test -race` for the lifecycle packages. |
| AC-AGENTS-MCP-PREP-005.5 | Existing Pi strategy and managed runtime tests remain green; no test writes a user-level Pi config. |

All concurrent tests use channels or wait groups for ordering. They must not
use sleeps to create the race window.

## Work orders

- [x] [Task 01: Fence shared project MCP ownership](task-01-fence-project-mcp-ownership.md)

## Verification results

- `go test -race` focused on the new Pi ownership, cleanup ordering, recovery,
  user-file, and metadata tests passed.
- `go vet ./internal/agent/mcpconfig ./internal/agent/runtime/lifecycle` passed.
- `make fmt` and the specification/document validation passed.
- The required package test command and the backend full suite exercise the new
  tests successfully, but remain non-zero because of pre-existing unrelated
  failures in mcpconfig/lifecycle and other backend packages.
- `make lint` could not run because `golangci-lint` is not installed in the
  current mise tool set. Root `make typecheck` could not run because `pnpm` is
  unavailable in the current worktree environment.

## Risks

- A user-edited or unrecoverable project file can be intentionally left behind
  because ownership cleanup fails closed.
- Existing pre-fix executions may have only a path list and no fingerprint;
  migration must not infer ownership from that incomplete evidence.
- A single project file can expose only one current Kandev endpoint, so the
  newest materialized generation is authoritative while older Pi processes
  retain the configuration they loaded at startup.
