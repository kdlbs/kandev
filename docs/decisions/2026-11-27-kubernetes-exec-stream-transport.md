# ADR-2026-11-27-kubernetes-exec-stream-transport: Kubernetes Exec Stream Transport

**Status:** accepted
**Date:** 2026-11-27
**Area:** backend, infra, security

## Context

The [resource ownership decision](2026-08-24-kubernetes-executor-resource-ownership.md)
reaches agentctl only through a process-local `127.0.0.1` port-forward and
lists port-forward and exec streaming as required capabilities. Runtimes that
run each Pod in a VM or a user-space kernel do not implement the CRI
`PortForward` call: Kata Containers (kata-containers/kata-containers#1693) and
gVisor (google/gvisor#3811). The generic containerd change
(containerd/containerd#14074) is still open. On such nodes bootstrap succeeds
over `pods/exec`, and every control connection then fails until the launch
times out. Those runtimes are the usual way to give agents Docker-in-Docker or
untrusted-code isolation, so the executor could not use them.

The direction was agreed with the maintainers in kdlbs/kandev#5505.

## Decision

Add an opt-in executor connection setting `transport: port_forward | exec`,
defaulting to `port_forward`. With `exec`, a second `PortForwarder` keeps the
existing contract: it binds the same process-local loopback listener and
carries each accepted connection over its own `pods/exec` stream, without a
TTY, into `agentctl tcp-stdio --port <port>` in the Pod's main container. The
bridge dials only Pod loopback. The two directions of a connection end
independently, as with kubectl port-forward: end of the request half-closes
the TCP write side, and the response is relayed until agentctl closes.

For executors that select it, exec streaming is the only required streaming
capability. Port-forward stays required for the default transport.

## Consequences

- agentctl stays bound to Pod loopback. No Service, Ingress, NetworkPolicy
  path, or Pod-spec credential is added, and the apiserver still authenticates
  every stream with the executor's existing `pods/exec` permission.
- Nonce handshake, bearer tokens, reconnect, and recovery are unchanged,
  because forwards were already process-local and are recreated after restart.
- Each new TCP connection costs one exec round trip. HTTP keep-alive and the
  long-lived WebSockets amortize it.
- The bridge must target the main container. Kandev's reserved
  `kandev-agentctl` port, enforced by template validation and admission, marks
  it independent of container order.
- **Test Kubernetes** must prove a data path for `exec`. Its streaming step
  sends random bytes through an exec stream and requires an exact echo.

## Alternatives Considered

- **Wait for CRI port-forward in Kata, gVisor, or containerd:** rejected
  because those changes have been open for years with no delivery date.
- **A Service, NodePort, or Pod IP route to agentctl:** rejected because it
  breaks the loopback-only invariant and adds a network surface and policy.
- **agentctl dialing out to the backend:** rejected for in-cluster Pods
  because it reverses the trust direction and requires a reachable backend
  endpoint. It remains an option for executors outside the cluster.
- **One multiplexed exec stream per Pod:** deferred. It saves round trips but
  needs a framing protocol and its own flow control. The per-connection stream
  keeps the bridge a plain byte copy.
