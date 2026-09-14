---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-APPRISE-RESCAN-001
---

# Apprise Rescan System Design

## Boundaries and evidence

`Controller.ListProviders` already returns freshly evaluated
`apprise_available` from `AppriseProvider.Available`, which calls
`exec.LookPath("apprise")`. Reuse `GET /api/v1/notification-providers` through
`listNotificationProviders({ cache: "no-store" })`. No new endpoint is needed.

`useNotificationProviders` currently loads once. `useNotificationsState`
copies availability into local state inside its draft hydration guard.
That guard can prevent availability updates while provider edits are pending.

Platform owns detection. Existing settings authorization and
[semantic notification rules](../requirements/notifications.md) remain the
boundaries for configuration and delivery.

## Requirement mapping

| Acceptance criteria | Design section |
| --- | --- |
| 001.1, 001.2 | Request and state flow |
| 001.3, 001.4 | Failure and draft preservation |
| 001.5 | Desktop and mobile presentation |

All rows refer to `AC-PLATFORM-APPRISE-RESCAN-*`.

## Request and state flow

Expose an explicit availability refresh from `useNotificationProviders`.
The domain hook owns the request, pending state, and request failure.
Wait for initial provider loading to finish before enabling manual refresh.
Guard duplicate calls synchronously as well as disabling the button.

Consume only `apprise_available` from the manual response. Add a narrow
`setAppriseAvailable` settings-store action so a delayed response cannot replace
provider records, events, or loading state changed by another operation.
Keep the existing initial-load behavior separate.

Read availability directly from the domain store in `useNotificationsState`,
or synchronize it independently of the provider draft guard. Remove obsolete
local availability setters after checking all callers. Preserve the existing
initial unknown-state fallback in this scoped change.

## Failure and draft preservation

The refresh changes availability only. Preserve provider baselines, drafts,
pending removals, name and URL input, and event selections on success or error.
This also avoids overwriting a provider save that finishes during a rescan.
Ignore response provider arrays for manual rescans.

Clear the previous request error when retrying. On failure, retain availability
and show localized inline retry feedback. Always clear pending state.
An unavailable result is a completed check, not a request failure.
Do not hide an already open create form if a rescan reports Apprise missing;
retain its input and keep the existing save behavior authoritative.

The action does not invoke create, update, delete, test-notification, or browser
permission APIs. It creates no persisted settings or schema changes.

## Desktop and mobile presentation

Entry point: `/settings/preferences/notifications`, External Providers.
Use an outline `Button` labeled `Rescan Apprise`, with the existing refresh icon.
Show `Checking...` while pending and a short detected/not-detected result after
completion. Explain visibly that detection runs on the Kandev server.
Keep the empty configured-provider message distinct from binary availability.

The local exemplar is `DesktopNotificationsSection` for refresh placement.
The curated `mobile-fab.tsx` exemplar contributes an explicit touch target and
pressed feedback; this contextual settings action stays inline.
Use a header row on desktop and a wrapping or stacked action below the heading
on phones. The action needs no picker, drawer, or new route because it has no
choices or deep content. Keep the existing settings page as the scroll owner.

Maintain normal desktop button density and apply the 44px minimum only for
phone/coarse-pointer use. No fixed control or additional safe-area container is
needed. Use a semantic button, accessible status/error feedback, and visible
focus styling. Share the hook and action handler across viewports.

Add copy in English, Portuguese, and Simplified Chinese, then generate both
Traditional Chinese catalogs with `pnpm run i18n:zh-hant`.

## Validation and documentation

Hook tests cover detection changes, duplicate prevention, failure/retry, and a
save that updates provider data before a delayed rescan resolves.
Settings tests cover availability updates during dirty drafts and preservation
of open create/edit forms. Desktop and mobile Playwright tests exercise the
button using controlled provider-list responses; mobile also checks geometry.

The existing backend lookup is reused without production Go changes.
Add a short Notifications how-to section to `docs/public/developer-tools.md`
during implementation. Explain server-local detection and that changing PATH
outside the running process can still require restarting Kandev.

No ADR is needed: this reuses the current endpoint, permissions, lookup method,
and configuration ownership.
