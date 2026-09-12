---
id: "02-explain-workflow-attention"
title: "Explain workflow attention"
status: done
wave: 2
depends_on:
  - "01-collect-workflow-attention"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.2
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.3
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.4
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.5
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.6
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.7
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.8
system_design:
  - ../../specs/integrations/system-design/github-workflow-attention.md
---

# Task 02: Explain Workflow Attention

## Summary

Show a readable approval reason in existing GitHub PR surfaces.
Keep real failures visible and provide the same information through the phone drawer.

## In scope

- Add the frontend observation type and shared interpretation of attention, stale evidence, and legacy payloads.
- Update task summary, icon, chip, registered GitHub adapter, popover, and detail presentation.
- Prevent approval-only "Fix CI" suggestions and retain the strict merge readiness predicate.
- Localize all copy and generate Traditional Chinese catalogs with `pnpm run i18n:zh-hant`.
- Extend desktop/mobile regression fixtures and tests without replacing existing scenarios.
- Update the GitHub status explanation in `docs/public/integrations.md` through `/docs-maintainer`.

## Out of scope

- New drawers, task-row touch targets, provider actions, or a plugin SDK redesign.
- Changing unrelated provider status derivation.

## Acceptance

1. Summary regression tests show the approval reason for zero checks, keep real failures visible, and remove redundant raw `unstable` copy.
2. Desktop and mobile E2E prove the GitHub link, reload persistence, status recovery, and existing navigation/dismissal behavior.
3. Counts and merge readiness remain correct for mixed, terminal, draft, unknown, stale, and multi-PR states.

## ASCII UI preview

UI-01 and UI-02 excerpts from the [combined previews](plan.md#ascii-ui-preview):

```text
Desktop task summary
PR #143
CI  (!) Awaiting maintainer approval

Phone: task -> PR chip -> existing drawer
PR #143
CI: Awaiting maintainer approval
Run tests
This workflow has not started.
[View on GitHub]
```

For mixed and unavailable states, follow UI-03 in the plan.
Criteria 001.3, 001.4, 001.6, and 001.8 own these outcomes.
The phone uses the existing drawer scroll owner and safe-area padding.
The provider link meets the 44px touch target, and desktop controls retain their existing density.

## Verification

Run from the repository root. Install dependencies once if this worktree has no completed install.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/github/pr-task-status-summary.test.ts components/github/pr-task-icon.test.ts components/github/pr-status-chip.test.tsx components/github/pr-ci-popover.test.ts components/github/pr-detail-panel.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/pr/pr-status-badge.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/pr/mobile-pr-ci-chip.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run any additional changed unit suite by exact path and record it here.
The managed E2E runner rebuilds production assets. Confirm test discovery in both projects.

## Files likely touched

- `apps/web/lib/types/github.ts` and the GitHub registered-provider adapter
- `apps/web/components/github/pr-task-status-summary.tsx`, `pr-task-icon.tsx`, `pr-status-chip.tsx`
- `apps/web/components/github/pr-ci-popover.tsx`, `pr-detail-panel.tsx`, and related tests
- Existing `apps/web/components/integrations/change-request-*` anatomy when generic status support requires it
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/github.json` and the owning shared-copy namespace
- `apps/web/e2e/helpers/api-client.ts` and the two PR E2E files above
- `docs/public/integrations.md`

## Dependencies

Task 01 provides the stored observation and mock workflow evidence.

## Risks

Legacy payloads can omit the observation. Feedback must not erase newer stored evidence.
Shared components serve other providers, so the GitHub change must not replace their state logic.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/github-workflow-attention.md)
- [Design](../../specs/integrations/system-design/github-workflow-attention.md)
- Existing PR status chip drawer and `mobile-pr-ci-chip.spec.ts`
- `/mobile-parity`, `/e2e`, and scoped web guidance

## Results

Implemented and verified on 2026-09-10. Existing GitHub PR summary, icon, chip, popover, detail, and mobile drawer surfaces now explain current workflow attention, preserve real failures, distinguish unavailable evidence, and suppress unexplained raw `unstable` output. Localized copy, mock fixtures, desktop/mobile E2E coverage, and public integration guidance are included.

Validation passed:

- The focused frontend suite passed 149 tests, including workflow interpretation, notice, and duplicate-row identity coverage.
- `pnpm run typecheck`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet`.
- Desktop PR E2E passed 10 tests; mobile PR E2E passed 8 tests.
- Public documentation tests passed 62 tests; `node scripts/validate-public-docs.mjs` accepted 46 published pages.
- `python3 scripts/lint-spec-files.py --all`.
- `git diff --check`.
- Dedicated desktop and mobile approval-attention screenshots were captured and visually reviewed from `.pr-assets`.

PR-fixup validation also passed the focused frontend suite after review remediation, `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet`. The managed desktop and mobile approval scenarios each passed after the final UI changes.

The exact-head review remediation adds coverage for newer stored `none` observations clearing cached approval,
newer stored approval reappearing over an older cached clear, changed-head cached feedback fallback, and the
existing stale-positive behavior for newer unknown reads. This is a data-selection correction with no layout or
touch interaction change, so the existing mobile browser scenario remains the relevant coverage.
