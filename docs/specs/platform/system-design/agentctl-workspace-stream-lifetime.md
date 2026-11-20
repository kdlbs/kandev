---
status: current
system: platform
requirements:
  - REQ-PLATFORM-AGENTCTL-WORKSPACE-STREAM-LIFETIME-001
created: 2026-10-20
owners:
  - kandev
---

# Agentctl Workspace Stream Lifetime Design

## Boundary and mapping

| Criteria | Implementation boundary |
| -------- | ----------------------- |
| .1       | `api.Server.handleWorkspaceStreamWS` and `forwardWorkspaceStream` in `agentctl/server/api/workspace.go` |
| .2       | The unchanged exit arms of `forwardWorkspaceStream` and `handleWorkspaceStreamInput` |

## Goroutine ownership

`handleWorkspaceStreamWS` owns two loops on one hijacked connection.

- The input loop (`handleWorkspaceStreamInput`) is the only reader. A read
  error is how a departed client shows up on this side.
- The forwarding loop (`forwardWorkspaceStream`) runs on the handler goroutine
  and selects on the handler's `done`, credential invalidation, the workspace
  subscription, and shell output.

Before this design, a read error made the input loop return silently. `done`
is closed only by the handler's own deferred return, which waits on the
forwarding loop, so it could not end that loop. The subscription stays open
while the instance's process manager lives, and `http.Server.Shutdown` does not
close hijacked connections. After an instance stopped, nothing ever fired, and
the parked handler pinned the instance.

The handler now runs the input loop through a wrapper that closes an
`inputDone` channel when the loop returns, for any reason. The forwarding loop
selects on `inputDone` and returns, so the handler's deferred unsubscribe and
socket close run. No new lock, timer, or teardown hook is added.

## Verification

The disposable API check dials the real `/api/v1/workspace/stream` endpoint, reads
the greeting, closes the client, publishes nothing, and requires the handler
to return within a bound. The existing package `goleak` check covers stray
goroutines. The existing stream tests cover delivery, ping and pong, and
credential rotation. No process is started.
