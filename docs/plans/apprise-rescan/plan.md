---
created: 2026-09-09
status: done
requirements:
  - REQ-PLATFORM-APPRISE-RESCAN-001
system_design:
  - ../../specs/platform/system-design/apprise-rescan.md
legacy_specs: []
---

# Implementation Plan: Apprise Rescan

## Overview

Deliver one vertical change: detect a newly installed Apprise from Notifications
settings while preserving drafts. Execute one work order sequentially.

## Scope

In scope: manual detection, pending/result/error feedback, draft preservation,
translations, desktop/mobile proof, and a short public how-to.

Out of scope: installation, imported configuration, automatic polling, new
endpoints, notification delivery, and unrelated settings changes.

## Technical approach

Follow the [system design](../../specs/platform/system-design/apprise-rescan.md).
Extend `useNotificationProviders` with an availability-only refresh. Add a
narrow store setter and separate availability from guarded draft hydration.
Wire a labeled button in `ExternalProvidersSection` and preserve open forms.
The backend already evaluates `exec.LookPath("apprise")` for every list request.

## Tests

- `use-notification-providers.test.tsx`: missing-to-detected, detected-to-missing,
  duplicate clicks, failed request followed by retry, and delayed response after
  provider data changes (AC 001.2 through 001.4).
- `notifications-settings-actions.test.tsx`: dirty name, URL, event, removal,
  and create-form drafts survive changed availability (AC 001.4).
- `notifications-settings.test.tsx`: action in both availability states,
  feedback, unchanged save dirtiness, and retained open form (AC 001.1-001.4).

## E2E tests

- `settings/apprise-rescan.spec.ts`, project `chromium`: click Rescan Apprise,
  observe fresh detection, and retain an unsaved event selection; cover retry
  after a failed request (AC 001.1-001.4).
- `settings/mobile-apprise-rescan.spec.ts`, project `mobile-chrome`: tap the
  action, observe detection and Add Apprise Provider, assert a 44px touch target
  and no document overflow (AC 001.2, 001.5).

Use controlled GET responses and deferred completion for deterministic checks.
Do not install Apprise or send remote notifications from browser tests.

## Work orders

- [x] [Task 01: Add Apprise rescan](task-01-apprise-rescan.md) (`done`)

## Verification results

Implementation completed.

- Targeted Vitest: 4 files, 37 tests passed.
- TypeScript typecheck passed.
- Targeted ESLint passed with zero warnings.
- `i18n:zh-hant` and `i18n:check` passed for all required catalogs.
- Managed Chromium and mobile-chrome E2E scenarios passed, including fresh
  desktop and mobile captures.
- Public-doc tests and validation passed for 46 pages.
- Specification lint and `git diff --check` passed.

## Risks

- An executable installed outside the running backend's PATH stays undetected.
- Updating the full provider store from a delayed rescan could revert a save;
  the availability-only setter prevents that overwrite.
- Both notifications source files are already large. Extract only a focused
  control if needed to meet lint limits; avoid broad component refactoring.
