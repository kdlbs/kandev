---
id: task-08
title: Tool invocation client + webhook & UI reverse proxies
status: pending
wave: 2
depends_on: [task-05]
plan: docs/plans/plugins/plan.md
---

# Tool invocation client + webhook & UI reverse proxies

## Title
Outbound HTTP to plugins: tool invocation client (kandev→plugin), external webhook
reverse proxy, and iframe UI page reverse proxy with header stripping.

## Inputs
- Spec `docs/specs/plugins/spec.md` → "Agent tool invocation" (POST
  `{base_url}/tools/{name}`, 30s timeout, `{tool_call_id, input, context}` →
  `{output}`), "External webhook proxy" (forward body+headers, add
  `X-Plugin-Id`,`X-Webhook-Key`), and the NEW UI proxy section (GET/POST
  `{base_url}{ui.page.path}`; strip `X-Frame-Options` and frame-restricting CSP
  from the plugin response so kandev can iframe it; inject nothing else).
- Header-stripping precedent: read `apps/backend/internal/agentctl/server/api/AGENTS.md`
  and the reverse-proxy code there (Accept-Encoding handling + iframe-blocking
  header removal) and mirror the approach with `httputil.ReverseProxy` +
  `ModifyResponse`.

## Acceptance
1. `internal/plugins/client.go`: `Client.InvokeTool(ctx, record, toolName, input, callCtx) (json.RawMessage, error)`,
   30s timeout, non-2xx → typed error. `Client.Health(ctx, record) error` (used by task-07).
2. `internal/plugins/proxy.go`: `WebhookProxy(record, webhookKey) http.Handler`
   forwarding to `{base_url}{endpoints.webhooks templated with key}`, adding
   `X-Plugin-Id`/`X-Webhook-Key`, preserving method/body/query.
3. `UIProxy(record, page) http.Handler` reverse-proxying to `{base_url}{page.path}`,
   `ModifyResponse` strips `X-Frame-Options` and removes `frame-ancestors` from CSP;
   rewrites relative redirects under the kandev proxy path.
4. Both proxies 404/503 when record is nil / not active (the handler layer task-09
   passes only valid records; still guard).

## Files
- `apps/backend/internal/plugins/client.go` + `client_test.go`
- `apps/backend/internal/plugins/proxy.go` + `proxy_test.go` (httptest upstream)

## Verification
- `go test ./internal/plugins/...` from `apps/backend`
- `make -C apps/backend lint`

## Output contract
Report: tool-call request/response shapes, exact headers stripped on UI proxy,
redirect-rewrite behavior. Import task-05 types only; do not edit other tasks' files.

## Dependencies
task-05.
