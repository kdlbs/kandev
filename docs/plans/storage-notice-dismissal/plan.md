---
created: 2026-11-16
status: implemented
requirements:
  - REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-004
system_design:
  - ../../specs/platform/system-design/delivery-storage-diagnostics.md
legacy_specs: []
---

# Implementation Plan: Storage notice dismissal

## Overview

Add a dismiss button to the Storage row above the composer. One sequential work order delivers the UI, persistence, tests, and user guidance.

## Scope

Include task-scoped dismissal for the browser tab session, desktop and phone controls, and failure-tolerant browser persistence.
Exclude backend changes, cleanup, telemetry changes, automatic recovery, and persistent account preferences.

## Technical approach

Extend `SessionDeliveryStorageNotice` and pass task identity through `ComposerStatusNotices`.
Follow `PRMergedBanner` and its helpers in `lib/local-storage.ts`, using a separate storage prefix.
Reuse `task:dismiss`, `IconX`, and the existing sizing and focus conventions.
Do not build a generic banner framework. Existing local state and session storage satisfy the requested lifetime.
Update the low-disk guidance in `docs/public/sessions-and-review.md` during implementation.

Platform owns this extension to its diagnostics contract. System-page owns maintenance and remains unchanged.
The user requested parity with merged PR dismissal. The selected scope and lifetime follow that existing behavior.
A dismissed row remains hidden through later observations, including new pressure. Independent recovery cards remain visible.

## ASCII UI preview

### UI-01: Storage notice above the composer

```text
Desktop, visible:
  ! Disk 90% used   [Storage] [Details v] [x]
  [ Existing composer                    ]

Phone, visible:
  ! Disk 90% used
    [Storage] [Details v] [x]
  [ Existing composer      ]

Desktop and phone, dismissed:
  [ Existing composer ]
```

The phone label may wrap within the row. The drawing illustrates wrapping, not a required second action row.
Control order and a visible final dismiss button are required. Spacing and example copy are illustrative.
Phone and coarse-pointer controls have at least 44px hit targets. The desktop dismiss icon retains compact sizing.
Use the existing inline status region and scroll owner. No overlay or extra scroll region is added.
Expanded Details disappears with the row. Independent historical recovery cards remain available.
This preview maps to criteria 004.1, 004.3, and 004.4 and both storage diagnostics Playwright specs.

## Tests

- `lib/local-storage.test.ts`: task isolation, separate PR keys, missing storage, read/write exceptions, cleanup ownership.
- `session-delivery-storage-notice.test.tsx`: immediate dismissal and expanded-details removal (004.1), remount and task-identity changes (004.2), later observation updates and preserved health (004.3), accessible name (004.4), failed persistence (004.5).
- Existing PR banner tests remain the regression check for the reused pattern.

## E2E tests

Extend `apps/web/e2e/tests/session/delivery-storage-diagnostics.spec.ts` in `chromium` for keyboard dismissal, reload with fresh warning injection, task switching, and retained draft.
Extend `mobile-delivery-storage-diagnostics.spec.ts` in `mobile-chrome` for tap dismissal, local and remote rows, 44px targets, row containment, and no horizontal overflow.
Add a narrow fine-pointer case to the desktop spec. Check 767px and 768px boundaries and long labels.
Use existing scoped WebSocket fixtures. Inject a fresh warning after reload so absence cannot pass because the fixture stopped sending data.
Cover the full task chat and Quick Chat composition where the shared notice mounts.
These flows cover criteria 004.1 through 004.4. Unit tests cover storage-denial behavior under 004.5.

## Work orders

- [x] [Task 01: Add task-scoped dismissal](task-01-dismiss-storage-notice.md)

One work order, no dependencies, sequential execution. Exact commands are in the work order.

## Companion records

The original [diagnostics package](../delivery-storage-diagnostics/plan.md) and [recovery package](../delivery-storage-recovery/plan.md) retain their completed scope and results.
This package adds dismissal and its tests. It does not repeat their backend implementation or claim new runtime verification.

## Verification results

Design validation passed on 2026-11-16:

- `python3 scripts/list-docs.py validate`: 452 decisions and 2,014 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `validateCoverage`: documentation diff exempt, representative planned source diff covered with no errors.
- `git diff --check -- docs/specs docs/plans`: passed. Both new plan files are present and untracked.

Implementation completed on 2026-11-16. Task 01 is done.

- Dependency installation passed with the frozen lockfile.
- Focused Vitest command: 64 tests passed across the three specified files. The three new component regressions first failed because Dismiss was absent.
- Typecheck, targeted ESLint (including the new browser helper), i18n:check, and i18n:ratchet passed.
- Public documentation validation passed for 55 pages.
- Managed Chromium and mobile-chrome runs passed all three tests each. Final expanded dismissal scenarios passed again in both projects with captures enabled.
- Tests prove keyboard/tap dismissal, reload, task isolation, draft retention, Quick Chat, phone target dimensions, and 767px/768px sizing boundaries.
- Desktop and phone local and long-remote-label screenshots were inspected. Controls fit within the notice and preserve the composer.
- Actual-diff documentation coverage preflight passed with no errors. Specification and whitespace checks passed.

The first host run inherited a non-loopback bind and failed readiness. A localhost retry exposed missing Chromium library search paths.
Successful runs used `KANDEV_SERVER_HOST=127.0.0.1` and `LD_LIBRARY_PATH` resolved from installed Nix libraries.
The initial managed run built fresh backend, web, and fixtures. Later runs used `--no-build` with those same artifacts.
No source changes were needed for either host environment issue.
The Quick Chat fixture now injects live health because an active runtime can replace a seeded detail snapshot.

Exact successful browser commands, with the host environment above:

```bash
(cd apps/web && pnpm e2e:run --no-build --project chromium tests/session/delivery-storage-diagnostics.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/session/mobile-delivery-storage-diagnostics.spec.ts)
(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --no-build --project chromium tests/session/delivery-storage-diagnostics.spec.ts -- --grep 'dismisses Storage')
(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --no-build --project mobile-chrome tests/session/mobile-delivery-storage-diagnostics.spec.ts -- --grep 'dismisses Storage')
```

## Risks

- Task identity changes can retain the previous task's local dismissal unless explicitly reset.
- Storage denial limits persistence to the mounted component.
- Adding an action can crowd narrow or translated rows. Browser geometry checks must cover local Storage links and long remote labels.
