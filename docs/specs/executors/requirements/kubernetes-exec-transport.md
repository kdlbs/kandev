---
status: active
system: executors
created: 2026-11-27
owners:
  - kandev
---

# Kubernetes Exec Stream Transport Requirements

## Overview

Sandboxed runtimes that run each Pod in a VM or a user-space kernel, such as
Kata Containers and gVisor, do not implement the CRI `PortForward` call. On
those nodes the backend can bootstrap a session Pod over `pods/exec` but never
reaches agentctl over `pods/portforward`. Administrators need an opt-in way to
carry the same loopback connections over `pods/exec`. Existing executors keep
port-forward unchanged.

## Requirements

### REQ-EXECUTORS-K8S-EXEC-001: Opt-in exec stream transport

**Intent:** An administrator can run Kubernetes sessions on runtimes without
CRI port-forward support, and every existing executor behaves as before.

#### Acceptance criteria

- **AC-EXECUTORS-K8S-EXEC-001.1:** The executor connection configuration
  shall accept `transport` with the values `port_forward` and `exec`. An absent
  value shall mean `port_forward`, and any other value shall fail validation.
- **AC-EXECUTORS-K8S-EXEC-001.2:** With `exec`, every backend connection to
  agentctl in a session Pod shall travel over its own `pods/exec` stream into
  `agentctl tcp-stdio`, and the backend shall open no `pods/portforward` stream.
- **AC-EXECUTORS-K8S-EXEC-001.3:** The transport shall be connection metadata
  of the running session, so reconnect, recovery, and restart use the
  transport the session was launched with.
- **AC-EXECUTORS-K8S-EXEC-001.4:** With `port_forward`, connection requests,
  required permissions, and diagnostics shall stay unchanged.

### REQ-EXECUTORS-K8S-EXEC-002: Request and response stay independent

**Intent:** A request that ends before its response starts still receives the
whole response, as it does over port-forward.

#### Acceptance criteria

- **AC-EXECUTORS-K8S-EXEC-002.1:** When the backend client half-closes a
  connection, the in-Pod bridge shall half-close its TCP write side, so the
  agentctl server observes end of input.
- **AC-EXECUTORS-K8S-EXEC-002.2:** After the request side ends, the backend
  relay and the in-Pod bridge shall keep delivering response bytes until the
  agentctl server closes the connection, with no drain deadline. A response
  that starts after a pause and arrives in several chunks shall arrive in full.
- **AC-EXECUTORS-K8S-EXEC-002.3:** A connection shall end when the agentctl
  server closes it, when the forward session closes, or when its exec stream
  fails. A failed stream on an open session shall produce one sanitized warning.

### REQ-EXECUTORS-K8S-EXEC-003: Main-container targeting and loopback invariant

**Intent:** The exec transport cannot widen the control surface or run the
bridge in a container that does not hold agentctl.

#### Acceptance criteria

- **AC-EXECUTORS-K8S-EXEC-003.1:** Every bridge stream shall open in the Pod's
  main container, the one that runs the Kandev bootstrap and owns the reserved
  `kandev-agentctl` port, whatever its name and its position among sidecars.
- **AC-EXECUTORS-K8S-EXEC-003.2:** A Pod without exactly one such container
  shall fail the connection with an error that names the missing main
  container, before any exec stream opens.
- **AC-EXECUTORS-K8S-EXEC-003.3:** The bridge shall dial only `127.0.0.1`
  inside the Pod. No Service, Ingress, Pod-spec credential, or extra RBAC verb
  beyond the executor's existing `pods/exec` permission shall be added.

### REQ-EXECUTORS-K8S-EXEC-004: Data-path validation

**Intent:** **Test Kubernetes** cannot pass for a transport that cannot carry
bytes.

#### Acceptance criteria

- **AC-EXECUTORS-K8S-EXEC-004.1:** With `exec`, the streaming step shall check
  only the exec permissions and shall pass only after random bytes sent
  through an exec stream in the probe Pod return unchanged.
- **AC-EXECUTORS-K8S-EXEC-004.2:** Public executor documentation shall describe
  the setting, its default, when to use it, its per-connection cost, and that
  the settings form does not show it yet.

## Out of scope

- A settings-form control for `transport`; it is API-only in this package.
- Multiplexing several connections over one exec stream.
- Changing the port-forward probe or the port-forward transport.
- Runtime detection that picks a transport automatically.
