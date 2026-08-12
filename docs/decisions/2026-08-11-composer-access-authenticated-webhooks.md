# ADR-2026-08-11-composer-access-authenticated-webhooks: Composer Access and Authenticated Webhooks

**Status:** accepted
**Date:** 2026-08-11
**Area:** frontend, backend, protocol, security

## Context

Voice Mode currently reaches private editor handles and native submit callbacks in four composer
families, while plugin slots receive identifiers only. Plugin UI already calls its backend through
webhooks, but declarations cannot distinguish public integration ingress from authenticated Kandev UI
calls and all requests share a 4 MiB cap. Voice transcription needs authenticated UI access and at
least 10 MiB while keeping its OpenAI key in plugin-owned secret settings.

## Decision

Kandev exposes a typed, mounted-composer capability only through composer slot props. The capability
adapts insertion, focus, state, and submission to the owning surface's existing editor/form code; it
does not expose draft contents or let plugins synthesize outbound messages. Task chat, Quick Chat,
task creation, and new-session creation each supply an adapter to the same public shape.

Kandev extends each webhook declaration with `access: public|authenticated` and
`max_body_bytes`. Existing declarations default to `public` and 4 MiB. Authenticated webhooks require
the normal Kandev identity and same-origin browser checks; their configured limit may be raised to a
16 MiB host ceiling. Both modes continue through the existing `HandleWebhook` RPC, with request
cancellation propagated through its context.

Plugin localization is deferred to a separate prerequisite. It is required before core Voice Mode
can be removed, but it is independent of composer access and authenticated backend invocation.

## Consequences

Plugins can augment native composers without duplicating message construction or form behavior. A
plugin UI can use the existing webhook API for a larger authenticated upload without adding a route,
frontend API, protobuf method, or token lifecycle. Plugin authors must deliberately classify each
webhook; older manifests retain their current behavior.

## Alternatives Considered

- **Expose drafts and editor handles through `host.store`.** Rejected because drafts are local React
  state, editor implementations differ, and a global mutable handle would outlive or target the wrong
  composer during navigation.
- **Let plugins send messages through `api_write:messages`.** Rejected because it bypasses the native
  submit and form behavior this extraction must preserve.
- **Add a separate browser-action route and RPC.** Rejected because access mode and body size are
  properties of the existing UI-to-plugin webhook relay; a second transport duplicates it.
- **Raise every webhook to 16 MiB.** Rejected because existing public webhook callers do not need the
  larger exposure. Limits remain declaration-specific and bounded by the host ceiling.
- **Add host localization in this package.** Rejected because locale/catalog ownership is independent
  of the two host blockers addressed here and can be designed as a separate prerequisite.
