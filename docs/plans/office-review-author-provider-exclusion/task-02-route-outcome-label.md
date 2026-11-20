---
id: "02-route-outcome-label"
title: "Show the author-exclusion route outcome and prove it end to end"
status: done
wave: 2
depends_on:
  - "01-dispatch-author-exclusion"
plan: "plan.md"
requirements:
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-002
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-003
  - REQ-OFFICE-REVIEW-AUTHOR-EXCL-004
acceptance_criteria:
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-002.1
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-003.2
  - AC-OFFICE-REVIEW-AUTHOR-EXCL-004.1
system_design:
  - ../../specs/office/system-design/review-author-provider-exclusion.md
---

# Task 02: Show the author-exclusion route outcome and prove it end to end

## Summary

Add the `skipped_author_provider` outcome to the web route-attempt types and
the route panel, with a localized label in all seven catalogs. Add one
Playwright spec that drives a task from Work to Review on a workflow declaring
the exclusion, and asserts that the reviewer run launches on the non-author
provider.

## Scope

- `RouteAttemptOutcome` union, `OUTCOME_LABEL`, `OUTCOME_VARIANT`.
- `office:routeOutcomeSkippedAuthorProvider` in `en`, `pt-pt`, `zh-cn`,
  `zh-hk`, `zh-tw`, `ja`, and `ko` (use `pnpm run i18n:zh-hant` for the
  Traditional pair). No em dash.
- Vitest for the route panel rendering the new label.
- E2E: import a workflow (or update the seeded one) with
  `exclude_author_provider: true` on the review seat action. Use mock
  providers with order `[claude-acp, codex-acp]`. Assert that the reviewer
  run's resolved provider is `codex-acp` and that its events include
  `routing.author_exclusion`.

## Exclusions

Structured rendering of run events, workflow-editor controls, and phone-specific
layout changes (the panel's composition is unchanged).

## UI-01 (from [plan](plan.md#ui-01-run-detail-route-attempts-panel-parked-seat-run))

```text
| #1 (!) claude-acp/sonnet  [balanced] [skipped: wrote this task] |
```

## Implementation acceptance

- The route panel renders the localized label for `skipped_author_provider`,
  and the i18n checks pass in all catalogs.
- The E2E passes: the reviewer run launches on the provider that did not do
  the work.

## Verification

```bash
cd apps/web
pnpm exec vitest run --maxWorkers=1 app/office/components/routing
pnpm run i18n:check
pnpm run i18n:ratchet
pnpm run typecheck
pnpm e2e:run --host --shards 1 --project chromium tests/office/office-review-author-exclusion.spec.ts
```

## Likely files

- `apps/web/lib/state/slices/office/routing-types.ts`
- `apps/web/app/office/components/routing/route-panel.tsx` (+ new `route-panel.test.tsx`)
- `apps/web/src/locales/*/office.json`
- `apps/web/e2e/tests/office/office-review-author-exclusion.spec.ts`
- `apps/web/e2e/helpers/office-routing.ts` (reuse)

## Risks

- The E2E needs the task to reach Review with a seated reviewer on a mock
  provider. Reuse the routing-fallback spec's backend restart and mock
  providers, and the office seed's workflow import path.

## Results

- `skipped_author_provider` was added to `RouteAttemptOutcome`, the route
  panel label and variant, and all catalogs ("skipped: wrote this task"). It
  is covered by `route-panel.test.tsx`.
- E2E `office-review-author-exclusion.spec.ts` (chromium) passes. It
  temporarily sets the flag on the seeded Office workflow's review seat,
  creates a specialist reviewer, and checks that the reviewer run launches on
  the provider that did not run Work. It also checks that the run detail
  Events log shows `routing.author_exclusion` naming the author. The reviewer
  wait allows for one checkout-contention retry. The Work launch can
  legitimately fall back, so the spec asserts relative to the observed
  author.
- i18n check, ratchet, and typecheck pass.
