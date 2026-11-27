---
status: current
system: tasks
requirements:
  - REQ-TASKS-TERMINAL-TARGET-001
---

# Terminal target lifecycle design

## Purpose and boundaries

Task environments own ordinary shell destinations. Lifecycle resolves the
authorized environment; the terminal gateway translates a missing destination
into a terminal transport outcome; the shared browser terminal owns retries
and visible connection state.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-TASKS-TERMINAL-TARGET-001 | [Connection outcome](#connection-outcome), [Browser state](#browser-state) |

## Connection outcome

`GetOrEnsureExecutionForEnvironment` preserves the wrapped
`ErrTaskEnvironmentNotFound` sentinel. On that error, the gateway returns HTTP
404 for ordinary HTTP requests. For a WebSocket request it performs the normal
origin-checked upgrade and sends close code 1000 with reason
`environment_not_found`, then closes without creating a process.

Browsers conceal the HTTP status of failed WebSocket handshakes. Returning 404
alone would remain indistinguishable from temporary connection failure to the
existing browser retry loop. An explicit close reason reuses the existing
completed-initial-command transport pattern and exposes no shell data.

Other lookup and readiness failures keep their existing 503 and retry behavior.
Authentication and environment access checks still precede this path. No
persistence schema, environment identity, or recovery-block policy changes.

## Browser state

`ws-reconnect` stops only on the exact missing-environment close code/reason or
the existing completed-initial-command outcome. It passes the close event to
the terminal connection state. Abnormal closes and other normal-close reasons
retain exponential backoff.

`PassthroughTerminal` tracks the unavailable environment identity separately
from the connected identity. A confirmed missing destination disables its
connection effect and uses the existing localized ended overlay, including
when cached workspace readiness says ready. A different environment starts
with ordinary connection behavior. Existing socket-identity guards reject
late callbacks from a replaced connection.

Desktop and phone reuse this state and overlay. Existing panel composition,
scrolling, keyboard controls, and touch targets are unchanged.

## Verification and observability

Gateway tests exercise the real WebSocket handshake and close frame, HTTP 404,
and transient 503 behavior. Fake-timer browser tests prove retries stop only
for permanent outcomes. Desktop and phone E2E tests retain stale browser state,
delete the real task, and assert ended feedback and no repeated connections.

A handled missing-environment WebSocket closes normally instead of emitting
the repeated execution-not-ready warning and 503 request log. Genuine readiness
failures retain the existing diagnostics. No new metrics are needed.
