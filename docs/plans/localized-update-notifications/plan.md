---
created: 2026-10-27
status: implemented
requirements:
  - REQ-PLATFORM-NOTIFICATIONS-001
system_design:
  - ../../specs/platform/system-design/localized-update-notifications.md
legacy_specs: []
---

# Implementation Plan: Localized Update Notifications

## Overview

The pseudo-locale CI suite exposed backend English fallback copy rendered by the
Local provider. Format a known release version using the active UI locale for
both browser and native delivery, retaining legacy fallback for events without
version metadata. This correction was found during PR #4863 CI remediation.

## Scope

Local Kandev release update notifications only. Runtime update notifications,
remote-provider payloads, navigation, layout, and provider selection remain
unchanged.

## Tests

The hook test exercises English backend fallback payloads in the pseudo locale
and native delivery. The pseudo-locale browser test injects the same wire shape
through the WebSocket fixture. Existing tests protect runtime and default-locale
behavior. Rendered geometry and mobile interactions do not change.

## Work orders

- [x] [Task 01: Localize release notifications](task-01-localize-release-notifications.md)

## Verification results

The focused unit suites passed 18 tests, the full pseudo-locale browser spec
passed 23 tests with retries disabled, and typecheck, i18n, lint, and
specification checks passed. The original CI shard also contains other specs;
the exact CI shard was not repeated locally because it uses four concurrent
processes. CI will rerun that shard on the updated head.

## Risks

A versioned Local event always uses client-generated release copy. Events
without version metadata retain the server-provided fallback.
