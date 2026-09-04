---
status: current
system: agents
requirements:
  - REQ-AGENTS-MCP-BRIDGE-RELIABILITY-001
created: 2026-09-04
owners:
  - Kandev
---

# MCP Bridge Reliability System Design

## Purpose and boundaries

This design makes the internal MCP bridge fail with a correlated error when
delivery is not possible. It also removes a startup race for recovered agent
streams.

The bridge does not set a common deadline for backend operations. Some tools
wait for a person, and agent launch can use a 15-minute budget. Each business
operation keeps its own context and deadline.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-AGENTS-MCP-BRIDGE-RELIABILITY-001` | [Current dispatcher binding](#current-dispatcher-binding), [Request ownership](#request-ownership), [Failure behavior](#failure-behavior), [Observability](#observability) |

## Confirmed fault

`provideAgentRuntime` calls `lifecycle.Manager.Start` before route registration
calls `SetMCPHandler`. Startup recovery starts reconnect work during this gap.

`StreamManager.connectUpdatesStream` passes the current `mcpHandler` value to
`Client.StreamUpdates`. The read loop keeps that interface value for the life
of the stream. A stream that starts during the gap therefore keeps a `nil`
handler after startup finishes.

The read loop silently ignores a request when its handler is `nil`. Agentctl
then waits on `ChannelBackendClient.pending` until the calling MCP client ends
the request.

Two other bridge states can lose a response. The buffered request channel can
accept a request without a stream consumer. A stream can also close after its
writer removes a request from the channel.

## Components and responsibilities

- `internal/mcp/server.ChannelBackendClient` owns agentctl request state and
  correlation.
- `internal/agentctl/server/api.Server` owns the agentctl side of each backend
  stream.
- `internal/agent/runtime/agentctl.Client` reads backend requests and writes
  their responses.
- `internal/agent/runtime/lifecycle.StreamManager` supplies the dispatcher and
  server-derived execution scope for each backend stream.

## Current dispatcher binding

`StreamManager` stores the dispatcher and its scope functions behind one
read-write mutex. Manager setter methods update that state through a
`StreamManager` method.

`mcpHandlerFor` returns a stable execution-bound proxy. The proxy reads the
current dispatcher and scope functions for each request. Thus a stream that
starts before route registration can use the dispatcher after registration.

If no dispatcher exists when a request arrives, the proxy returns a typed
unavailable error. `dispatchMCPRequest` converts that error to a correlated
WebSocket error response.

`readUpdatesStream` also handles a direct `nil` handler defensively. It writes
a correlated error response instead of skipping the request. This path covers
callers that do not use `StreamManager`.

If a dispatcher returns both a `nil` response and a `nil` error for a request,
`dispatchMCPRequest` writes a correlated internal error. WebSocket
notifications remain separate from request messages and do not require a
response.

## Request ownership

`ChannelBackendClient.requestCh` becomes unbuffered. A send succeeds only when
an active stream writer accepts the request. The existing five-second send
limit therefore detects the absence of a consumer.

Each pending request stores a result channel and an optional backend-stream
identifier. The stream writer binds the request to its identifier after a
successful WebSocket write.

If the write fails, the writer completes that request with a delivery error.
When the stream ends, the API server completes only requests bound to that
stream. A replacement stream can overlap with an old stream without losing its
requests.

Session reset still completes all pending requests with the existing reset
error. Client close still rejects new requests and releases all current waits.

## Control flow

1. The local MCP handler registers a pending request before it sends the
   request to `requestCh`.
2. An active stream writer accepts the unbuffered request.
3. The writer sends the request and binds it to the stream identifier.
4. The backend read loop records receipt and resolves the current dispatcher.
5. The dispatcher returns one correlated response or error.
6. Agentctl completes the matching pending request.

If steps 2 through 5 cannot finish, the owning component completes the pending
request with the applicable transport error.

## Failure behavior

| Condition | Result |
| --- | --- |
| No active stream writer | The existing send timer returns an error within five seconds. |
| No configured dispatcher | The backend returns a correlated unavailable error. |
| Dispatcher returns no response | The backend returns a correlated internal error. |
| WebSocket write fails | Agentctl completes that request with a delivery error. |
| Owning backend stream closes | Agentctl completes requests bound to that stream with a disconnect error. |
| Request context ends | `RequestPayload` returns the context error. |
| Session resets | Pending requests return the existing session-reset error. |

This design does not add a common response deadline. Such a deadline can stop
valid clarification waits and long launch operations. Transport ownership and
caller contexts provide the required termination signals.

## Observability

The backend records one request-received event at `Info`. The event includes
the action, request ID, session ID, and pending ID.

Unavailable dispatch, empty response, write failure, send timeout, and stream
disconnect events use `Warn` or `Error`. Each event includes the action and
request ID when those values exist. The bridge does not record request payloads
or tool arguments at these levels.

Existing trace spans continue to measure dispatcher duration and response
outcome.

## Testing strategy

Unit tests cover these cases:

- A handler proxy created before `SetMCPHandler` uses the later dispatcher and
  retains execution scope.
- A direct stream with a `nil` handler returns a correlated error frame.
- A dispatcher with no response returns a correlated error frame.
- A request without a stream consumer cannot enter the request channel.
- A stream write error releases the matching request.
- A stream disconnect releases only requests owned by that stream.
- Successful responses, context cancellation, reset, close, and clarification
  waits keep their current behavior.

Race-enabled tests cover concurrent dispatcher updates, request completion,
stream replacement, reset, and close.
