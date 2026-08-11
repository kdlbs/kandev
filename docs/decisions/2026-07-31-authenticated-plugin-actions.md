# ADR-2026-07-31-authenticated-plugin-actions: Authenticated Plugin Actions

**Status:** accepted
**Date:** 2026-07-31
**Area:** backend, frontend, protocol, security

## Context

The frontend plugin host currently maps plugin fetches toward `/api/plugins/:id/...`,
while the host plugin endpoint is an externally callable webhook proxy. Reusing that
public callback surface for browser commands would bypass normal user authentication,
resource authorization, request limits, and cancellation controls.

Repository-provider plugins need browser-triggered connection, browse, review, and
watch operations without turning each provider into a first-party HTTP route.

## Decision

The manifest declares plugin action keys, each with canonical field `scope` set to
`workspace`, `task`, or `repository`, plus a bounded body size. Kandev continues to
read the prerelease `resource_scope` spelling for package compatibility, but new
manifests and serialized records use `scope`. Kandev exposes one authenticated
route: `POST /api/plugins/:id/actions/:key`. It rejects inactive plugins and undeclared
actions; normal Kandev authentication and server-side authorization verify every
referenced resource before the host dispatches `Plugin.HandleAction`.

The host derives task/workspace relationships itself, forwards verified actor and
resource context separately from untrusted action JSON, applies a hard timeout and
cancellation, caps request/response bodies, and permits only a small response-header
allowlist. The frontend contract is `host.api.invokeAction(...)`; plugin code must not
use public webhook paths for authenticated browser work. Resource selectors use the
documented camelCase JSON envelope (`workspaceId`, `taskId`, `repositoryId`). Callers
may pass an `AbortSignal`; registry/provider teardown propagates it through the browser
request and backend context.

`PluginActionResponse.status` is an optional HTTP status projection. Zero keeps the
legacy `200 OK`; otherwise the host accepts statuses from 200 through 599. This lets
provider-neutral adapters preserve safe domain outcomes such as invalid input,
authentication expiry, conflicts, and rate limits without turning them into transport
failures. `Retry-After` joins the response-header allowlist for throttling responses.
Invalid statuses fail closed as `502`, while plugin transport/runtime failures remain
`503` and do not expose internal error text.

`/api/plugins/:id/webhooks/:key` remains public only for provider callbacks/events.
OAuth callbacks use signed, expiring, single-use state and PKCE.

## Consequences

Future plugins gain one reusable browser-to-plugin RPC seam without Kandev learning
provider endpoints or payloads. Plugin authors declare each action up front and must
design against explicit resource scope and bounded input/output. The host gains a
security-critical authorization boundary, but no provider-specific handler branch.
Callers can distinguish retryable and user-correctable domain outcomes while the host
continues to redact unexpected implementation failures.

## Alternatives Considered

- Reuse public webhook proxy for browser calls: rejected because callback endpoints do
  not establish user identity or authorize Kandev resources.
- Add provider-specific REST routes in the host: rejected because every connector
  would pull provider payloads and authentication into core.
- Let frontend bundles call plugin processes directly: rejected because managed plugin
  transport is not a browser-accessible authorization boundary.
