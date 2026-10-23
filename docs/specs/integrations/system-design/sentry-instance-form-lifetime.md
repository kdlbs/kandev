---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-SENTRY-FORM-LIFETIME-001
---

# Sentry Instance Form Lifetime System Design

## Purpose and boundaries

The integrations system owns Sentry instance configuration. This local repair
uses the existing [settings coordinator](../../ui/system-design/settings-manual-save.md)
and [ADR 0046](../../../decisions/0046-settings-route-save-coordinator.md).
No reusable authority or architecture decision is introduced.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-INTEGRATIONS-SENTRY-FORM-LIFETIME-001 | Opening ownership; Submission and recovery; Desktop and phone |

## Components and responsibilities

`apps/web/components/sentry/sentry-settings.tsx` owns the mutually exclusive
add/edit opening and its dirty card. `sentry-instance-form.tsx` owns fields,
baseline, test-connection feedback, and contributor registration.
`SettingsSaveProvider` owns route-level admission, status, and navigation.
`lib/api/domains/sentry-api.ts` sends POST `/api/v1/sentry/instances` or PUT
`/api/v1/sentry/instances/:id` with captured `workspace_id` and payload.
The existing backend Sentry service remains the durable configuration owner.

## Opening ownership

Each explicit Add/Edit starts a fresh local identity, independent of instance ID
and serialized field values. Cancel invalidates it immediately. Captured
completion and dirty callbacks may close or mark clean only their own current
opening. Successful completion always retains list refresh through the independent
`onPersisted` callback, even after form unmount. `onSaved` closes only a mounted
form with its captured opening still current. The list refresh runs independently
of coordinator completion, so a pending GET does not keep a newer draft busy
after the accepted write has settled. Callback identities
remain stable during ordinary field changes so effect cleanup does not clear
current dirty state. A section remount starts a separate owner. A polling removal/restoration can
remount a form without another explicit opening; the form mount guard also
prevents that earlier completion from closing the replacement.

Contributor identity also distinguishes mounted forms. Reusing the same logical
instance ID and identical revision must not attach an old failure to a newer
form or report a newer dirty form safe to leave. The shared coordinator is not
changed. Testing/saving state, baseline, and inline test results are form-local;
unmounted producers cannot publish into replacement form state.

## Submission and recovery

The form captures the submitted workspace, instance and fields before awaiting
fetch. Existing success/error toasts remain unconditional for accepted writes.
Only current-opening closure/dirty state is conditional. Current errors preserve
the draft and retry path. The list keeps existing request-versioned refresh.
A cancelled accepted write is never aborted or replayed. No new persistence,
authentication, credential handling, metrics, or server contract is needed.

## Desktop and phone

Reuse the shipped dedicated Settings/Sentry page and vertically grouped inline
instance form. Both viewports share this owner and the fixed safe-area-aware
floating Save surface. Existing settings-content scrolling, labelled controls,
tap targets, and navigation remain unchanged. This state-only repair introduces
no layout or input modality branch. Browser proof uses desktop clicks and native
emulated phone taps, a disposable mock-Sentry backend, and a held causal response.
It does not establish physical device, OS keyboard, or real-provider behavior.

## Implementation record

[Plan](../../../plans/sentry-instance-form-ownership/plan.md).
