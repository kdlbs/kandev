---
id: "01-review-fixes"
title: "Apply workspace pause review fixes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-KILL-SWITCH-002
  - REQ-OFFICE-KILL-SWITCH-003
  - REQ-OFFICE-KILL-SWITCH-005
  - REQ-OFFICE-KILL-SWITCH-006
acceptance_criteria:
  - AC-OFFICE-KILL-SWITCH-003.3
  - AC-OFFICE-KILL-SWITCH-003.6
  - AC-OFFICE-KILL-SWITCH-003.7
  - AC-OFFICE-KILL-SWITCH-005.6
  - AC-OFFICE-KILL-SWITCH-006.4
  - AC-OFFICE-KILL-SWITCH-006.6
  - AC-OFFICE-KILL-SWITCH-006.8
  - AC-OFFICE-KILL-SWITCH-006.9
  - AC-OFFICE-KILL-SWITCH-006.11
system_design:
  - ../../specs/office/system-design/workspace-kill-switch-01.md
  - ../../specs/office/system-design/workspace-kill-switch-02.md
---

# Task 01: Apply workspace pause review fixes

## Scope

- Require workspace management authorization for pause and resume mutations.
- Keep active Office task sessions discoverable for a repeated halt sweep.
- Return sweep failures through the API and show a localized retry action on
  desktop and mobile Office surfaces.
- Reject malformed JSON request bodies and keep the empty resume-body contract.
- Update the internal design record, copy, and focused regression tests.
- Preserve the cached pause record while re-reading the same workspace after
  a remount. Clear it when the workspace changes, and distinguish a pending
  read from a failed read so only failures show the unavailable warning.
- Update the cached workspace owner on an accepted response, and ignore an
  older failed read only when a newer read can settle its state. A failed POST
  leaves the last-applied tag unchanged so an outstanding GET can settle in
  either completion order; accepted successes still supersede older failures.

## Verification

- Focused backend Go tests for the pause handlers, sweep, scope middleware, and
  SQLite/PostgreSQL repository paths.
- Focused frontend API and hook tests, i18n checks, typecheck, and spec lint.
- Permanent regressions use the real app store, hook, API normalization and
  banner with deferred HTTP responses. They cover cached remounts, workspace
  changes, cold-read errors, obsolete reads, both GET/failed-POST completion
  orders, and successful read/mutation supersession. Store cases separately
  cover accepted GET/POST cache-owner synchronization.
- Focused command from `apps/web`: `pnpm exec vitest run
  hooks/domains/office/use-workspace-pause-lifecycle.test.tsx
  hooks/domains/office/use-workspace-pause.test.tsx
  lib/state/slices/office/office-pause.test.ts
  app/office/components/workspace-pause-banner.test.tsx`.
- These changes normalize state within the existing desktop topbar and phone
  workspace-actions drawer. The nearest phone exemplar is
  `components/kanban/mobile-menu-sheet.tsx`. No layout or touch behavior changes;
  targeted rendered lifecycle coverage satisfies the mobile-parity state-only
  exception. Existing `mobile-workspace-kill-switch.spec.ts` covers touch
  pause/resume, 44px controls and horizontal overflow in hosted E2E.
- Validation is hosted for this review; results are recorded after completion.
