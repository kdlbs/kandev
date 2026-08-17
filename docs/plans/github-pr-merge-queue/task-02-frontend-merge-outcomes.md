---
id: "02-frontend-merge-outcomes"
title: "Frontend merge outcomes"
status: done
wave: 2
depends_on: ["01-backend-queue-aware-merge"]
plan: "plan.md"
spec: "../../specs/github-pr-merge-queue/spec.md"
---

# Task 02: Frontend Merge Outcomes

## Inputs

- `docs/specs/github-pr-merge-queue/spec.md`
- `docs/plans/github-pr-merge-queue/plan.md`
- `apps/web/AGENTS.md`
- `.agents/skills/mobile-parity/references/kandev-mobile-ui-language.md`

## Likely Files

- `apps/web/lib/api/domains/github-pr-api.ts`
- `apps/web/lib/api/domains/github-api.test.ts`
- `apps/web/components/github/pr-task-icon.tsx`
- `apps/web/components/github/pr-task-icon.test.ts`
- `apps/web/components/github/pr-merge-button.tsx`
- `apps/web/components/github/pr-merge-button.test.tsx`
- `apps/web/src/locales/*/github.json`

## Acceptance

- The existing merge action appears for direct-merge and queue-required PRs
  only after explicit successful checks and satisfied review requirements.
- Accepted outcomes show distinct localized merged or queued feedback, refresh
  PR state, and prevent duplicate submission; rejected requests remain retryable.
- Desktop, compact, and mobile Review surfaces reuse the same mutation and
  eligibility behavior with accessible, touch-usable controls.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- lib/api/domains/github-api.test.ts components/github/pr-task-icon.test.ts components/github/pr-merge-button.test.tsx && cd web && pnpm run i18n:check && pnpm run typecheck
```

## Dependencies And Risks

- Depends on Task 01's response contract.
- `mergeable_state=blocked` is overloaded; the frontend predicate must not turn
  failed protection, missing review, or changes-requested states into queue
  actions.
- Locale key parity is build-gated across all supported languages.

## Results

- `pnpm --filter @kandev/web test -- --run lib/api/domains/github-api.test.ts components/github/pr-task-icon.test.ts` passed: 96 tests.
- `pnpm run i18n:check` passed with all supported catalogs complete.
- `pnpm run typecheck` passed.
- Review remediation keeps GitHub's overloaded `blocked` state visually neutral
  and labels the pre-submit action `Merge PR`; only the accepted response claims
  that GitHub added the PR to its merge queue.
- Queue feedback is driven by GitHub's terminal `enqueued` result, not
  asynchronous `pending` acceptance.
- Focused terminal-outcome coverage passes for merged, queued, rejected, and
  duplicate-click paths. The full frontend suite passed 12,046 tests across
  1,455 files, with four skips.
