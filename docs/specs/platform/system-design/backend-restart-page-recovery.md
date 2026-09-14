---
status: current
system: platform
requirements:
  - REQ-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001
created: 2026-09-03
updated: 2026-09-05
owners:
  - kandev
---

# Backend Restart Page Recovery System Design

## Purpose and boundaries

This design detects when an authenticated application document belongs to an
earlier backend process. It then presents one explicit reload path across all
application routes.

The backend owns process identity. The shared application shell owns detection
and recovery state. Agent settings keep a typed stale-interlock fallback for the
race before proactive detection completes.

This design does not change WebSocket retry policy, the public health endpoint,
or the interim settings interlock security model.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001` | [Identity contract](#identity-contract), [Detection flow](#detection-flow), [Recovery coordinator](#recovery-coordinator), [Alert surface](#alert-surface), [Testing strategy](#testing-strategy) |

## Identity contract

`info.Service` already creates one random `boot_id` for each backend process.
It returns that value from `GET /api/v1/system/info`.

The backend adds the same value to `BootPayload.Runtime` as `bootId`. The HTML
boot payload and the `/api/v1/app-state` fallback use the same source. The
frontend parser accepts the field as optional for compatibility with an older
payload or a partial test fixture.

The boot ID is process identity, not a credential. It contains no interlock
token, session identifier, or user data. This change does not add fields to
`GET /health`.

## Components and responsibilities

- `webapp.RuntimeConfig` transports the document's initial `bootId`.
- `fetchSystemInfo({ cache: "no-store" })` obtains the current backend identity.
- `BackendGenerationGuard` observes successful WebSocket connections from the
  shared application state. It compares the live ID with the boot payload ID.
- `backend-reload-coordinator` is a small external store. It latches the
  reload-required state, deduplicates signals, and coordinates intentional
  restart owners.
- `BackendReloadRequiredAlert` renders the global in-flow recovery surface.
- The shared API client recognizes the stable interlock error code and signals
  the same coordinator.
- Agent settings presenters suppress a local error only when the coordinator
  handled that exact stale-interlock error.

The guard and alert mount in `AppStatusSurfaceProvider`, above route content.
They remain independent of the optional application status bar. This location
also keeps the alert visible when route data is empty or stale.

## Detection flow

1. The backend creates one `boot_id` and includes it in the document boot
   payload.
2. The application shell starts its normal WebSocket connection.
3. Each transition to `connected` starts one no-cache system information
   request. A sequence guard ignores an older response after a later connection.
4. If the request fails or returns no ID, the guard records no restart. The
   existing connectivity UI remains responsible for network state.
5. If the live ID equals the document boot ID, the guard records no restart.
6. If the IDs differ, the coordinator permanently latches reload-required for
   the current document.
7. The alert becomes visible without waiting for a mutation or route change.

The comparison always uses the original document boot ID. It does not replace
the baseline after reconnect. This rule prevents a stale document from becoming
current without a full reload.

If the boot payload has no valid ID, proactive comparison is unavailable. The
typed stale-interlock response remains the recovery fallback for protected
agent settings.

## Stale-interlock fallback

The middleware response keeps HTTP 403 and adds this stable field:

```json
{
  "error": "interim settings interlock required",
  "error_code": "interim_settings_interlock_required"
}
```

The frontend matches only `error_code`. It does not match the English `error`
value or all HTTP 403 responses.

The shared API client signals the coordinator after it parses the response and
before it rejects the caller promise. It marks that `ApiError` as handled.
Agent settings presenters use the marker to prevent an internal or duplicate
error message. Agent profile deletion joins the shared response parser while it
keeps its typed HTTP 409 conflict result.

## Recovery coordinator

The coordinator has document-local state with these values:

- `current`: no changed backend generation is proven.
- `reload-required`: a changed generation or stale-interlock response is proven.

The transition to `reload-required` is one-way in the current document. Repeated
identity mismatches and interlock errors do not create new alerts or reports.

An intentional restart flow can register itself as the active reload owner
before it sends the restart request. The global alert stays hidden while that
flow presents its own blocking progress or reload surface. When the owner
unmounts without reloading, the global alert becomes the fallback. A self-update
keeps ownership through its existing controlled document reload.

The generic `useKandevRestart` flow and the self-update flow use this ownership
contract. They do not maintain a second backend-generation decision. Their
existing progress, error, and completion behavior stays intact.

## Alert surface

When no intentional restart owner is active, `BackendReloadRequiredAlert`
renders at the top of the route-content column. It is a persistent
`role="alert"` region, not a toast or modal.

The alert is non-dismissible. It does not block the current route, so a user can
copy unsaved work before reloading. Client-side route changes keep the same
coordinator and alert instance.

The **Reload page** action calls `window.location.reload()`. This preserves the
current path, query, and fragment. The next document receives the new boot ID
and starts in `current` state.

The alert and the existing connectivity warning can appear together. They
report different facts. Connectivity reports reachability. The reload alert
reports a proven process-generation change.

## Persistence and failure handling

The coordinator does not use local storage, session storage, or backend
persistence. The current document is the scope of the stale state.

A failed system information request cannot prove a restart. It does not show a
reload alert. The next successful WebSocket connection starts another check.

If the coordinator is not installed during early application startup, the
stale-interlock `ApiError` uses localized reload guidance at its caller. The raw
backend error does not become user-visible.

## Security

The system information request uses the existing authenticated endpoint and
credentials. The boot ID is safe browser-facing process metadata.

The interlock keeps its current constant-time comparison, bearer rejection,
route scope, fail-closed behavior, and token transport. The new error code does
not expose the token.

## Observability

The coordinator sends one frontend diagnostic report when a document first
enters reload-required state. The report uses the `backend-reload` report
source. Its title is `boot_id_changed` or `settings_interlock_rejected`.

The report includes browser context and no token value. It does not include a
synthetic error-toast stack or error object. The backend records the report at
info level with the fixed message `frontend backend reload required`.

Actual error-toast reports keep their error severity and fixed message. The
client cannot supply a log level or message template.

Backend logs keep the current HTTP request and restart evidence. This design
does not add a metric or durable event.

## Responsive and accessible behavior

Desktop and tablet show the alert in flow at the top of the route-content
column. Phone uses the same alert. Its text and action stack vertically.

The phone action fills the available width and has a minimum height of 44px. At
larger breakpoints, the action can use the compact button size. The alert stays
inside the existing scroll layout and does not cover bottom navigation or safe
areas.

The alert has a textual title, explanation, and button. It uses `role="alert"`
and does not rely on color. The button supports normal keyboard focus and
activation.

## Internationalization

The title, explanation, warning, and action exist in `en`, `pseudo`, `pt-pt`,
`zh-cn`, `zh-hk`, and `zh-tw`. The copy uses no Unicode em dash.

## Testing strategy

Backend boot payload tests assert that HTML and `/api/v1/app-state` expose the
same `bootId` as `/api/v1/system/info`. Existing system information tests keep
the within-process stability guarantee.

Frontend boot parser tests cover valid, missing, and malformed boot IDs. Guard
tests cover initial connection, reconnect, out-of-order responses, same-ID
responses, changed IDs, failed requests, and unmount cleanup.

Coordinator and alert tests cover the one-way latch, deduplication, route
survival, explicit reload, intentional restart ownership, localized copy, and
the no-coordinator fallback.

Backend and API client tests cover the stable stale-interlock error code. They
also cover unrelated HTTP 403 responses and profile-delete HTTP 409 conflicts.

Desktop Playwright coverage restarts a real test backend while a non-settings
route is open. It checks that the alert appears after reconnect and before
another user action. It then selects the reload action and checks that the new
document has no alert.

Mobile Playwright coverage repeats the real restart flow. It also checks the
44px action height, one scroll owner, horizontal containment, and navigation
clearance.

## Related decisions

- [SPA failure containment and deployment recovery](../../../decisions/2026-07-27-spa-failure-containment-and-deployment-recovery.md)
- [Settings route save coordinator](../../../decisions/0046-settings-route-save-coordinator.md)
- [Go-served single-page application](../../../decisions/0021-go-served-single-page-application.md)
