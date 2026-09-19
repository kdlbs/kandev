---
id: "02-frontend-merge-outcomes"
title: "Frontend merge outcomes"
status: done
wave: 2
depends_on: ["01-backend-queue-aware-merge"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-pr-merge-queue.md"
---

# Task 02: Frontend Merge Outcomes

## Inputs

- `docs/specs/integrations/requirements/github-pr-merge-queue.md`
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
- An active queue entry uses the dedicated `#966600` color before other
  non-terminal review, check, draft, dirty, or behind states. A failing sibling
  still wins multi-PR aggregation, terminal colors remain authoritative, and
  future non-empty queue states use generic queued copy.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- lib/api/domains/github-api.test.ts components/github/pr-task-icon.test.ts components/github/pr-merge-button.test.tsx components/github/pr-task-status-summary.test.ts components/github/pr-status-chip.test.tsx components/github/pr-merge-queue-status.test.tsx && cd web && pnpm run i18n:check && pnpm run typecheck
```

## Dependencies And Risks

- Depends on Task 01's response contract.
- `mergeable_state=blocked` is overloaded; the frontend predicate must not turn
  failed protection, missing review, or changes-requested states into queue
  actions.
- Locale key parity is build-gated across all supported languages.

## Results

- The six-file queue-status command passed 160 tests, including the queue
  precedence, failing-sibling, and future-state fallback regressions. The
  existing API and merge-button action tests remain covered by Task 02's
  frontend suite.
- `pnpm run i18n:check` passed with 7,223 referenced keys, 8,779 English
  entries, 48 orphans, and all supported catalogs complete.
- `pnpm run typecheck` passed.
- Review remediation keeps GitHub's overloaded `blocked` state visually neutral
  and labels the pre-submit action `Merge PR`; only the accepted response claims
  that GitHub added the PR to its merge queue.
- Queue state is checked after terminal state and before other non-terminal
  states in both task-icon and compact-chip status derivation. Multi-PR status
  ranking still lets a failing sibling dominate a queued sibling.
- Future provider queue enums map to the generic queued status rather than an
  `Unknown` label.
- Queue feedback is driven by GitHub's terminal `enqueued` result, not
  asynchronous `pending` acceptance.
- Focused terminal-outcome coverage passes for merged, queued, rejected, and
  duplicate-click paths. The full frontend suite passed 12,046 tests across
  1,455 files, with four skips.
