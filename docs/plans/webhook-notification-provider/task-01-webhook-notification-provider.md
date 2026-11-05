---
id: "01-webhook-notification-provider"
title: "Add the webhook notification provider"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WEBHOOK-NOTIFICATIONS-001
acceptance_criteria:
  - AC-PLATFORM-WEBHOOK-NOTIFICATIONS-001.1
  - AC-PLATFORM-WEBHOOK-NOTIFICATIONS-001.2
  - AC-PLATFORM-WEBHOOK-NOTIFICATIONS-001.3
  - AC-PLATFORM-WEBHOOK-NOTIFICATIONS-001.4
  - AC-PLATFORM-WEBHOOK-NOTIFICATIONS-001.5
  - AC-PLATFORM-WEBHOOK-NOTIFICATIONS-001.6
system_design:
  - ../../specs/platform/system-design/webhook-notifications.md
---

# Task 01: Add the Webhook Notification Provider

## Summary

Add the `webhook` notification provider and its settings surface, then harden
the save path and outbound request policy against the review findings. The
provider reuses the existing provider CRUD, ownership checks, event
subscriptions, and atomic save. It does not introduce a second notification
configuration path.

## Scope and files

Backend:

- `apps/backend/internal/notifications/models/models.go`: add the provider type.
- `apps/backend/internal/notifications/providers/webhook.go`: validation,
  payload, delivery, SSRF dial policy, private-network opt-in, and redirect
  policy.
- `apps/backend/internal/notifications/providers/webhook_test.go`: payload,
  header, redirect, SSRF, opt-in, timeout, sanitized errors, and IPv6
  transition range coverage.
- `apps/backend/internal/notifications/service/service.go` and
  `webhook_provider_test.go`: register the provider and cover provider CRUD.
- `apps/backend/internal/common/config`: define the typed private-network
  setting, source precedence, catalog entry, and tests.
- `apps/backend/internal/notifications/controller/controller.go`: default name.
- `apps/backend/internal/notifications/dto/dto.go` and `dto_test.go`: serialize
  empty subscriptions as an empty list.

Frontend:

- `notifications-settings-webhook-helpers.ts`: header parsing, validation,
  formatting, and webhook form state.
- `notifications-settings-webhook-providers.tsx` and
  `notifications-settings.tsx`: settings section and page wiring.
- `notifications-settings-actions.ts`: create/update drafts and mutual
  exclusion.
- `notification-save-reconciliation.ts`: compare every submitted webhook field
  and per-form identity before clearing a create form.
- `notifications-settings-webhook-save.test.tsx`,
  `notifications-settings-actions.test.tsx`, and
  `notifications-settings.test.tsx`: save, validation, raw header draft, and
  form behavior.
- `apps/web/lib/types/http.ts` and `apps/web/src/locales/*/settings.json`:
  provider type and localized copy.
- `apps/web/e2e/tests/settings/webhook-provider-save-reload.spec.ts`: desktop
  create, save, reload, and edit journey.
- `apps/web/e2e/tests/settings/mobile-webhook-provider.spec.ts`: phone
  create, edit, remove, and touch target coverage.
- `docs/public/developer-tools.md` and `docs/public/configuration.md`: webhook
  setup, payload, credentials, testing, and operator configuration.

## Acceptance

1. Notification settings creates, edits, tests, and removes a webhook provider
   with name, URL, headers, and secret on desktop and phone.
2. Delivery posts the versioned JSON payload with configured headers and the
   optional bearer secret, times out after 10 seconds, and reports a sanitized
   status for non-2xx responses. Transport errors do not include the URL or
   response text.
3. Invalid header names or values are rejected by the backend and the settings
   form; header names that differ only by case are rejected instead of
   overwriting nondeterministically.
4. Private, loopback, link-local, IPv6 NAT64, IPv6 6to4, and reserved
   destinations are rejected by default at validation and dial time. The
   typed `notifications.webhookAllowPrivateNetworks` setting permits
   intentional private destinations. Its default is false; YAML and the
   `KANDEV_WEBHOOK_ALLOW_PRIVATE_NETWORKS` environment variable configure it.
5. Cross-origin redirects fail; same-origin redirects are allowed up to the
   configured limit, so credential headers never reach another origin.
6. A saved webhook provider reloads with its configuration and `events: []`
   when it has no subscriptions. Edits made during an in-flight create remain
   in the form, invalid raw header drafts survive provider switches and block
   Done and Save, and Apprise and webhook create forms cannot be open together.
   Edit and Remove actions have phone-sized touch targets.

## Regression sequence

Add backend table tests for payload fields, header validation, case-duplicate
headers, redirect policy, literal and resolved private addresses, the opt-in,
and timeout behavior before changing the provider. Add service tests for
registration and CRUD, and DTO tests for nil subscriptions.

Add frontend tests for invalid headers, mutual exclusion, submitted-field
reconciliation, replacement form identity, and raw header drafts during
provider switches. Extend the settings tests for create/edit/remove rendering.
Add desktop and phone Playwright journeys. The phone journey checks the touch
targets while creating, editing, and removing a provider.

## Exact verification commands

From `apps/backend`:

```bash
go test ./internal/notifications/...
```

From `apps/web`:

```bash
pnpm exec vitest run components/settings/notifications-settings-actions.test.tsx components/settings/notifications-settings.test.tsx components/settings/notifications-settings-webhook-save.test.tsx
pnpm run typecheck
pnpm exec eslint --max-warnings 0 components/settings/notification-save-reconciliation.ts components/settings/notifications-settings-actions.ts components/settings/notifications-settings-provider-updates.ts components/settings/notifications-settings-webhook-helpers.ts components/settings/notifications-settings-webhook-providers.tsx components/settings/notifications-settings.tsx components/settings/notifications-settings-actions.test.tsx components/settings/notifications-settings.test.tsx components/settings/notifications-settings-webhook-save.test.tsx lib/types/http.ts eslint.i18n.options.mjs e2e/tests/settings/webhook-provider-save-reload.spec.ts
pnpm run i18n:check
pnpm e2e:run --host --shards 1 --project chromium tests/settings/webhook-provider-save-reload.spec.ts
pnpm e2e:run --host --shards 1 --project mobile-chrome tests/settings/mobile-webhook-provider.spec.ts
```

From the repository root:

```bash
gofmt -l apps/backend/internal/notifications/providers/webhook.go apps/backend/internal/notifications/providers/webhook_test.go apps/backend/internal/notifications/service/service.go apps/backend/internal/notifications/service/webhook_provider_test.go apps/backend/internal/notifications/controller/controller.go apps/backend/internal/notifications/dto/dto.go apps/backend/internal/notifications/dto/dto_test.go
```

## Dependencies

None. The provider uses the existing notification service, provider registry,
settings save path, and e2e fixtures.

## Inputs

- [Semantic notification requirements](../../specs/platform/requirements/notifications.md).
- [Webhook notification system design](../../specs/platform/system-design/webhook-notifications.md).
- Existing Apprise provider and settings patterns.
- Repository outbound SSRF guidance in `docs/public/extending-kandev.md`.

## Output contract

Report provider behavior, validation and SSRF policy, redirect handling,
save/reconciliation behavior, files changed, exact test results, and task
status.
