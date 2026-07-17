---
id: task-04
title: Fixture plugin binary
status: done
wave: 1
depends_on: []
plan: docs/plans/plugins/plan.md
---

# Fixture plugin binary

## Title
Standalone Go HTTP server implementing the plugin contract, for Go integration
tests and Playwright e2e.

## Inputs
- Spec `docs/specs/plugins/spec.md` → event delivery headers/HMAC, tool
  invocation, webhook proxy, UI page contract.
- Must be self-contained: only stdlib + `crypto/hmac`,`crypto/sha256`. NO imports
  from `internal/plugins` (proves the contract is decoupled).

## Acceptance
1. `apps/backend/cmd/plugin-fixture/main.go` starts an HTTP server on a port from
   `-addr`/`PLUGIN_FIXTURE_ADDR` (default `127.0.0.1:0` → print chosen addr on
   stdout as `LISTENING <addr>` so tests can capture it).
2. Endpoints:
   - `GET /health` → 200 `{"status":"ok"}`.
   - `POST /events` → verifies `X-Kandev-Signature: sha256=<hmac>` against
     `HMAC-SHA256(secret, rawBody)` using secret from `-secret`/`PLUGIN_FIXTURE_SECRET`;
     records delivery (event_type + event_id) in memory; returns 200. Bad sig → 401.
   - `POST /tools/echo` → returns `{"output": <input>}`.
   - `POST /webhooks/test-hook` → records the call; returns 200.
   - `GET /ui/dashboard` → returns a small HTML page with `<h1 id="plugin-dashboard">Fixture Plugin</h1>`
     and a `<script>` that reads theme via postMessage (enough for e2e to assert render).
   - `GET /_debug/deliveries` → JSON list of recorded event_types (test-only introspection).
3. Graceful shutdown on SIGINT/SIGTERM.

## Files
- `apps/backend/cmd/plugin-fixture/main.go`
- `apps/backend/cmd/plugin-fixture/main_test.go` (spins the server, asserts health +
  HMAC accept/reject + echo)

## Verification
- `go test ./cmd/plugin-fixture/...` from `apps/backend`
- `go build ./cmd/plugin-fixture` from `apps/backend`
- `make -C apps/backend lint`

## Output contract
Report: chosen flags/env, the `LISTENING <addr>` handshake, endpoints implemented,
how HMAC is verified. Stay within `apps/backend/cmd/plugin-fixture/`.

## Dependencies
None.
