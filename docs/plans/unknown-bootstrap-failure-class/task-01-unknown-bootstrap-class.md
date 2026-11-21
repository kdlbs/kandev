---
id: "01-unknown-bootstrap-class"
title: "Record a bounded class for unknown bootstrap failures"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
  - REQ-AGENTS-UNKNOWN-BOOTSTRAP-CLASS-001
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.25
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.26
  - AC-AGENTS-UNKNOWN-BOOTSTRAP-CLASS-001.1
  - AC-AGENTS-UNKNOWN-BOOTSTRAP-CLASS-001.2
system_design:
  - ../../specs/agents/system-design/session-startup-failure-explanations.md
---

# Task 01: Record a bounded class for unknown bootstrap failures

## Summary

Return a typed agentctl start rejection, derive a bounded class for every unknown
bootstrap failure, and project that class as the persisted cause.

## In scope

- `StartRejectedError` in `agentctl/client.go` (AC-001.1).
- `ClassifyUnknownBootstrapError` and `BootstrapFailure.Class` in
  `lifecycle/bootstrap_failure.go` (AC-001.1).
- The `cause=<class>` projection and the sanitized agentctl summary in
  `executor/launch_failure.go` (AC-001.2).

## Verification

- `go test ./internal/agent/runtime/agentctl/ -run Start`
- `go test ./internal/agent/runtime/lifecycle/ -run 'Bootstrap|Startup'`
- `go test ./internal/orchestrator/executor/ -run 'LaunchFailure|Bootstrap|StartFailure'`
