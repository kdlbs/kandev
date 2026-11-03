---
id: "01-detail-binding-decision"
title: "Correct the GitHub PR detail binding decision"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-DETAIL-DECISION-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-DETAIL-DECISION-001.1
  - AC-INTEGRATIONS-GITHUB-PR-DETAIL-DECISION-001.2
  - AC-INTEGRATIONS-GITHUB-PR-DETAIL-DECISION-001.3
  - AC-INTEGRATIONS-GITHUB-PR-DETAIL-DECISION-001.4
system_design:
  - ../../specs/integrations/system-design/github-pr-detail-review-decision.md
---

# Task 01: Correct the GitHub PR detail binding decision

## Summary

Make the existing detail header reflect binding GitHub review history while
leaving review bodies and current reviewer actions intact. Establish the
independent user-visible regression before changing production.

## In scope

- Derive the detail-only decision using the existing binding reducer as the
  reference, with specified fallback and ordering semantics.
- Verify actual desktop detail and phone Review consumer, plus focused controls.

## Out of scope

Backend and status, counts, `has_issues`, watchers, GitLab, review-request
lifecycle, persisted mutations, authorization, merge gates, query sync, copy,
layout and neutral-body filtering.

## Acceptance

1. Before production changes, an independently authored actual rendered test
   fails on approval then same-author comment while an identical approval-only
   control passes. The follow-up body is visible in the causal path; test uses real
   `PRDetailContent`, `ChangeRequestDetail`, provider and hook, mocking only
   feedback HTTP response and required WS/panel-title transport.
2. After the focused correction the regression passes; change-request after
   comment remains visible, both same-author binding transitions and mixed
   authors behave per the existing reducer, and neutral-only, dismissed-only,
   no-review, requested-reviewer, case, ordering, and equal-time controls do
   not fabricate approval or alter actions. Include source-order nonbinding,
   anonymous and timestamp cases consistent with the design and existing Go
   reducer, including current requested/re-request controls.
3. A focused rendered check goes through actual `MobileReviewPanel` and
   `ReviewDetailPanelComponent` for a selected GitHub PR and verifies the
   decision and comment body. Existing phone navigation and action behavior are
   preserved. Record HappyDOM's limits and actual test counts.

## ASCII UI preview

UI-01 / UI-02, AC .1-.4 ([full preview](plan.md#ascii-ui-preview)):

```text
Desktop: [PR title] [Approved]     Phone Review: [PR selector]
         reviewer: follow-up                     [PR title] [Approved]
                                                 reviewer: follow-up
                                                 [Review navigation]
```

## Verification

Only after ROOT's full design review and a later separate implementation
instruction plus exclusive heavy-work grant, set PATH and install once:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH"
(cd apps && pnpm install --frozen-lockfile)
```

Follow `/tdd`, `/mobile-parity`,
and applicable scoped guidance. Each long run uses `scripts/run-quiet` with a
unique task-owned `KANDEV_RUN_QUIET_DIR`, and retain every native handle, join,
exit, collected count and logs. The wrapper may delete old logs inside that
directory, so each invocation must have a new owned directory. No overlapping heavy commands.

From repository root, after writing the independent regression and before
production, and again after correction (choose unique owned log directories for
each):

```bash
(cd apps/web && pnpm exec vitest run --project browser-locales --maxWorkers=1 --no-file-parallelism components/github/pr-detail-review-decision.test.tsx)
```

Run the new phone-consumer test in its actual configured project; if combined
with the new test above, record both collected scenarios. The following current
reviewed suites belong to browser, while `pr-detail-panel.test.ts` is not in
the reviewed list and is browser-locales; confirm the project map at execution:

```bash
(cd apps/web && pnpm exec vitest run --project browser --maxWorkers=1 --no-file-parallelism components/github/pr-reviews-section.test.tsx components/task/mobile/session-mobile-layout.test.tsx)
(cd apps/web && pnpm exec vitest run --project browser-locales --maxWorkers=1 --no-file-parallelism components/github/pr-detail-panel.test.ts)
```

Run scoped ESLint for every actually changed TS/TSX file, including any helper
and new tests, with explicit paths: `(cd apps/web && pnpm exec eslint --max-warnings 0 <changed-file-paths>)`.
A canonical web typecheck requires a fresh separate cap grant; earlier 4 GiB
OOM and 6 GiB pass are only prior evidence, not a new run. After authorized
staging of all actual additions, run `(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet && pnpm run e2e:sleep-ratchet)`;
run the current documentation coverage validator against the actual staged
changes and complete referenced documents, and normal active hooks. The design
checkpoint used `.github/scripts/pr-docs.cjs` `validateCoverage` with the four
documents and the anticipated panel path; repeat using actual paths, without
claiming the simulated scope is the final diff. Before
staging, run:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short
```

Record red/green outcomes and discovered tests, exact commands, status, public
docs assessment and any blocker. Unknown/resource/auth/OOM/out-of-scope or
unchanged retry requires ROOT checkpoint, not silent retries. Ordinary owned
fixture/type/lint correction at an approved cap remains within the granted
release. Do not run broader suites without a reason.

## Files likely touched

- `apps/web/components/github/pr-detail-panel.tsx` and, if warranted, a scoped
  pure helper near this GitHub domain.
- `apps/web/components/github/pr-detail-review-decision.test.tsx`, plus a
  phone consumer test if separate.
- This plan, work order, and linked integration requirements and design.

## Dependencies

ROOT must fully review all four artifacts and refinements; a later separate
implementation instruction and exclusive heavy grant are required. None of the
work above authorizes delegation.

## Risks

The backend binding reference's ordered iteration and anonymous identities are
subtle. A simplistic latest-row or all-approved reducer loses approvals or
masks active change requests. Existing REST status may still disagree.

## Parallelism

Sequential.

## Inputs

Linked requirement, system design, real component, provider, hook and existing
Go reducer. Qualification receipt is factual only; private candidate and logs
are prohibited.

## Results

- RED: `browser-locales` decision suite collected two tests; causal header
  assertion failed after the comment body rendered and approval-only passed.
- GREEN: `browser-locales` decision and existing detail suites: 40 passed.
  The decision suite includes the actual selected phone consumer and visible
  follow-up body. The existing `browser` review section and mobile layout
  suites passed 33 tests. HappyDOM does not prove Chromium behavior.
- Scoped ESLint passed for both changed TSX files. Canonical web typecheck passed
  under `NODE_OPTIONS=--max-old-space-size=6144`; the first attempt exposed a
  missing required fixture workspace ID, corrected before the passing run.
- No public documentation or screenshot change is needed for the unchanged
  documented workflow and terminology. Catalog and staged ratchets are part of
  the final delivery checks.
