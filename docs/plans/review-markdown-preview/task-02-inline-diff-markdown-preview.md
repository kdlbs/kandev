---
id: "02-inline-diff-markdown-preview"
title: "Render changed Markdown inside Review"
status: done
wave: 2
depends_on: ["01-review-markdown-preview"]
plan: "plan.md"
spec: "../../specs/ui/requirements/review-markdown-preview.md"
---

# Task 02: Render changed Markdown inside Review

## Acceptance

1. Desktop, tablet, and mobile keep the Review dialog open while toggling a `.md` or `.mdx` row between
   its textual diff and a rendered changed-content preview; no file tab or Files viewer opens.
2. Complete added/untracked diffs render as one document, while modified hunks remain separate and
   partial/truncated content is visibly labelled.
3. Files without renderable new-side Markdown do not expose the action, and the existing sanitized
   renderer, review state, comments, filtering, and file ordering remain intact.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
pnpm --filter @kandev/web test -- --run \
  components/review/review-markdown-diff-preview.test.ts \
  components/review/review-diff-toolbar.test.tsx \
  components/review/review-diff-list-grouping.test.tsx
pnpm --dir web e2e:run tests/review/review-markdown-preview.spec.ts
pnpm --dir web e2e:run -- --project=mobile-chrome tests/review/mobile-review-markdown-preview.spec.ts
cd ..
make fmt
make typecheck test lint
```

Completed 2026-07-29:

- RED: parser test initially failed because `review-markdown-diff-preview` did not exist; desktop
  and mobile E2E expectations also failed under the prior dialog-closing navigation behavior.
- GREEN: focused Vitest (`18` assertions across parser, toolbar, and row-state coverage), desktop
  Playwright, and `mobile-chrome` Playwright all passed.
- Full verification passed: `make fmt`, then `make typecheck test lint`.

## Files likely touched

- `apps/web/components/review/review-markdown-diff-preview.tsx`
- `apps/web/components/review/review-markdown-diff-preview.test.ts`
- `apps/web/components/review/review-diff-toolbar.tsx`
- `apps/web/components/review/review-diff-toolbar.test.tsx`
- `apps/web/components/review/review-diff-header.tsx`
- `apps/web/components/review/review-diff-list.tsx`
- `apps/web/components/review/review-diff-list-grouping.test.tsx`
- `apps/web/components/review/review-dialog-surface.tsx`
- `apps/web/components/review/review-dialog.tsx`
- `apps/web/components/task/dockview-review-dialog.tsx`
- `apps/web/components/task/use-review-dialog.ts`
- `apps/web/components/task/mobile/session-mobile-review-dialog.tsx`
- `apps/web/components/task/mobile/session-tablet-layout.tsx`
- `apps/web/e2e/tests/review/review-markdown-preview.spec.ts`
- `apps/web/e2e/tests/review/mobile-review-markdown-preview.spec.ts`

## Dependencies

Task 01 is the shipped baseline whose external navigation behavior this task replaces. Reuse its
toolbar entry points and the existing sanitized Markdown renderer.

## Parallelism

Sequential. Parser output, toolbar eligibility, row state, responsive behavior, and E2E assertions
share one component contract.

## Inputs

- `docs/specs/ui/requirements/review-markdown-preview.md`
- `docs/plans/review-markdown-preview/plan.md`
- `apps/web/AGENTS.md`
- `.agents/skills/tdd/SKILL.md`
- `.agents/skills/e2e/SKILL.md`
- `.agents/skills/mobile-parity/SKILL.md`

## Risks

- Never concatenate separate modified hunks into a fake continuous document.
- Do not render deleted lines or unified-diff metadata as Markdown.
- Do not introduce a second scroll owner inside the mobile Review dialog.
- Keep raw HTML sanitization identical to the existing Markdown preview surface.

## Output contract

Report files changed, RED/GREEN test evidence, exact verification results, blockers or residual
risks, and update this task plus `plan.md` to done.
