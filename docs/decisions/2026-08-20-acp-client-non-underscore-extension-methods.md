# ADR-2026-08-20-acp-client-non-underscore-extension-methods: Route Non-Underscore Inbound Client Methods to the Extension Handler

**Status:** accepted
**Date:** 2026-08-20
**Area:** backend, protocol

## Context

ACP reserves the `_`-prefix namespace for extension methods, and the
`kdlbs/acp-go-sdk` fork only routes `_`-prefixed inbound methods to a peer's
`ExtensionMethodHandler`; every other unrecognized method falls through the
generated `handle` switch and returns `-32601` method-not-found. Cursor breaks
that assumption: it delivers rich subagent metadata over an agent-to-client
**request** named `cursor/task` (no `_` prefix), so Kandev, acting as the ACP
client, rejects it today and drops the subagent's prompt, model, description, and
agent id. Other current and future agent CLIs are likely to introduce their own
vendor-namespaced (`vendor/method`) inbound requests for the same reason, and the
adapter needs one consistent seam to accept or decline them rather than a
per-method fork patch each time.

## Decision

Widen the SDK fork's client-side dispatch and let Kandev's ACP `Client` own the
accept/decline policy for unrecognized inbound methods.

- In `github.com/kdlbs/acp-go-sdk` `extensions.go`,
  `ClientSideConnection.handleWithExtensions` routes **any** method the generated
  `ClientSideConnection.handle` switch does not recognize to the client's
  `ExtensionMethodHandler` when the `Client` implements one, instead of only
  `_`-prefixed names. The `_`-prefix contract for the *outbound*
  `CallExtension` / `NotifyExtension` helpers (`validateExtensionMethodName`) is
  unchanged: Kandev only relaxes what it will *accept*, not what it will *send*.
  The `AgentSideConnection` path is unchanged. A new pseudo-version is published
  and the `replace` in `apps/backend/go.mod` is bumped to it.
- Kandev's `Client`
  (`apps/backend/internal/agentctl/server/acp/client.go`) implements
  `HandleExtensionMethod(ctx, method, params)`. It returns an empty success
  result only for methods it explicitly recognizes (`cursor/task` first) and
  returns `acp.NewMethodNotFound(method)` for everything else, so an unknown
  method still declines correctly rather than being silently accepted.
- Parsing and correlation of a recognized method's params stay out of the SDK and
  out of `client.go`; they live in the agent-specific dialect
  (`internal/agentctl/server/adapter/transport/acp/dialect_cursor.go`), consistent
  with [ADR 0044](0044-acp-agent-compatibility-dialects.md).

## Consequences

- Kandev can accept vendor-namespaced inbound requests from any ACP agent CLI
  through one client-owned allowlist, without a fork patch per method.
- The accept/decline decision is centralized and explicit: an unrecognized method
  still returns method-not-found, so the change does not blanket-accept unknown
  traffic.
- The fork now diverges further from upstream `coder/acp-go-sdk` on inbound
  client dispatch, so the divergence must be re-checked on every SDK rebase and
  is recorded here as the reason.
- The relaxation is asymmetric (inbound accept only); outbound sends still refuse
  non-`_` names, preserving spec-compliant behavior toward peers.

## Alternatives Considered

### Wrap the transport instead of touching the fork (plan Option B)

Interpose a reader between agent stdout and `NewClientSideConnection` that peeks
frames, replies to `cursor/task` out-of-band on the shared writer, and passes
everything else through. Rejected: it duplicates the SDK's JSON-RPC framing and
response correctness, adds a second code path that must stay in lockstep with the
SDK, and is higher risk than a small, well-scoped fork change we already own.

### Ask Cursor to use a `_`-prefixed method

Not actionable: `cursor/task` is Cursor's wire contract and Kandev does not
control it. The client must adapt to observed agent behavior.

### Handle `cursor/task` with a hardcoded branch in the SDK fork

Rejected: it embeds one vendor's method name in a protocol library and forces a
fork bump for the next agent that does the same. Routing unrecognized methods to
the client keeps vendor knowledge in Kandev's dialect layer.
