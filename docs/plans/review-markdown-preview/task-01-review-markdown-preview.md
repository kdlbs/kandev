---
id: "01-review-markdown-preview"
title: "Expose rendered Markdown from Review diffs"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/review-markdown-preview.md"
---

# Task 01: Expose rendered Markdown from Review diffs

## Acceptance

1. Desktop and mobile Review headers expose `Preview markdown` only for `.md`/`.mdx` files and
   open the selected file directly in rendered mode.
2. Preview requests preserve the review row's repository name so same-path files in different
   repositories cannot be confused.
3. Desktop and mobile Playwright scenarios prove the user-visible flow, with the mobile action
   remaining reachable through the existing 44 px menu.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
pnpm --filter @kandev/web test -- --run \
  components/review/review-diff-toolbar.test.tsx \
  components/task/mobile/session-mobile-layout.test.tsx \
  components/task/mobile/mobile-file-viewer-panel.test.tsx
pnpm --dir web e2e:run tests/review/review-markdown-preview.spec.ts
pnpm --dir web e2e:run tests/review/mobile-review-markdown-preview.spec.ts -- --project=mobile-chrome
cd ..
make fmt
make typecheck test lint
```

## Files likely touched

- `apps/web/components/review/review-diff-toolbar.tsx`
- `apps/web/components/review/review-diff-toolbar.test.tsx`
- `apps/web/components/review/review-diff-header.tsx`
- `apps/web/components/review/review-diff-list.tsx`
- `apps/web/components/review/review-dialog-surface.tsx`
- `apps/web/components/review/review-dialog.tsx`
- `apps/web/components/task/use-review-dialog.ts`
- `apps/web/components/task/dockview-review-dialog.tsx`
- `apps/web/components/task/mobile/session-mobile-layout.tsx`
- `apps/web/components/task/mobile/session-mobile-review-dialog.tsx`
- `apps/web/components/task/mobile/session-mobile-layout.test.tsx`
- `apps/web/components/task/mobile/mobile-file-viewer-panel.tsx`
- `apps/web/components/task/mobile/mobile-file-viewer-panel.test.tsx`
- `apps/web/e2e/tests/review/review-markdown-preview.spec.ts`
- `apps/web/e2e/tests/review/mobile-review-markdown-preview.spec.ts`

## Dependencies

None. Reuse the existing file editor, Markdown renderer, Review toolbar, responsive menu, and
mobile file viewer.

## Parallelism

Sequential. The responsive paths share callback types, Review wiring, and E2E setup.

## Inputs

- `docs/specs/ui/requirements/review-markdown-preview.md`
- `docs/plans/review-markdown-preview/plan.md`
- `apps/web/AGENTS.md`
- `.agents/skills/tdd/SKILL.md`
- `.agents/skills/e2e/SKILL.md`
- `.agents/skills/mobile-parity/SKILL.md`

## Risks

- Preserve repository scoping through every callback.
- Keep preview intent coupled to the non-stale mobile file request.
- Do not change Markdown rendering or raw-HTML safety behavior.

## Completion evidence

- Changed files: Review toolbar/dialog wiring, desktop and mobile file viewers, tablet preview
  routing, unit coverage, desktop/mobile Playwright coverage, and this plan package.
- RED: focused toolbar and mobile preview tests initially failed because preview requests did not
  receive repository context and mobile/tablet preview state was not available.
- GREEN: focused Vitest coverage passed after the implementation; desktop and mobile Playwright
  review-preview scenarios passed.
- Verification: `make fmt` and `make typecheck test lint` passed before the implementation commit;
  subsequent PR fixes reran focused Vitest, TypeScript typecheck, and changed-file ESLint checks.
- Residual risk: preview-environment deployment depends on external credentials; an earlier sprite
  authentication failure was unrelated to the code and later CI runs were retried.

## Output contract

Report files changed, RED/GREEN test evidence, exact verification results, blockers or residual
risks, and update this task plus `plan.md` to done.
