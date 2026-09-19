---
id: "01-localize-watcher-and-task-fallbacks"
title: "Localize watcher and task fallbacks"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/i18n-audit-watcher-copy.md"
---

# Task 01: Localize Watcher And Task Fallbacks

## Acceptance

- Every GitHub, GitLab, Jira, and Linear watcher profile default resolves localized copy during
  render while sentinel values and submitted empty IDs remain unchanged.
- Watcher repository empty/loading/default copy resolves during render while repository and branch
  domain values remain untranslated.
- A sender badge with no live or snapshot title renders the localized unknown-task fallback.

## Verification

```bash
cd apps/web && pnpm test -- --run lib/watcher-profile-default.test.ts lib/watcher-repository-default.test.ts components/task/chat/messages/chat-message.test.tsx components/github/review-watch-dialog.test.tsx components/gitlab/issue-watch-dialog.test.tsx components/gitlab/review-watch-dialog.test.tsx
```

## Files likely touched

- `apps/web/lib/watcher-profile-default.ts`
- `apps/web/lib/watcher-profile-default.test.ts`
- `apps/web/lib/watcher-repository-default.ts`
- `apps/web/lib/watcher-repository-default.test.ts`
- `apps/web/components/watcher-repository-fields.tsx`
- focused shared-field test if needed
- `apps/web/components/github/issue-watch-dialog.tsx`
- `apps/web/components/github/review-watch-dialog.tsx`
- `apps/web/components/gitlab/watch-dialog.tsx`
- `apps/web/components/jira/jira-issue-watch-dialog.tsx`
- `apps/web/components/linear/linear-issue-watch-dialog.tsx`
- `apps/web/components/task/chat/messages/sender-task-badge.tsx`
- closest existing sender badge/chat-message test
- affected locale namespace files for newly referenced keys
- this task file and `plan.md`

## Dependencies

None.

## Parallelism

Sequential. The dialogs share helper and catalog contracts.

## Inputs

- Spec scenarios for watcher profile defaults, repository states, and unknown sender tasks.
- Existing sentinel normalization tests and existing watcher dialog component-test patterns.
- Frontend i18n rule forbidding module-scope `t()` and translation of domain values.

## Output contract

Report RED/GREEN evidence, exact key ownership, unchanged sentinel/payload behavior, files changed,
tests run, blockers and risks, then update this task and `plan.md` statuses.

## Results

RED: `cd apps/web && pnpm test -- --run lib/watcher-repository-default.test.ts components/task/chat/messages/chat-message.test.tsx` failed on the three English-returning placeholder states and the pseudo-locale unknown-task fallback. The remaining chat failures were cascading locale leakage after the intended assertion aborted.

GREEN: `cd apps/web && pnpm test -- --run lib/watcher-profile-default.test.ts lib/watcher-repository-default.test.ts components/task/chat/messages/chat-message.test.tsx components/github/review-watch-dialog.test.tsx components/gitlab/issue-watch-dialog.test.tsx components/gitlab/review-watch-dialog.test.tsx` passed 44 tests in 6 files.

PR review remediation: `cd apps/web && pnpm test -- --run components/watcher-repository-fields.test.tsx lib/watcher-profile-default.test.ts lib/watcher-repository-default.test.ts components/task/chat/messages/chat-message.test.tsx components/github/review-watch-dialog.test.tsx components/gitlab/issue-watch-dialog.test.tsx components/gitlab/review-watch-dialog.test.tsx` passed 46 tests in 7 files after proving that repository-first and loading placeholders remain visible while the branch selector is disabled.

CI/review follow-up: `cd apps/web && pnpm test -- --run components/watcher-repository-fields.test.tsx components/task/chat/messages/chat-message.test.tsx && pnpm run typecheck` passed 31 tests in 2 files and the full web TypeScript check. The sender fallback test now proves an already-mounted badge changes from exact English to exact pseudo copy after a live locale switch.

Sentinel values and empty-ID normalization are unchanged. Display copy resolves inside React render paths; no external side effects or security boundaries apply.
