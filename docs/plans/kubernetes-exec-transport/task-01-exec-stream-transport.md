---
id: "01-exec-stream-transport"
title: "Carry agentctl connections over pods/exec"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-K8S-EXEC-001
  - REQ-EXECUTORS-K8S-EXEC-002
  - REQ-EXECUTORS-K8S-EXEC-003
  - REQ-EXECUTORS-K8S-EXEC-004
acceptance_criteria:
  - AC-EXECUTORS-K8S-EXEC-001.1
  - AC-EXECUTORS-K8S-EXEC-001.2
  - AC-EXECUTORS-K8S-EXEC-001.3
  - AC-EXECUTORS-K8S-EXEC-001.4
  - AC-EXECUTORS-K8S-EXEC-002.1
  - AC-EXECUTORS-K8S-EXEC-002.2
  - AC-EXECUTORS-K8S-EXEC-002.3
  - AC-EXECUTORS-K8S-EXEC-003.1
  - AC-EXECUTORS-K8S-EXEC-003.2
  - AC-EXECUTORS-K8S-EXEC-003.3
  - AC-EXECUTORS-K8S-EXEC-004.1
  - AC-EXECUTORS-K8S-EXEC-004.2
system_design:
  - ../../specs/executors/system-design/kubernetes-exec-transport.md
---

# Task 01: Carry agentctl Connections over pods/exec

## Summary

Add the opt-in `exec` transport to the Kubernetes executor, with the
`agentctl tcp-stdio` bridge, main-container targeting, the data-path check
in **Test Kubernetes**, and public documentation.

## In scope

- `internal/agentctl/tcpstdio` and its agentctl subcommand dispatch.
- `ExecutorConfig.Transport`, `ExecPortForwarder`, `AgentctlContainer`, and
  `StreamOperations.ForwardsOverExec` in `internal/agent/kubernetes`.
- Transport selection, connection metadata, and container resolution in
  `internal/agent/runtime/lifecycle` and `internal/task/service`.
- The exec probe and its permissions in `internal/kubernetes`.
- `docs/public/k8s.md` and the transport decision record.

## Out of scope

- The settings-form field, connection multiplexing, and auto-detection.

## Acceptance

- Absent or `port_forward` configs behave exactly as before.
- With `exec`, a delayed, chunked response after the request ends arrives in
  full through the live client over WebSocket v5 and SPDY v4.
- With sidecars in any position, every exec stream opens in the configured
  main container. A Pod without exactly one marked main container fails
  before any stream opens.

## Verification

From `apps/backend`:

```bash
go test -trimpath ./internal/agentctl/tcpstdio ./cmd/agentctl ./internal/agent/kubernetes ./internal/kubernetes -count=1
go test -trimpath ./internal/agent/runtime/lifecycle -run 'Kubernetes|Exec|Forward' -count=1
go vet ./internal/agentctl/tcpstdio ./internal/agent/kubernetes ./internal/agent/runtime/lifecycle ./internal/kubernetes
```
