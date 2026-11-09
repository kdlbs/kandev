---
status: draft
system: platform
created: 2026-10-26
owners:
  - Kandev
---

# Interactive rate-throttle retry requirements

## Overview

An explicit transient rate limit can omit a reset time. Interactive conversations
need bounded recovery under the existing prompt-safety rules.

## Requirements

### REQ-PLATFORM-RATE-THROTTLE-RETRY-001: Bounded interactive throttle recovery

- **AC-PLATFORM-RATE-THROTTLE-RETRY-001.1:** A current high-confidence
  `rate_limited` failure without a reset deadline shall be eligible for the
  concrete-profile interactive retry loop, including Quick Chat. Eligibility
  remains subject to existing delivery, replay, and continuation safety gates.
- **AC-PLATFORM-RATE-THROTTLE-RETRY-001.2:** An eligible episode shall allow at
  most five additional attempts, using nominal delays of 5, 10, 20, 40, and
  60 seconds. A valid future reset within 60 seconds retains existing handling.
  Longer, expired, or invalid supplied deadlines shall not gain eligibility
  through this missing-deadline rule.
- **AC-PLATFORM-RATE-THROTTLE-RETRY-001.3:** Quota, usage, authentication,
  unknown, and low-confidence failures shall not gain automatic retries.
  Uncertain delivery, stale evidence, and unsafe replay shall remain blocked.
  Provider-owned retry activity shall not start a concurrent Kandev retry loop.
- **AC-PLATFORM-RATE-THROTTLE-RETRY-001.4:** Existing cancellation, supersession,
  exhaustion, reload, and shutdown behavior shall apply. Desktop and phone shall
  show the backend-owned retry notice and Cancel action, without duplicate
  user-message rows or restoration of an accepted prompt into the composer.
- **AC-PLATFORM-RATE-THROTTLE-RETRY-001.5:** This eligibility extension shall not
  alter Office, dynamic-candidate, utility, or passthrough retry policy. It shall
  not change the selected model, profile, provider, or account.

This is a policy extension to the earlier exclusion on retry-policy changes.
It does not change budgets, scheduling ownership, or provider-native recovery.
[Design](../system-design/provider-error-recovery.md#interactive-throttles-without-reset-metadata).
[Implementation package](../../../plans/quick-chat-draft-and-rate-retry/plan.md).

## Out of scope

New retry owners, replay permissions, provider switching, Office policy changes,
and interpretation of raw provider error Data.
