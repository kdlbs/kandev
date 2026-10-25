---
id: "02"
title: "Broker relay"
status: todo
wave: 2
depends_on: ["01"]
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-002
acceptance_criteria:
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.1
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.2
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.3
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.4
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.5
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.6
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.7
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.8
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.9
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.10
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.11
  - AC-EXECUTORS-SESSION-CONTROL-ISOLATION-002.12
system_design:
  - ../../specs/executors/system-design/session-control-plane-isolation.md
---

# Task 02: Broker relay

## Summary

Relay the credential broker through the instance listener for same-host executors.

## Scope

- Relay handler for resolve and reissue, upstream URL in the create request alongside the unchanged broker URL environment value, agentctl effective broker URL for every process-visible use, readiness probe limits (20 s curl, 25 s wait).

## Exclusions

Remote executor transport; lease semantics.

## Acceptance

1. Same-host agents see only the instance URL; remote executors still require HTTPS.
2. Limits, timeouts, no-retry, caller-cancel and method/path rules hold; reissue works through the relay.
3. Managed-launch signals (readiness probe, credential snapshot, helper activation) still fire.

## Verification

`(cd apps/backend && go test ./internal/agentctl/... ./internal/orchestrator/executor/... ./internal/backendapp/... -count=1)`

## Likely files

internal/backendapp/orchestrator.go; orchestrator/executor/executor_credentials.go; agent/runtime/lifecycle/broker_reachability.go; agentctl/server/api.
