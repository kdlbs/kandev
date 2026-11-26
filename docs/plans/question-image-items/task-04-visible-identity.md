---
id: "04-visible-identity"
title: "Visible image identity on the review stage"
status: done
wave: 3
depends_on:
  - "03-item-review"
plan: "plan.md"
requirements:
  - REQ-TASKS-QUESTION-IMAGE-ITEMS-004
acceptance_criteria:
  - AC-TASKS-QUESTION-IMAGE-ITEMS-004.1
  - AC-TASKS-QUESTION-IMAGE-ITEMS-004.2
  - AC-TASKS-QUESTION-IMAGE-ITEMS-004.3
system_design:
  - ../../specs/tasks/system-design/question-image-items.md
---

# Task 04: Visible image identity

## Summary

Issue #5495, identity half. While a pair or operator comparison is on the stage,
badges always show which image, or which side, is visible. The status region
announces the same facts. Design: "Visible identity".

## Scope

- `visibleIdentity` in `lib/question-review/identity.ts`, a pure function over
  `effectiveCompareState`.
- `IdentityBadges` on the canvas overlay: one badge in Toggle and hold, one per
  side in Wipe (follows the divider, clear of the handle and Mark area), one per
  pane in Side by side, one in Onion skin and Difference.
- The stage announcement uses `visibleIdentity` for every mode.
- Forced-colors borders; slot-specific design tokens for A and B.
- Localized copy in all locales.

## Exclusions

Badges on singles and gallery items, which show one unambiguous image with its
description.

## UI preview

From [the plan](plan.md#ui-02-desktop-review-view-on-a-pair-opened-from-a-late-form):

```text
|  | [A · before]                                                  [✎ Mark area] |
|  | [A · before]            ‖            [B · after]               [✎ Mark area] |
```

## Acceptance

1. Each mode shows the identity required by AC-ITEMS-004.1, and flip, swap, pick, step,
   mode change, item change, and hold update it immediately.
2. The status region announces the identity; badges stay visible in forced colors
   and never cover the wipe handle, zoom cluster, or Mark area.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/question-review/identity.test.ts components/task/chat/question-review)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/chat/question-image-review.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome e2e/tests/chat/mobile-question-image-review.spec.ts)
```

E2E: on a labelled pair, flip, swap, and switch to Wipe, asserting badge text after
each step; on phone, assert the badge does not overlap the bottom bar or zoom chip.

## Likely files

- New `apps/web/lib/question-review/identity.ts` and `identity.test.ts`
- New `apps/web/components/task/chat/question-review/review-identity-badges.tsx`
- `apps/web/components/task/chat/question-review/review-compare-stage.tsx`,
  `review-compare-status.tsx`
- `apps/web/locales/*/task.json`
- E2E specs

## Risks

- Badge placement in Wipe at extreme divider positions; clamp and test at 0% and
  100%.

## Results

Implemented on 2026-11-24: `lib/question-review/identity.ts`,
`review-identity-badges.tsx`, and identity-based announcements in
`review-compare-status.tsx`. Unit tests: `identity.test.ts` and the identity cases in
`question-review-items.test.tsx`. E2E: desktop flip, swap, and Wipe badges clear
the handle; the phone badge stays above the zoom chip and bottom bar.

Review follow-up (2026-11-25): badges wrap instead of truncating and sit below
Mark area; Wipe uses a fixed Left/Right legend; announcements carry the same slot
names as the badges, so images with the same description stay distinct.
