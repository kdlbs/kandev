---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-K8S-EXEC-001
  - REQ-EXECUTORS-K8S-EXEC-002
  - REQ-EXECUTORS-K8S-EXEC-003
  - REQ-EXECUTORS-K8S-EXEC-004
---

# Kubernetes Exec Stream Transport System Design

## Purpose and boundaries

The [requirements](../requirements/kubernetes-exec-transport.md) add a second
implementation behind the existing `PortForwarder` seam in
`internal/agent/kubernetes`. Callers of `startKubernetesForward`, agentctl
clients, nonce handshakes, bearer tokens, and recovery stay unchanged. The
[transport decision](../../../decisions/2026-11-27-kubernetes-exec-stream-transport.md)
records why the exec stream was chosen over other paths.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-EXECUTORS-K8S-EXEC-001 | Configuration and selection |
| REQ-EXECUTORS-K8S-EXEC-002 | Connection relay and half-close |
| REQ-EXECUTORS-K8S-EXEC-003 | Main-container targeting |
| REQ-EXECUTORS-K8S-EXEC-004 | Test Kubernetes |

## Configuration and selection

`ExecutorConfig.Transport` holds `port_forward` or `exec`.
`EffectiveTransport` maps an empty value to `port_forward`, and `Validate`
rejects every other value. The key is trusted executor connection metadata:
it is merged from the stored executor config, persisted with the runtime
record, and read back by reconnect and recovery like the other connection keys.

`newKubernetesForwarder` returns the existing port-forwarder unless the
transport is `exec`. In that case it returns an `ExecPortForwarder` that uses
the same live `PodExecutor` as bootstrap uploads and runs the uploaded
`/opt/kandev/agentctl` as the bridge.

## Connection relay and half-close

`ExecPortForwarder.Forward` binds the same process-local `127.0.0.1:0`
listener as port-forward and reports ready once it is bound. Each accepted
connection opens its own exec stream without a TTY, with stdin and stdout
bound to the accepted connection:

```text
backend client ── 127.0.0.1 listener ── pods/exec stream ── agentctl tcp-stdio ── 127.0.0.1:<port> in the Pod
```

The two directions end independently. Unlike containerd's CRI port-forward,
which does not half-close and closes both directions about one second after
either one ends
([`portForward`](https://github.com/containerd/containerd/blob/59228e1af476a2b67cd80319e7e7249842413a05/internal/cri/server/sandbox_portforward_linux.go#L83-L125)),
there is no drain deadline:

1. The backend client half-closes. Reading the accepted connection returns EOF,
   and the client-go executor closes the stream's stdin (SPDY half-close or the
   WebSocket v5 close signal) and keeps copying stdout.
2. `tcpstdio.Bridge` sees EOF on stdin and calls `CloseWrite` on its TCP
   connection, so the agentctl server reads EOF.
3. The bridge keeps copying the TCP connection to stdout until the server
   closes it. There is no drain deadline after stdin EOF.
4. The exec stream ends when the bridge exits, and the relay closes the
   accepted connection after the last response byte.

A connection also ends when the forward session closes (its context cancels
every exec stream) or when the stream fails. A failure on an open session logs
one warning with the Pod, port, duration, and a bounded, credential-redacted
stderr tail. Opens and closes log at debug level.

The in-Pod bridge has no context of its own. When the backend side is gone,
the bridge lives until agentctl closes the connection. Go's net/http does that
at once for an idle connection, and also for a handler that honours its
request context, because net/http cancels that context when the client's EOF
arrives. A handler that ignores its context keeps the bridge until it returns.

## Main-container targeting

Only the main container holds the uploaded agentctl. `AgentctlContainer` picks
the one container that owns the reserved `kandev-agentctl` container port.
`applyMainContainer` sets that port on the configured main container, template
validation rejects it on every other container, and admission rejects an
admitted Pod that adds it elsewhere. The choice therefore ignores container
order and sidecar names. `startKubernetesForward` resolves the container only
when `StreamOperations.ForwardsOverExec` is true. A Pod without exactly one
such container fails before any stream opens. Port-forward requests carry no
container, as before.

## Test Kubernetes

With `exec`, the streaming step requests only the `pods/exec` permissions and
runs `cat` in the probe Pod's main container. It sends 64 KiB of random bytes
and passes only on an exact echo. Stdin stays open until the whole echo has
arrived, or for at most 10 seconds, and only then closes. Kata Containers can
drop stdout still in flight at stdin EOF, and an early close made a working
data path report `32768 of 65536 bytes`. The handshake-only exec and port-forward
probes remain for `port_forward`.

## Verification

- `internal/agentctl/tcpstdio`: the real bridge half-closes on stdin EOF and
  delivers a delayed, chunked response in full.
- `internal/agent/kubernetes`: the forwarder with the real bridge in process
  keeps the response after the client half-closes. `AgentctlContainer` tests
  cover sidecar order, a custom main container name, and missing or duplicate markers.
- `internal/agent/runtime/lifecycle`: the live runtime client, with a fake
  apiserver that speaks WebSocket v5 and SPDY v4 and runs the real bridge,
  delivers the delayed response and opens the stream in the main container of
  a Pod whose first container is a sidecar. Create-instance tests with sidecars
  check that every bridge and bootstrap exec targets the main container.

## Delivery

See the [exec transport plan](../../../plans/kubernetes-exec-transport/plan.md).
