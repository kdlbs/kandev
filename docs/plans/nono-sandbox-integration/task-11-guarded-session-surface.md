---
id: "11-guarded-session-surface"
title: "Enforce the MCP surface for guarded sessions"
status: in_progress
wave: 9
depends_on:
  - "04-policy-authority"
  - "06-approval-channel"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SANDBOX-009
  - REQ-AGENTS-SANDBOX-010
acceptance_criteria:
  - AC-AGENTS-SANDBOX-009.1
  - AC-AGENTS-SANDBOX-009.2
  - AC-AGENTS-SANDBOX-009.3
  - AC-AGENTS-SANDBOX-009.4
  - AC-AGENTS-SANDBOX-009.5
  - AC-AGENTS-SANDBOX-009.6
  - AC-AGENTS-SANDBOX-009.7
  - AC-AGENTS-SANDBOX-009.8
  - AC-AGENTS-SANDBOX-009.9
  - AC-AGENTS-SANDBOX-009.10
  - AC-AGENTS-SANDBOX-009.11
  - AC-AGENTS-SANDBOX-009.12
  - AC-AGENTS-SANDBOX-010.1
  - AC-AGENTS-SANDBOX-010.2
  - AC-AGENTS-SANDBOX-010.3
  - AC-AGENTS-SANDBOX-010.4
  - AC-AGENTS-SANDBOX-010.5
  - AC-AGENTS-SANDBOX-010.6
system_design:
  - ../../specs/agents/system-design/sandbox-guarded-session-surface.md
---

# Task 11: Enforce the MCP surface for guarded sessions

## Summary

Add a server-side MCP surface for sessions whose task holds a sandbox task guard, and stop auto-approving their mutating Kandev tools. Child-task confinement itself is already delivered by task guards; this work order covers only what that model leaves open.

## Scope

- `sandbox.Service.TaskGuarded` and `scope.GuardSource`, wired in `backendapp` for both resolver construction sites.
- Dispatcher guard, payload canonicalization, and classification table under `internal/mcp`.
- agentctl approval policy for sandboxed instances.

## Validation

- `go test ./internal/mcp/... ./internal/agentctl/server/process -run Guard` and `./internal/agent/sandbox -run TaskGuarded`.
- `bash scripts/lint-go-changed <changed go files>`.
- Failures in `internal/agent/sandbox` task-guard store tests (`sandbox policy is invalid`) reproduce on the merge-base `777b59d6f3`; they are not caused by this work.

## Result

Implemented in the same branch as the spec. Not run: PostgreSQL suites (no DSN).
