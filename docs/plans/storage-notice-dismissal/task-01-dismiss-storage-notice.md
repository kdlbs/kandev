---
id: "01-dismiss-storage-notice"
title: "Add task-scoped Storage notice dismissal"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-004
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-004.1
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-004.2
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-004.3
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-004.4
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-004.5
system_design:
  - ../../specs/platform/system-design/delivery-storage-diagnostics.md
---

# Task 01: Add task-scoped Storage notice dismissal

## Summary

Add the accessible dismiss action and remember it for the task in the browser tab session.
Preserve health observations, recovery restrictions, and the existing composer.

## In scope

- Browser persistence helpers, cleanup integration, task identity wiring, and local dismissal state.
- Desktop and phone button, targeted tests, and the existing public storage guidance.

## Out of scope

Backend changes, cleanup policy, new dependencies, account preferences, and recovery-card dismissal.

## Acceptance

1. Criteria 004.1 and 004.2 pass for both initial mounting and task identity changes.
2. Criteria 004.3 and 004.5 pass without modifying runtime health or losing the draft.
3. Criterion 004.4 passes on desktop, phone, and narrow fine-pointer layouts with translated accessible text.

## ASCII UI preview

See the [combined preview](plan.md#ascii-ui-preview).

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

## Verification

Use TDD. Run new behavior tests to failure before implementation, then run the complete block from the repository root.
Install dependencies once for this worktree. Managed browser runs build fresh artifacts and run sequentially.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/local-storage.test.ts components/task/chat/session-delivery-storage-notice.test.tsx components/task/chat/pr-archive-banners.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint lib/local-storage.ts components/task/chat/session-delivery-storage-notice.tsx components/task/chat/composer-status-notices.tsx)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/session/delivery-storage-diagnostics.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-delivery-storage-diagnostics.spec.ts)
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Inspect the focused desktop and phone captures for wrapping, control containment, focus, and retained composer layout.
Run documentation coverage preflight with `.github/scripts/pr-docs.cjs` `validateCoverage` against the actual changed paths and referenced documents.
Record all command results. Do not run broad verification unrelated to this work order.

## Files likely touched

- `apps/web/lib/local-storage.ts` and `local-storage.test.ts`.
- `apps/web/components/task/chat/session-delivery-storage-notice.tsx` and its test file.
- `apps/web/components/task/chat/composer-status-notices.tsx`.
- `apps/web/e2e/tests/session/delivery-storage-diagnostics.spec.ts` and `mobile-delivery-storage-diagnostics.spec.ts`.
- `docs/public/sessions-and-review.md`.

## Dependencies

None.

## Risks

Same-instance task changes must not inherit dismissal. Browser storage failures must not prevent immediate hiding.
Do not alter freshness timer behavior while adding the visibility condition.

## Parallelism

`sequential`

## Inputs

- [Requirement 004](../../specs/platform/requirements/delivery-storage-diagnostics.md#req-platform-delivery-storage-diagnostics-004-composer-notice-dismissal).
- [Dismissal design](../../specs/platform/system-design/delivery-storage-diagnostics.md#composer-notice-dismissal).
- `pr-archive-banners.tsx`, `lib/local-storage.ts`, and existing storage notice browser tests.
- Repository mobile-parity, better-ui, e2e, TDD, and docs-maintainer guidance.

## Results

Implemented the task-scoped tab-session dismissal and cleanup helpers, task identity wiring, localized icon action, and public guidance.
The notice resets local state on task identity changes without remounting the composer.
Browser storage failures preserve immediate local dismissal.

All specified checks passed. The focused unit command passed 64 tests.
Both complete browser specs passed three tests. The final dismissal scenario passed in each project with captures enabled.
The shared browser scenario lives in `apps/web/e2e/helpers/storage-notice-dismissal.ts` and covers task chat plus Quick Chat.
See [plan verification results](plan.md#verification-results) for commands, host environment corrections, and rendered evidence.
No backend or locale changes were required.

PR review follow-up: reload and return-to-task assertions now wait for the active session's warning in the app store.
The desktop scenario reaches Dismiss with Tab from the visible Details control and asserts focus before Enter.
Both focused browser scenarios passed again. Targeted ESLint and formatting checks passed.
