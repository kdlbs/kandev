---
id: "02-prove-resize-interaction"
title: "Prove Markdown table resizing and mobile parity"
status: done
wave: 2
depends_on: ["01-build-resize-renderer"]
plan: "plan.md"
spec: "../../specs/ui/requirements/resizable-markdown-tables.md"
---

# Task 02: Prove Resize Interaction and Mobile Parity

## Acceptance

- A desktop browser test drags a separator from its body-row hit area and proves
  equal-and-opposite adjacent width changes, an unchanged third column, a stable
  table width, and 64-pixel clamping.
- Desktop tests prove double-click reset, arrow-key adjustment, and `Enter` reset.
- A locally scrolling wide table keeps separators aligned after scrolling and
  creates no chat- or document-level horizontal overflow.
- Mobile Chrome exposes no separator controls and preserves ordinary table
  wrapping plus wide-table local scrolling.
- Existing Markdown wrapping regressions remain green.

## TDD sequence

1. RED: extend the desktop wrapping fixture with a three-column interaction case
   and prove current tables have no separator.
2. GREEN: drive pointer, double-click, and keyboard behavior against the Task 01
   implementation; refine only externally observable defects.
3. RED/GREEN: add mobile absence and parity assertions to the existing phone
   fixture.
4. Run both complete focused files to catch wrapping or scroll-owner regressions.

## Verification

```bash
make build-web
(cd apps/web && pnpm e2e:run tests/chat/markdown-wrap.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-markdown-wrap.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web lint)
git diff --check
```

## Files likely touched

- `apps/web/e2e/tests/chat/markdown-wrap.spec.ts`
- `apps/web/e2e/tests/chat/mobile-markdown-wrap.spec.ts`
- Task 01 production files only if browser evidence reveals an interaction bug

## Dependencies

- `01-build-resize-renderer`

## Parallelism

`sequential`. Browser assertions depend on the completed renderer contract and
may legitimately feed a defect back into Task 01 files.

## Inputs

- Existing exact pnpm-table and wide-table fixtures in both wrapping suites
- `docs/specs/ui/requirements/resizable-markdown-tables.md`, especially desktop interaction,
  local-scroll ownership, and mobile capability scenarios

## Output contract

Report desktop and mobile command results, before/after column and table widths,
the tested hit-point location, local/document overflow measurements, files
changed, and any unsupported browser behavior.

## Result

- RED: the desktop user-flow test failed because the shared renderer exposed no
  resize separator.
- GREEN: the rebuilt production bundle passed all 8 desktop Markdown wrapping
  tests and both Mobile Chrome Markdown tests.
- The desktop test drags from the first body row, verifies equal-and-opposite
  60-pixel adjacent changes, an unchanged third column and table width, the
  64-pixel clamp, double-click reset, 8-pixel keyboard adjustment, and `Enter`
  reset.
- A wide desktop table keeps its separator aligned after 160 pixels of local
  scroll. Chat and document overflow checks remain clean.
- Mobile Chrome exposes no separators while ordinary tables wrap and wide tables
  retain local scrolling. No unsupported browser behavior remains in scope.
