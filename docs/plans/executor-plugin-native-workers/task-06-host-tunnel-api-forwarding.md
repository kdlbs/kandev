---
id: "06-host-tunnel-api-forwarding"
title: "Negotiated transport and API forwarding (R2)"
status: pending
wave: 4
depends_on:
  - "05"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-NATIVE-WORKER-004
acceptance_criteria:
  - AC-EXECUTORS-NATIVE-WORKER-004.1
  - AC-EXECUTORS-NATIVE-WORKER-004.2
  - AC-EXECUTORS-NATIVE-WORKER-004.3
  - AC-EXECUTORS-NATIVE-WORKER-004.4
  - AC-EXECUTORS-NATIVE-WORKER-004.5
  - AC-EXECUTORS-NATIVE-WORKER-004.6
  - AC-EXECUTORS-NATIVE-WORKER-004.7
  - AC-EXECUTORS-NATIVE-WORKER-004.8
  - AC-EXECUTORS-NATIVE-WORKER-004.9
  - AC-EXECUTORS-NATIVE-WORKER-004.10
system_design:
  - ../../specs/executors/system-design/remote-executor-plugin-native-workers.md
---

# Task 06: Negotiated transport and API forwarding (R2)

## Summary

Add the OpenExecutorTunnel stream, the lease transport field, the host tunnel dialer used by agentctl and gateway proxies, and provider-forwarded API URL handling.

## Size

L: ~600 source, ~900 test lines.

## In scope

- Add the OpenExecutorTunnel stream, the lease transport field, the host tunnel dialer used by agentctl and gateway proxies, and provider-forwarded API URL handling.

## Out of scope

- Any change to https lease validation; a public relay.

## Acceptance

- A host_tunnel lease carries agentctl HTTP and WebSocket traffic, including editor and preview proxies, with nothing sent before the provider acknowledges.
- Cancellation, plugin restart, disable, and limits close streams without changing the environment.

## Failing-first tests

- No bytes before ack; unacknowledged stream closes at 15 s as unavailable (fails: no stream exists).
- Disallowed port refused before opening.
- host_tunnel lease from a provider without the capability rejected.
- Cancel closes stream and provider notified; count returns to zero.
- Plugin restart closes all streams; next request reopens; no mutating replay.
- 65th stream is rate_limited; others unaffected.
- provider_forwarded skips public URL check, still probes, rolls back on probe failure; host_public unchanged.
- Invalid-endpoint tests: https validation still rejects loopback.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race -trimpath ./internal/agent/runtime/agentctl ./internal/gateway/websocket ./internal/plugins/... ./pkg/pluginsdk -count=1)
```

```bash
go test -race -trimpath ./internal/agent/runtime/lifecycle -count=1
```

## Likely files and risks

- `plugin.proto`
- `pkg/pluginsdk`
- `internal/agent/runtime/agentctl/endpoint_transport.go`
- `internal/gateway/websocket (vscode_proxy, port_proxy, port_tunnel)`
- Risk: concurrency on both dialects; add the PostgreSQL race run before marking done.
