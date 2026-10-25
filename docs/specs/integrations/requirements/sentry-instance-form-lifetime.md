---
status: active
system: integrations
created: 2026-10-22
owners:
  - kandev
---

# Sentry Instance Form Lifetime Requirements

## Overview

Integration settings owns the Sentry configuration draft and accepted writes.
A user can cancel a saving form and open another form before the response arrives.
The current draft must remain usable when that earlier request settles.
The shared [manual-save contract](../../ui/requirements/settings-manual-save.md)
continues to apply.

## Terminology

- **Opening:** One continuous presentation of an add or instance-edit form.
  Reopening the same instance starts another opening, even with identical text.
- **Accepted write:** A create or update already submitted through Save.

## Requirements

### REQ-INTEGRATIONS-SENTRY-FORM-LIFETIME-001: Current Sentry form continuity

**Intent:** Let users continue configuring Sentry while earlier writes settle.

#### Acceptance criteria

- **AC-INTEGRATIONS-SENTRY-FORM-LIFETIME-001.1:** After an accepted create or update, cancelling and opening an add,
  another instance edit, or the same instance edit shall preserve the newer
  opening's exact name, URL, token, selection, dirty registration, and usable
  Save and Cancel controls when the earlier request succeeds or fails.
- **AC-INTEGRATIONS-SENTRY-FORM-LIFETIME-001.2:** Cancelling or unmounting an opening shall not cancel its accepted
  write or retarget its captured workspace, instance, or payload. Successful
  writes shall refresh the instance list and retain truthful success feedback;
  failures shall retain truthful error feedback.
- **AC-INTEGRATIONS-SENTRY-FORM-LIFETIME-001.3:** A successful current unchanged opening shall close and become
  clean. A failed current opening shall retain its draft and support retry.
  Cancelling an unsaved current opening shall remove its dirty registration
  without a write or a spurious navigation blocker.
- **AC-INTEGRATIONS-SENTRY-FORM-LIFETIME-001.4:** Desktop and phone users shall receive the same ownership and
  recovery behavior through the existing settings controls.

## Out of scope

Other integrations, watcher filters, shared coordinator redesign, Query/store
migration, backend schemas, credential policy, request cancellation, global
journals, cross-tab coordination, and transport exactly-once guarantees.
