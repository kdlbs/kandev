---
id: "02-authenticated-transport"
title: "Share remote authentication across clients and proxies"
status: done
wave: 2
depends_on:
  - "01-provider-contract"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PLUGIN-003
acceptance_criteria:
  - AC-EXECUTORS-PLUGIN-003.1
  - AC-EXECUTORS-PLUGIN-003.2
  - AC-EXECUTORS-PLUGIN-003.3
  - AC-EXECUTORS-PLUGIN-003.4
system_design:
  - ../../specs/executors/system-design/remote-executor-plugins.md
---

# Task 02: Share remote authentication across clients and proxies

## Summary

Add the validated endpoint and connection lease layer. Verify ordinary HTTP, each WebSocket client, and editor/preview proxy upgrades through the same authentication authority.

## In scope

- Preserve NewClient(host, port) and add an endpoint constructor with shared lease resolution, singleflight refresh, cancellation and connection generations.
- Replace all fixed-prefix WebSocket URL derivations and DefaultDialer use with the shared path.
- Wire vscode_proxy, port_proxy and port_tunnel to the same upstream authentication, redirect policy and generation invalidation.

## Out of scope

- Cloud allocation, browser UI changes, exposing application ports directly through provider credentials.

## Acceptance

- HTTP, agent stream, workspace stream, shell terminal, LSP, editor and preview upgrade tests all observe the expected provider and host authentication.
- Concurrent refresh mints one lease; expired, revoked, malformed and cross-origin requests fail safely without replaying mutations.
- Existing host/port transport behavior remains covered and unchanged.

## Verification

Use TDD for changed logic. Run commands from the repository root.
Tests named below are new unless an existing path is listed. Do not accept a no-tests result.

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/agentctl ./internal/gateway/websocket -count=1)
```

Required evidence:

- `internal/agent/runtime/agentctl/endpoint_transport_test.go: TestPluginExecutorTransportMatrix, TestPluginExecutorLeaseRefresh, TestPluginExecutorEndpointRejection`
- `internal/gateway/websocket/executor_transport_test.go: TestPluginExecutorProxyHTTPAndUpgrade, TestPluginExecutorProxyGeneration`

## Files likely touched

- `apps/backend/internal/agent/runtime/agentctl/client.go`
- `apps/backend/internal/agent/runtime/agentctl/agent.go, workspace_stream.go, client_shell_terminal.go, client_lsp.go`
- `apps/backend/internal/agent/runtime/agentctl/ (new endpoint_transport.go and endpoint_transport_test.go)`
- `apps/backend/internal/gateway/websocket/vscode_proxy.go, port_proxy.go, port_tunnel.go`
- `apps/backend/internal/gateway/websocket/ (new executor_transport_test.go)`

## Dependencies

[Task 01](task-01-provider-contract.md).

## Risks

ReverseProxy handles upgrades independently of Gorilla dialing. Test both; preserve application subprotocols and redact provider-only values.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/remote-executor-plugins.md), using the IDs in frontmatter.
- [System design](../../specs/executors/system-design/remote-executor-plugins.md), including the named contract and flow.
- Existing source and nearby tests in the file list; follow scoped `AGENTS.md`.
- [Accepted ADR](../../decisions/2026-09-26-remote-executor-plugin-boundary.md).

## Results

Implemented a validated endpoint client with in-memory, generation-scoped connection leases and
singleflight refresh 30 seconds before expiry. HTTP and WebSocket requests share provider and
host authentication, redirect protection, and DNS checks at dial time. VS Code, preview, and
port-tunnel reverse proxies use the same transport; cached editor/preview proxies compare both
endpoint and connection generation. Generation changes close idle connections and leave active
streams connected. Existing NewClient(host, port, ...) behavior remains covered.

Validation passed:

- From apps/backend, go test -race ./internal/agent/runtime/agentctl ./internal/gateway/websocket -count=1 passed. It includes TestPluginExecutorTransportMatrix, TestPluginExecutorLeaseRefresh, TestPluginExecutorEndpointRejection, TestPluginExecutorRevokedLeaseFailsClosed, TestPluginExecutorProxyHTTPAndUpgrade, and TestPluginExecutorProxyGeneration.
- From the repository root, make -C apps/backend lint passed with 0 issues.
- git diff --check and gofmt -l on all Task 02 Go files passed.

The TLS matrix uses a trusted test certificate and injected fake public DNS mapping. It does not
disable certificate verification. Cross-origin redirects fail before a mutating request can replay.

Review remediation: `BootstrapHandshake` now publishes its token through the same synchronized source
used by HTTP, WebSocket dialing, and `Client.AuthToken()`. `TestPluginExecutorEndpointBootstrapHandshake`
performs an authenticated WebSocket upgrade after the real handshake, in addition to its HTTP check.
