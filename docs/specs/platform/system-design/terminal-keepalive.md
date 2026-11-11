---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-TERMINAL-KEEPALIVE-001
---

# Terminal connection keepalive design

## Purpose and boundaries

Platform supplies one connection policy for browser terminal WebSockets. The
gateway and login PTY handlers use that policy after their existing admission
checks. This design satisfies the
[terminal keepalive requirement](../requirements/terminal-keepalive.md).

The [task terminal design](../../tasks/system-design/task-terminal-persistence.md)
retains descriptor ownership, launch authorization, and shell lifetime.
The main application WebSocket retains its existing read and write pumps.
Backend-to-agentctl streams retain their existing control-frame behavior.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-PLATFORM-TERMINAL-KEEPALIVE-001` | Shared policy, attachment coverage, connection ownership, failure and recovery, verification |

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| New `internal/common/wskeepalive/keepalive.go` | Timing defaults, terminal ping loop, pong deadline, idempotent stop and join |
| `internal/gateway/websocket/client.go` | Consume shared timing constants through its existing pumps |
| `internal/gateway/websocket/terminal_handler.go` | Initialize private, per-handler keepalive timings |
| `internal/gateway/websocket/terminal_pumps.go` | Attach keepalive to local shell, local passthrough, and browser side of the remote bridge |
| `internal/agent/loginpty/handlers.go` | Attach keepalive to the shared login, host-shell, and Quick Terminal stream |
| `components/task/ws-reconnect.ts` in the web app | Retain existing retry, reset, and replay behavior after genuine failure |

## Shared policy

Move the existing gateway timing values into the common package:

- Write timeout: 10 seconds.
- Pong timeout: 60 seconds.
- Ping interval: 54 seconds, or nine tenths of the pong timeout.

The gateway's existing `writeWait`, `pongWait`, and `pingPeriod` constants can
reference these shared values. Preserve its queued writer and admission behavior.
Do not attach a second heartbeat to the main gateway connection.

The terminal helper accepts a Gorilla connection and timing values. It returns
an idempotent stop function and any setup error. Setup installs the first read
deadline and a pong handler before the reader starts. Only a received pong
renews that deadline. Ordinary input, resize messages, and output do not renew it.

Each terminal handler stores private timing values initialized by its constructor.
Tests can supply shorter intervals per handler. These values are internal test
seams, with no environment variables, runtime flags, or public settings.
Reject invalid intervals before creating a ticker. Production callers always
use the shared defaults.

## Attachment coverage

Install the helper at the three gateway bridge boundaries and the shared login
stream. These boundaries cover every successful, long-lived browser attachment.

| Browser route or boundary | Terminal source | Keepalive owner |
| --- | --- | --- |
| `/terminal/environment/:id?terminalId=...` | Local task, fixed, or script shell | `runUserShellBridge` |
| `/terminal/session/:id?mode=agent` | Local agent passthrough | `runTerminalBridge` |
| Both gateway routes through a remote stream | Remote shell or passthrough | Browser connection in `bridgeBinaryWebSockets` |
| `/api/v1/agent-login/sessions/:id/stream` | Agent login, host shell, Quick Terminal | `Handlers.handleStream` |

Remote executor differences stay behind existing stream acquisition. The helper
does not infer provider support or open another upstream connection. Existing
unsupported-executor errors remain unchanged.

Missing-environment and completed-command routes send a close frame immediately.
They do not start a keepalive loop. HTTP failures before upgrade retain their
existing status, authorization, and origin behavior.

## Concurrent reads and writes

The helper sends pings with Gorilla `WriteControl`, with an absolute write
deadline. Gorilla permits this operation concurrently with data writes and
`Close`. A separate goroutine must not use `WriteMessage` or `SetWriteDeadline`
on a terminal connection that already has a data writer.

Install read deadlines and the pong handler before the existing reader starts.
The pong callback then runs in that reader. Do not create another reader or
change the gateway's `wsWriter` mutex and output ordering.

The ping loop must observe cancellation on every wait. Output traffic must not
reset or starve its ticker. Use one loop per browser connection, without a
shared cancellation channel or process-wide timer mutation.

## Connection ownership

Every successful start has one owning bridge or handler. Its stop function
stops the ticker and joins the goroutine. Repeated stop calls are safe.
Connection closure must precede a join that can wait for a blocked control write.

A ping write failure closes the browser connection to unblock its reader and
any pending data write. A read deadline follows the same existing cleanup path.
Failure to install the initial deadline closes the new connection and returns
without leaving a loop or subscription behind.

Local gateway cleanup retains `ClearDirectOutputBySession` or
`ClearUserShellDirectOutput`. Close the socket before output detachment or
`wsWriter.Close` can acquire a writer mutex. This unblocks any pending data
write before cleanup waits on that writer. Closing a terminal stream must not
call process stop or consume another launch command.

The remote bridge closes both connections and joins both relay goroutines when
either direction ends. Stop and join its keepalive before returning. Only the
browser-facing connection receives this new policy.

For login streams, install keepalive immediately after upgrade, before buffered
output or subscription setup. Completion of the writer closes the connection
after its final exit message. This unblocks the reader. Reader failure closes the
connection before joining the writer, so an idle or blocked writer can drain.
The existing disconnect signal and deferred unsubscribe remain single-owner
operations. Join the heartbeat and writer before handler completion.

Heartbeat frames never pass through PTY `Write`, output buffers, activity
notifications, or session supervision. Agent-login inactivity and hard limits
therefore retain their existing meaning.

## Failure and recovery

A responsive browser handles control frames without application changes.
Desktop task panels, phone Terminal navigation, and Quick Terminal keep their
existing composition and selected PTY. No layout, copy, or localization changes
are required.

True network loss, suspension beyond the pong deadline, or an unavailable peer
still closes the stream. Existing reconnect logic then resets the terminal
before replaying buffered output. Removing that reset without a replay cursor
would duplicate history and belongs to a separate design.

The 54-second interval addresses the reported approximately 90-second timeout
and a conventional 60-second proxy timeout. Shorter proxy limits, absolute age
limits, and long scheduling pauses remain deployment constraints.

## Data, security, and observability

There are no migrations, persisted fields, new application messages, or new
permission checks. Existing route admission remains authoritative.
Keepalive payloads contain no terminal, task, or session data.

Use existing connection and read/write failure logs. Do not log every ping,
terminal output, credentials, or new high-cardinality metrics.
The public WebSocket reference gains a terminal-specific scope note when the
implementation ships. Its JSON-envelope limits remain specific to the main socket.

## Verification

Helper tests pin production defaults, pong-only renewal, bounded control-write
failure, and idempotent cleanup. Use `testing/synctest` with a narrow fake
connection where feasible. Add a leak-checking `TestMain` for the new package.

Real WebSocket tests cover all three gateway bridges and `handleStream`.
They observe control pings through an active peer reader, return pongs, and
prove continued binary/text delivery after several cycles. One peer suppresses
pongs while another remains responsive. PTY output cannot keep the silent peer
alive. Cover disconnect, write failure, upstream exit, and subscription cleanup.

A test-owned TCP proxy enforces inactivity on bytes from the terminal backend.
It forwards the actual upgraded byte stream, including control frames.
It must not answer pings itself or count unrelated main-socket traffic.
Use per-handler timings with `ping interval < proxy timeout < pong timeout`.
Hold the connection for at least three proxy windows, then send input and
verify output through the same attachment. The unfixed handler fails during
the first idle window. Close and join every proxy connection during cleanup.

Existing desktop and phone terminal E2E tests verify visible shell operation,
selection, and reconnect compatibility. They complement the proxy regression
and do not substitute for its idle-connection evidence.

## Decisions and delivery

No new ADR is required. The design applies the established gateway policy
without changing transport ownership or the application protocol. The design
records the local rationale and the deferred replay alternative.

See the [fix plan and work order](../../../plans/terminal-keepalive/plan.md).
