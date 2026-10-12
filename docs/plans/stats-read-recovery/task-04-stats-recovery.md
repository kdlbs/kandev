---
id: "04-stats-recovery"
title: "Recover failed statistics sections"
status: done
wave: 1
depends_on:
  - 03-analytics-admission
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INTERACTIVE-READS-003
acceptance_criteria:
  - AC-PLATFORM-INTERACTIVE-READS-003.1
  - AC-PLATFORM-INTERACTIVE-READS-003.2
  - AC-PLATFORM-INTERACTIVE-READS-003.3
  - AC-PLATFORM-INTERACTIVE-READS-003.4
  - AC-PLATFORM-INTERACTIVE-READS-003.5
system_design:
  - ../../specs/platform/system-design/interactive-read-availability.md
---

# Task 04: Recover failed statistics sections

## Summary

Add bounded per-section recovery and localized Retry controls to the existing Stats page.
Keep successful cards and prevent stale-selection responses from appearing.

## In scope

- Own Stats hook retry state, generation cancellation, foreground/manual coalescing, error classification, rendered retry status, and Copy Stats gating.
- Own UI-02 desktop/phone tests and all locale entries.

## Out of scope

Other work orders, new product metrics, health-policy changes, and live-instance mutation.

## Acceptance

- A temporary section failure retries at most twice automatically, respects Retry-After, and preserves successful current-selection sections.
- Manual/foreground retry cannot duplicate an active section request; hidden/unmounted/changed-selection work cancels safely.
- Desktop and phone show accessible errors and usable Retry; Copy Stats enables only after all sections have valid current-selection data.

## Regression tests

Name hook regressions `recovers a transient failed section`, `stops after the retry budget`, `cancels retries on range and workspace change`, and `coalesces recovery triggers`. Use fake timers and deferred responses; distinguish middleware body.code from ApiError.errorCode. Cover permanent/auth/parse failures and aborts. Component tests assert retained cards, disabled retry while pending, keyboard activation, accessible status, and Copy Stats gating. Browser tests cover mixed success/failure and touch targets without document overflow.

## ASCII UI preview

UI-02, [full preview](plan.md#ascii-ui-preview):

```text
Activity
Could not load activity. [Retry]
```

Map to AC-PLATFORM-INTERACTIVE-READS-003.1/.2/.4/.5. Desktop uses its existing grid;
phone uses its current single-column cards. Retain prior chart data below the
notice when available. During retry show disabled Retrying status. Use at least
44px touch targets, with 28px desktop controls. No new overlay or scroll owner.

## Verification

Run from the repository root. Before the first pnpm command in a fresh worktree,
run `(cd apps && pnpm install --frozen-lockfile)`. New test paths below must be
created by this work order; verify test discovery before claiming a pass.

```bash
(cd apps/web && pnpm exec vitest run app/stats/stats-data.test.tsx app/stats/stats-page-client.test.tsx app/stats/stats-utils.test.ts lib/api/domains/stats-api.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/layout/stats-read-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/layout/mobile-stats-read-recovery.spec.ts tests/layout/mobile-stats-nav.spec.ts)
```

If additional test files are changed during extraction, add their exact commands
here and run them before completion. Record skipped external services as blockers
to that validation, never as passing evidence.

## Files likely touched

- `apps/web/app/stats/stats-data.tsx`, `stats-page-client.tsx`, `stats-sections.tsx`
- New `apps/web/app/stats/stats-data.test.tsx`, `stats-page-client.test.tsx`
- `apps/web/lib/api/domains/stats-api.ts` only if its request options need forwarding
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/stats.json`
- `docs/public/feature-status.md`: describe section retry after implementation ships
- New E2E files named in Verification.

## Dependencies

03-analytics-admission

## Risks

Metric compatibility, portable SQL, cancellation cleanup, and misleading performance claims. See the manifest risks.

## Parallelism

`sequential`

## Inputs

- Applicable requirements and system designs from frontmatter, read in full.
- Investigation evidence and current source pointers in [plan.md](plan.md).
- Existing tests adjacent to owned files; E2E uses `test-base`, API seeding, and causal waits.

## Results

Implemented bounded per-section Stats recovery. Each section has its own
generation and abort controller, preserves successful data while a retry is in
flight, honors `Retry-After`, retries temporary failures at 2 s and 5 s, and
stops after two automatic attempts. Foreground and manual retries coalesce with
active requests. Permanent, authorization, parse, and cancellation failures do
not loop. Copy Stats remains disabled until all seven sections contain current
data.

The incident reproduction supplied the RED evidence: one failed Stats request
left the page without a recovery path until a range or workspace change. GREEN
evidence is:

- `cd apps/web && pnpm exec vitest run app/stats/stats-data.test.tsx app/stats/stats-page-client.test.tsx app/stats/stats-utils.test.ts lib/api/domains/stats-api.test.ts` passes 33 tests in 4 files.
- `cd apps/web && pnpm run typecheck`, `pnpm run lint`, and `pnpm run i18n:check` pass.
- Desktop and phone Playwright recovery scenarios pass. They verify retained
  successful sections, automatic/manual recovery, Copy Stats gating, accessible
  error status, and mobile Retry touch targets.
- `cd apps/web && pnpm run build:e2e` passes the production E2E build.

The Stats browser checks use the disposable E2E profile and seeded fixture data;
they do not mutate a live instance.
