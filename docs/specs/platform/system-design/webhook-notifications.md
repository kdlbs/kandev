---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-WEBHOOK-NOTIFICATIONS-001
---

# Webhook Notification Provider System Design

## Boundaries and ownership

Platform owns the notification provider registry and settings surface. The new
`webhook` provider extends the existing provider contract in
`apps/backend/internal/notifications/providers`; event selection, occurrence
identity, claims, and retries stay in the notification service and
[semantic notification rules](../requirements/notifications.md).

Backend ownership:

- `providers/webhook.go` validates configuration, builds the payload, and
  performs the outbound request.
- `models` adds the provider type; `service.NewService` registers the provider.
- `common/config` owns the private-network policy. `backendapp` passes the
  typed startup value through the notification service to the webhook provider.
- `controller` supplies the default provider name.
- `dto.FromProvider` normalizes nil subscriptions to an empty list.

Frontend ownership:

- `notifications-settings-webhook-helpers.ts` owns header parsing, validation,
  formatting, and webhook form state.
- `notifications-settings-webhook-providers.tsx` renders the create, edit, and
  summary surfaces.
- `notifications-settings-actions.ts` builds create and update drafts.
- `notification-save-reconciliation.ts` acknowledges saves without clearing
  newer edits.

## Requirement mapping

| Acceptance criteria | Design section |
| --- | --- |
| 001.1 | Settings and provider registry |
| 001.2 | Payload and delivery |
| 001.3 | Configuration validation |
| 001.4, 001.5 | Outbound destination safety |
| 001.6 | Save, reload, and draft reconciliation |

All rows refer to `AC-PLATFORM-WEBHOOK-NOTIFICATIONS-*`.

## Settings and provider registry

A webhook provider uses the existing provider CRUD, ownership checks, event
subscriptions, and atomic provider save path. The settings page filters
providers by `type === "webhook"` and renders them in a dedicated section with
the shared floating Save action. The existing provider test action is reused.

Create and edit forms share one webhook form state. Opening one provider's edit
form, or starting a webhook create, closes the other provider create form. This
prevents one Save from carrying two independent create drafts. Draft identity
and a snapshot of every submitted field let acknowledgement distinguish the
submitted form from a replacement or a newer edit.

Edit headers keep their raw text in a map keyed by provider ID. Invalid text
stays visible when the user switches providers. **Done** keeps the edit form
open while its headers are invalid, and Save rejects every invalid header draft.
Cancel removes that provider's raw draft and restores its saved configuration.

## Payload and delivery

Delivery uses `POST` with `Content-Type: application/json` and this versioned
payload:

| Field | Source |
| --- | --- |
| `version` | Fixed payload contract version |
| `event` | Notification event type |
| `title` | Notification title |
| `message` | Notification body |
| `type` | Presentation type, currently `info` |
| `fields` | Non-empty `task_id`, `task_session_id`, `occurrence_id` |

Configured headers are applied before sending. When a non-empty secret exists
and no configured header already supplies `Authorization` (case-insensitive),
the secret becomes `Authorization: Bearer <secret>`. The request has a 10-second
timeout. A non-2xx response reports its status without its body. Transport
errors use a generic message. This prevents URL path or query values and
response text from entering service error logs. Response bodies are discarded
up to 4 KiB before the connection closes.

## Configuration validation

Both backend validation and the settings form enforce:

- URL scheme is `http` or `https`, with a host.
- Header names match the HTTP field-name grammar.
- Header values contain no control characters.
- Header names that differ only by case are duplicates and are rejected.

The backend validates stored configuration again before every send, so a
configuration written outside the settings UI cannot bypass these rules. The
frontend reports one localized invalid-headers error before save.

## Outbound destination safety

The provider must not become an SSRF proxy for a provider owner.

- `parseWebhookConfig` rejects literal non-global addresses and localhost
  hostnames.
- A custom dialer resolves each named host and validates every candidate address
  before connecting. This covers DNS rebinding and changes between validation
  and delivery.
- The custom transport disables proxy use so the dial-time check sees the real
  destination.
- Loopback, RFC1918, shared address space, link-local, multicast, unspecified,
  documentation, benchmarking, and reserved ranges are rejected. IPv6 NAT64
  (`64:ff9b::/96`) and 6to4 (`2002::/16`) ranges are also rejected because they
  can carry private IPv4 destinations.
- The typed startup setting `notifications.webhookAllowPrivateNetworks` allows
  intentional private or local integrations. Its default is `false`; YAML and
  `KANDEV_WEBHOOK_ALLOW_PRIVATE_NETWORKS` use the standard startup source
  precedence. The setting applies to the whole backend process.

Redirects are handled by the HTTP client policy. Only same-origin redirects
(same scheme and authority) are followed, up to ten hops. A cross-origin
redirect fails, so custom credential headers never reach a different origin.

## Save, reload, and draft reconciliation

Provider creation uses the existing atomic save path. A webhook create draft is
sent with the provider name, URL, parsed headers, and optional secret. A
provider with no subscriptions is serialized with `events: []`.

Save acknowledgement compares the current draft against the submitted snapshot:
URL, name, secret, and the normalized header signature. If any submitted field
changed while the request was in flight, the form stays open and dirty. Only an
unchanged draft is cleared. Raw per-provider header drafts are acknowledged
only when they still match the submitted text. A per-form identity separates a
replacement form with identical text from the submitted form.

## Validation and documentation

Automated coverage lives in:

- `providers/webhook_test.go` for payloads, headers, redirects, SSRF, timeout,
  and invalid configuration.
- `service/webhook_provider_test.go` for registration and provider CRUD.
- `notifications-settings-webhook-save.test.tsx` and
  `notifications-settings-actions.test.tsx` for save reconciliation and
  mutual exclusion, invalid raw header drafts, and provider switching.
- `apps/web/e2e/tests/settings/webhook-provider-save-reload.spec.ts` for the
  desktop create, save, reload, and edit journey.
- `apps/web/e2e/tests/settings/mobile-webhook-provider.spec.ts` for phone
  creation, editing, removal, and touch target sizes.

Operator-facing behavior is documented in `docs/public/developer-tools.md`.
No ADR is required: the provider follows the existing provider boundary, and
the SSRF and redirect policy reuse established outbound validation patterns.
