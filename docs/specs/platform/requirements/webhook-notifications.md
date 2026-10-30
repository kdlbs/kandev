---
status: draft
system: platform
created: 2026-10-29
owners:
  - kandev
---

# Webhook Notification Provider Requirements

## Overview

Kandev notification events can be delivered to Local, System, and Apprise
providers. Users running HTTP automation need a direct outbound integration
without installing Apprise. This requirement adds an opt-in webhook provider
inside notification settings while preserving the existing provider event model
and delivery boundaries described by
[semantic notifications](notifications.md).

## Requirements

### REQ-PLATFORM-WEBHOOK-NOTIFICATIONS-001: Webhook notification provider

**Intent:** Deliver selected notification events to a configured HTTP endpoint
while keeping provider configuration validated, durable, and safe to expose to
the backend.

#### Acceptance criteria

- **AC-PLATFORM-WEBHOOK-NOTIFICATIONS-001.1:** Notifications settings shall offer
  a Webhook Providers section where a user can create, edit, test, and remove a
  webhook provider with a name, destination URL, optional headers, and an
  optional secret. The section shall use the existing shared Save, Reset, and
  localized settings behavior on desktop and phone.
- **AC-PLATFORM-WEBHOOK-NOTIFICATIONS-001.2:** A webhook delivery shall POST an
  `application/json` payload with a payload version, event type, title, message,
  type, and non-empty task, session, and occurrence fields. Configured headers
  shall be applied, an optional secret shall become a Bearer authorization
  header unless the configuration already supplies `Authorization`, and each
  delivery shall fail with a bounded error after at most 10 seconds.
- **AC-PLATFORM-WEBHOOK-NOTIFICATIONS-001.3:** A webhook URL shall require an
  `http` or `https` scheme and a host. Header names and values shall be valid
  HTTP fields, and names that differ only by case shall be rejected. The backend
  shall enforce these rules during validation and delivery, and the settings
  form shall report invalid headers before saving.
- **AC-PLATFORM-WEBHOOK-NOTIFICATIONS-001.4:** By default Kandev shall reject
  webhook destinations that resolve to loopback, private, link-local,
  multicast, unspecified, or reserved addresses, including at connection time so
  a changed DNS answer cannot bypass validation. A documented backend
  environment opt-in may allow private destinations for intentional local
  integrations.
- **AC-PLATFORM-WEBHOOK-NOTIFICATIONS-001.5:** A webhook request shall follow
  only same-origin redirects. A cross-origin redirect shall fail before
  configured credential headers are sent to the new origin.
- **AC-PLATFORM-WEBHOOK-NOTIFICATIONS-001.6:** A successful create or edit shall
  persist the provider and its subscriptions, restore the same configuration
  after reload, and serialize a provider with no enabled subscriptions as an
  empty event list rather than `null`. Edits made while a create request is in
  flight shall remain in the form, and the Apprise and webhook create forms
  shall not be open at the same time.

## Out of scope

- Installing or configuring external automation services.
- Changing event selection, delivery retries, or deduplication owned by
  [semantic notifications](notifications.md).
- Template scripting, payload transforms, or per-event header overrides.
