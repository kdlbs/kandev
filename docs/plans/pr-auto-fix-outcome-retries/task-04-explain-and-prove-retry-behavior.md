---
id: "04-explain-and-prove-retry-behavior"
title: "Explain and prove retry behavior"
status: done
wave: 3
depends_on:
  - "02-reconcile-turns-through-explicit-outcomes"
  - "03-upgrade-prompt-defaults-safely"
plan: "plan.md"
requirements:
  - REQ-UI-CI-PR-AUTOMATION-001
acceptance_criteria:
  - AC-UI-CI-PR-AUTOMATION-001.8
  - AC-UI-CI-PR-AUTOMATION-001.11
  - AC-UI-CI-PR-AUTOMATION-001.12
  - AC-UI-CI-PR-AUTOMATION-001.13
  - AC-UI-CI-PR-AUTOMATION-001.14
  - AC-UI-CI-PR-AUTOMATION-001.15
system_design:
  - ../../specs/ui/system-design/ci-pr-automation-01.md
  - ../../specs/ui/system-design/ci-pr-automation-03.md
---

# Task 04: Explain and Prove Retry Behavior

## Summary

Update the shared automation help and public review documentation, then prove
the completed retry flow on desktop and mobile.

## In scope

- Explain that an undispositioned or unverified round can retry and consume
  another slot, while non-actionable and blocked outcomes suppress an unchanged
  snapshot.
- Update English, Portuguese, Simplified Chinese, Hong Kong Traditional
  Chinese, and Taiwan Traditional Chinese locale catalogs.
- Update GitHub PR automation guidance in `sessions-and-review.md`,
  `integrations.md`, and the saved-prompt compatibility note in
  `developer-tools.md`.
- Extend focused component and Playwright coverage for round progression and
  help content.
- Run a final backend/frontend/docs verification sweep for the package.

## Out of scope

- Changing the existing popover/drawer hierarchy, scroll ownership, touch
  targets, or composer badges.
- Adding a user outcome control.
- GitLab public behavior changes.

## Mobile design contract

- **Desktop outcome:** the existing PR automation popover explains the retry
  and disposition rules next to the existing round count.
- **Mobile entry point:** tap the existing PR status chip and use the same help
  affordance in `PRStatusChipDrawer`.
- **Nearest exemplar:** `PRStatusChipDrawer` in
  `apps/web/components/github/pr-status-chip.tsx`; keep its inset drawer, fixed
  header, and single internal scroll body.
- **Composition:** content-only change. Shared help content and business state
  remain identical across viewports; no mobile-specific markup is added.
- **Proof:** desktop and `mobile-chrome` tests assert the retry explanation,
  round increment, drawer containment, and no document horizontal overflow.

## Acceptance

- Users can tell why the same failed checks may start another round and that
  each retry counts toward 10.
- Desktop and mobile expose the same explanation without a layout regression.
- Public docs distinguish in-flight dedupe from permanent acknowledgement.

## Verification

```bash
cd apps/web && pnpm exec vitest run components/github/pr-status-chip.auto-fix-rounds.test.tsx components/github/pr-ci-popover.automation.test.tsx
cd apps/web && pnpm e2e:run --project=chromium tests/pr/ci-automation-options.spec.ts
cd apps/web && pnpm e2e:run --project=mobile-chrome tests/pr/mobile-ci-automation-options.spec.ts
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm run i18n:ratchet
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
make -C apps/backend test
make -C apps/backend lint
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/github/pr-ci-automation-rows.tsx`
- `apps/web/components/github/pr-status-chip.auto-fix-rounds.test.tsx`
- `apps/web/components/github/pr-ci-popover.automation.test.tsx`
- `apps/web/src/locales/en/github.json`
- `apps/web/src/locales/pt-pt/github.json`
- `apps/web/src/locales/zh-cn/github.json`
- `apps/web/src/locales/zh-hk/github.json`
- `apps/web/src/locales/zh-tw/github.json`
- `apps/web/e2e/tests/pr/ci-automation-options.spec.ts`
- `apps/web/e2e/tests/pr/mobile-ci-automation-options.spec.ts`
- `docs/public/sessions-and-review.md`
- `docs/public/integrations.md`
- `docs/public/developer-tools.md`

## Dependencies

- Tasks 02 and 03.

## Risks

- Long help copy can overflow the mobile drawer. Reuse the existing scroll
  owner and keep copy concise.
- E2E must observe a real second round, not only mutate a mocked count.

## Parallelism

`sequential`

## Results

Updated the shared five-locale help copy, public GitHub automation guidance,
and MCP documentation coverage. Added component assertions plus desktop and
mobile Playwright coverage for retry explanations and round progression. The
full validation sweep passed without changing the existing mobile drawer or
desktop popover composition.
