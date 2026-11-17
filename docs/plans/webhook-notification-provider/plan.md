---
created: 2026-10-29
status: complete
requirements:
  - REQ-PLATFORM-WEBHOOK-NOTIFICATIONS-001
system_design:
  - ../../specs/platform/system-design/webhook-notifications.md
legacy_specs: []
---

# Webhook Notification Provider

## Outcome

Add an opt-in notification provider that posts selected events to an HTTP
endpoint. The provider participates in the existing provider registry, event
subscriptions, atomic save path, and settings page instead of introducing a
parallel notification configuration.

The implementation also closes the review findings from PR #4899: newer draft
edits survive an in-flight create, the Apprise and webhook create forms cannot
be open together, invalid or duplicate headers are rejected before delivery,
redirects cannot forward configured credentials to another origin, and webhook
destinations are validated against an SSRF policy before every connection.

## Contracts and traceability

| Outcome | Acceptance criteria | Work order |
| --- | --- | --- |
| Webhook providers are configurable in notification settings | `AC-PLATFORM-WEBHOOK-NOTIFICATIONS-001.1` | 01 |
| Events are delivered as a versioned JSON POST with bounded failure behavior | `AC-PLATFORM-WEBHOOK-NOTIFICATIONS-001.2` | 01 |
| Invalid and case-duplicate headers are rejected in the backend and UI | `AC-PLATFORM-WEBHOOK-NOTIFICATIONS-001.3` | 01 |
| Private destinations are denied unless the operator opts in, including at dial time | `AC-PLATFORM-WEBHOOK-NOTIFICATIONS-001.4` | 01 |
| Cross-origin redirects cannot receive configured credentials | `AC-PLATFORM-WEBHOOK-NOTIFICATIONS-001.5` | 01 |
| Save and reload preserve the accepted provider and newer draft edits | `AC-PLATFORM-WEBHOOK-NOTIFICATIONS-001.6` | 01 |

The system design records the provider boundary, payload contract, validation
rules, outbound safety policy, and reconciliation behavior. No ADR is needed:
the change follows the existing provider pattern and established outbound
validation guidance.

## Work orders and dependency order

- [x] [Task 01: Add the webhook notification provider](task-01-webhook-notification-provider.md)

One work order covers the backend provider, settings surface, save behavior,
and automated coverage because the delivered behavior is one provider
integration.

## Verification strategy

Backend coverage proves payload construction, header validation, redirect
policy, SSRF rejection, the private-network opt-in, and timeout behavior.
Frontend coverage proves save reconciliation, form identity, mutual exclusion,
header validation, and provider persistence. The desktop Playwright journey
creates a provider through the UI, saves it, reloads the settings page, and
checks the restored configuration. Typecheck, lint, localization checks, and
Go formatting complete the validation.

## Results

The implementation is present on the PR branch. The backend notification
packages, targeted frontend unit tests, typecheck, changed-file lint,
localization checks, and the webhook save/reload Playwright journey pass. The
remaining notification-service failure, when the full package is run, also
reproduces unchanged on `github/main` and is unrelated to this provider.

Public operator guidance lives in `docs/public/developer-tools.md`. The
documentation coverage work order is this package.
