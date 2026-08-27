---
id: "01-preserve-review-path-punctuation"
title: "Preserve Review path punctuation"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/review-file-status.md"
---

# Task 01: Preserve Review path punctuation

## Acceptance

- Desktop and phone sticky Review headers render dot-prefixed directory segments in their original character order; `.agents/skills/pr-fixup/SKILL.md` never appears as `agents/skills/pr-fixup./SKILL.md` or with the dot beside another segment.
- Constrained headers retain leading-edge directory truncation, the nearest directory suffix, the separate filename, the full title/accessible path, existing status/actions geometry, and zero document-level horizontal overflow.
- Focused component, desktop Chromium, mobile Chromium, typecheck, lint, and diff checks pass with no persistent diagnostic artifacts or processes left behind.

## Verification

Install dependencies only if this worktree does not already have them:

```bash
cd apps && pnpm install --frozen-lockfile
```

Run the new component regression before the source change and record its expected failure. Add the desktop rendered-order regression and run it against the production build before the source change; record the expected visual-order failure. After the minimal fix, run:

```bash
cd apps && pnpm --filter @kandev/web test -- components/review/review-diff-header.test.tsx
cd apps/web && pnpm e2e:run tests/review/review-file-status.spec.ts -- --workers=1 --retries=0
cd apps/web && pnpm e2e:run --project mobile-chrome tests/review/mobile-review-file-status.spec.ts -- --workers=1 --retries=0
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
git diff --check
```

The managed desktop and mobile E2E commands each rebuild the production backend and Vite assets. Do not treat DOM `textContent` as rendered-order evidence; the bug leaves DOM data correct.

## Files likely touched

- `apps/web/components/review/review-diff-header.tsx`
- `apps/web/components/review/review-diff-header.test.tsx`
- `apps/web/e2e/helpers/layout-assertions.ts`
- `apps/web/e2e/tests/review/review-file-status.spec.ts`
- `apps/web/e2e/tests/review/mobile-review-file-status.spec.ts`
- `docs/specs/ui/requirements/review-file-status.md`
- `docs/plans/review-path-punctuation/plan.md`
- `docs/plans/review-path-punctuation/task-01-preserve-review-path-punctuation.md`

## Dependencies

None.

## Parallelism

`sequential` because the component markup and both responsive regressions implement one shared bidi/truncation contract.

## Inputs

- `docs/specs/ui/requirements/review-file-status.md`, especially the sticky-header path and mobile overflow scenarios.
- `docs/plans/review-path-punctuation/plan.md`, including the confirmed root cause and mobile design contract.
- `apps/web/components/review/AGENTS.md` and `apps/web/AGENTS.md`.
- `/tdd`, `/e2e`, and `/mobile-parity` guidance during implementation.

## Output contract

Report RED and GREEN results with exact commands and counts, all changed files, failure/capture artifact paths, cleanup and teardown evidence, public-doc/i18n impact, residual risks, and synchronized task/plan statuses.

## Results

- Added one shared directory renderer that preserves the existing RTL truncation wrapper and isolates the literal path inside `<bdi dir="ltr">`. Desktop and phone keep their existing composition, filename, full title/accessible label, actions, status, and geometry.
- Added desktop/mobile component coverage and a shared Playwright range-geometry probe. Desktop now seeds `.agents/skills/pr-fixup/SKILL.md`; mobile uses a longer dot-prefixed path and proves the directory is genuinely constrained while document overflow remains absent.

### TDD evidence

- **Component RED:** `cd apps && pnpm --filter @kandev/web test -- components/review/review-diff-header.test.tsx` exited 1: 2 new cases failed and 6 prior cases passed. Desktop had no directory hook; mobile had no LTR isolate.
- **Browser setup RED:** the first full production command exited 1 because the planned desktop directory hook did not exist yet. Its failure screenshot was `apps/web/e2e/test-results/review-review-file-status--2fac6-th-and-explains-a-pure-move-chromium/test-failed-1.png`; the context was the sibling `error-context.md`.
- **Browser visual RED:** after changing only the test locator, `cd apps/web && pnpm e2e:run --no-build tests/review/review-file-status.spec.ts -- --workers=1 --retries=0` exited 1 with expected `.agents/skills/pr-fixup` and received `agents/skills/pr-fixup.`. This used the unchanged production bundle from the preceding full build.

### Final verification

- `cd apps && pnpm --filter @kandev/web test -- components/review/review-diff-header.test.tsx`: 1 file passed, 8 tests passed.
- `cd apps/web && pnpm e2e:run tests/review/review-file-status.spec.ts -- --workers=1 --retries=0`: Chromium, 1 passed. A test-only stable-selector refactor then passed once more with `--no-build`.
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/review/mobile-review-file-status.spec.ts -- --workers=1 --retries=0`: mobile-chrome, 1 passed.
- `cd apps/web && pnpm run typecheck`: passed.
- `cd apps && pnpm --filter @kandev/web lint`: passed. An earlier run found one `max-lines-per-function` warning in the enlarged test describe; splitting the new cases into their own describe resolved it.
- `git diff --check`: passed.

### Cleanup and impact

- Later managed GREEN runs cleared the RED screenshot/context and tore down their owned backends, browser workers, and current-run `/tmp/kandev-e2e-*` directories. Only the ignored successful-run marker `apps/web/e2e/test-results/.last-run.json` remains; older temp directories belong to earlier unrelated runs and were not touched.
- No public docs, locale catalogs, APIs, persistence, or user-facing copy changed. The internal behavioral spec was amended. Strong right-to-left Unicode path content remains explicitly out of scope.
