---
created: 2026-11-27
status: implemented
requirements:
  - REQ-EXECUTORS-K8S-EXEC-001
  - REQ-EXECUTORS-K8S-EXEC-002
  - REQ-EXECUTORS-K8S-EXEC-003
  - REQ-EXECUTORS-K8S-EXEC-004
system_design:
  - ../../specs/executors/system-design/kubernetes-exec-transport.md
legacy_specs:
  - ../../specs/kubernetes-executor/spec.md
---

# Implementation Plan: Kubernetes Exec Stream Transport

## Overview

Let the Kubernetes executor reach agentctl on runtimes without CRI
port-forward support by carrying each loopback connection over `pods/exec`.
The setting is opt-in, and port-forward stays the default. The direction was
agreed in kdlbs/kandev#5505.

## Scope

### In scope

- The `transport` connection key, validation, persistence, and reconnect.
- The `agentctl tcp-stdio` bridge and the backend `ExecPortForwarder`.
- Main-container targeting among sidecars.
- The data-path check in **Test Kubernetes** for `exec`.
- Public executor documentation and the transport decision record.

### Out of scope

- A settings-form control for `transport` and its locales.
- Multiplexing connections over one exec stream, and runtime auto-detection.

## Technical approach

Add `ExecPortForwarder` behind the existing `PortForwarder` seam, so
`startKubernetesForward` and every agentctl client stay unchanged. Put the
bridge in a standard-library-only package, `internal/agentctl/tcpstdio`, which
agentctl runs as a subcommand before configuration loading and which tests
import directly. Resolve the target container from the Kandev-reserved
`kandev-agentctl` port, and only for the exec transport, so port-forward
requests stay unchanged.

## Tests

- `internal/agentctl/tcpstdio/tcpstdio_test.go`:
  `TestRunPreservesResponseAfterRequestEnds` (EXEC-002.1, 002.2),
  `TestRunExitsWhenPeerClosesWhileStdinStaysOpen`, and
  `TestBridgeStopsWhenStdoutFails` (EXEC-002.3), plus argument and dial
  failure tests (EXEC-003.3).
- `internal/agent/kubernetes`:
  `TestExecPortForwarderPreservesResponseAfterRequestEnds` (EXEC-002.2),
  `TestAgentctlContainerSelectsMainContainerAmongSidecars` and
  `TestAgentctlContainerFailsWithoutExactlyOneMainContainer` (EXEC-003.1,
  003.2), and the forwarder and logging tests (EXEC-001.2, 002.3).
- `internal/agent/runtime/lifecycle`:
  `TestKubernetesExecTransportPreservesResponseAfterRequestEnds` runs the live
  client against a fake apiserver over WebSocket v5 and SPDY v4 with the real
  bridge (EXEC-002, 003.1).
  `TestKubernetesExecTransportTargetsMainContainerAmongSidecars` and
  `TestStartKubernetesForward*` (EXEC-003.1, 003.2, 001.4), plus config and
  metadata tests (EXEC-001.1, 001.3).
- `internal/kubernetes/streaming_transport_test.go`: the exec data-path probe
  and its permissions (EXEC-004.1).

## E2E tests

The Kind E2E clusters run runc, which supports port-forward, so they cannot
show the failure this package fixes. Unit tests drive the real client-go
stream protocols against a fake apiserver instead. An earlier revision of this
change, which still had a 10-second drain deadline after the end of the request
and chose the container by its bootstrap command, ran on Kata Containers 3.32
(Cloud Hypervisor) on Kubernetes 1.34. There a Docker-in-Docker task and a
backend restart mid-turn passed. No UI change requires new web E2E tests.

## Work orders

- [x] [Task 01: Exec stream transport](task-01-exec-stream-transport.md)

## Verification results

Implemented. Each new half-close test fails when either relay ends the
connection at the end of the request. Each sidecar test fails when the first
container is chosen.

## Risks

- A UI save of the executor connection drops the API-only `transport` key and
  returns the executor to port-forward until the settings field lands.
- Each connection costs one exec round trip. An agentctl client that opens
  many short connections becomes slower than over port-forward.
