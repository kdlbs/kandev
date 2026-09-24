---
id: "01-render-math"
title: "Render math across shared Markdown"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MARKDOWN-MATH-001
  - REQ-UI-MARKDOWN-MATH-002
acceptance_criteria:
  - AC-UI-MARKDOWN-MATH-001.1
  - AC-UI-MARKDOWN-MATH-001.2
  - AC-UI-MARKDOWN-MATH-001.3
  - AC-UI-MARKDOWN-MATH-001.4
  - AC-UI-MARKDOWN-MATH-001.5
  - AC-UI-MARKDOWN-MATH-001.6
  - AC-UI-MARKDOWN-MATH-002.1
  - AC-UI-MARKDOWN-MATH-002.2
  - AC-UI-MARKDOWN-MATH-002.3
  - AC-UI-MARKDOWN-MATH-002.4
  - AC-UI-MARKDOWN-MATH-002.5
system_design:
  - ../../specs/ui/system-design/markdown-math.md
---

# Task 01: Render Math Across Shared Markdown

## Summary

Integrate KaTeX throughout the shared Markdown pipeline.
Preserve ordinary text, sanitizer safety, source positions, and phone readability.

## In scope

- Install the requested packages through pnpm and update the workspace lockfile.
- Add stylesheet assets, shared plugins, and narrowly scoped syntax compatibility where required.
- Wire every shared-plugin consumer listed in the system design.
- Preserve preview sanitization and comment task-link behavior.
- Add meaningful unit, component, desktop, and phone coverage through TDD.
- Add the public syntax reference and record validation results.

## Out of scope

- Editor extensions, specialized Markdown pipelines, backend changes, and new settings.
- Unrelated renderer refactoring or broader sanitizer changes.

## Acceptance

- All mapped formula and compatibility criteria pass with the real rendering pipelines.
- Desktop and phone render readable formulas without page overflow or executable untrusted content.
- Required checks pass, documentation matches the implementation, and durable statuses reflect the results.

## ASCII UI preview

### UI-01: Formula reading, desktop and phone

```text
Energy: E = mc^2

         a
        ---
         b

Cost: $100 and $200
```

See the [full preview](plan.md#ascii-ui-preview).
Inline math stays in prose. Display math occupies its own region.
Phone overflow stays local to that region. The existing view owns vertical scrolling.
Spacing is illustrative. Applies to AC-UI-MARKDOWN-MATH-001.1 through 001.6 and 002.1.

## Verification

Run from the repository root unless the command changes directories.
If dependencies are absent, install them before running pnpm checks.

```bash
cd apps && pnpm install --frozen-lockfile
```

Install feature dependencies during the implementation phase:

```bash
cd apps/web && pnpm add remark-math rehype-katex katex
cd apps/web && pnpm add -D @types/katex
```

Run each command from a fresh repository-root shell:

```bash
cd apps/web && pnpm vitest run components/shared/markdown-components.test.tsx
cd apps/web && pnpm vitest run components/shared/markdown-math-renderers.test.tsx components/shared/memoized-markdown.test.tsx components/shared/chat-markdown-motion.test.tsx components/task/markdown-preview-content.test.ts components/task/markdown-preview-content.external-link.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run lint
cd apps/web && pnpm e2e:run --project chromium e2e/tests/chat/markdown-math.spec.ts e2e/tests/chat/markdown-preview.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/chat/mobile-markdown-math.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

If compatibility code is required, also run its focused tests:

```bash
cd apps/web && pnpm vitest run lib/markdown/remark-math-compat.test.ts
```

Record the failing test before implementation, then the passing result.
The E2E runner rebuilds production assets. Run suites sequentially with the configured worker budget.
Inspect desktop and phone screenshots in both themes. Record any unavailable runtime dependency as a blocker.
Do not claim browser validation from component snapshots alone.

## Files likely touched

- `apps/web/package.json`, `apps/pnpm-lock.yaml`, `apps/web/app/globals.css`.
- `apps/web/components/shared/markdown-components.tsx` and its test file.
- The eleven consumers listed in the system design, including the four primary renderers.
- Proposed `apps/web/lib/markdown/remark-math-compat.ts` and its test file, if parser gaps require them.
- Proposed `apps/web/lib/markdown/math-sanitize-schema.ts`, if needed to share exact marker allowances.
- `apps/web/lib/markdown/chat-text-motion-plugin.ts`, only if motion integration tests require a subtree exclusion.
- `apps/web/components/shared/{markdown-math-renderers,memoized-markdown,chat-markdown-motion}.test.tsx`.
- `apps/web/components/task/markdown-preview-content.test.ts` and `.external-link.test.tsx`.
- `apps/web/e2e/tests/chat/{markdown-math,mobile-markdown-math}.spec.ts`.
- `docs/public/sessions-and-review.md` and this design package's statuses/results.

## Dependencies

None. Implementation is authorized by the current user request.

## Risks

See the [plan risks](plan.md#risks).
Preserve existing schema attributes when extending the class allowlist.
Do not weaken the currency assertion to accept KaTeX output.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/markdown-math.md).
- [System design](../../specs/ui/system-design/markdown-math.md).
- `apps/web/AGENTS.md` and `apps/web/components/task/chat/AGENTS.md`.
- Existing `markdown-components.test.tsx`, `mobile-markdown-wrap.spec.ts`, and `markdown-preview.spec.ts` patterns.
- Repository `/tdd`, `/e2e`, `/mobile-parity`, and `/docs-maintainer` skills.

## Results

Implemented shared KaTeX Markdown rendering across the shared plugin consumers. Added parsed-node compatibility for currency literals and single-line display formulas, preserved sanitizer ordering and chat motion boundaries, bundled KaTeX fonts through the Vite entry, added local phone overflow styling, and documented the supported syntax.

TDD RED was recorded before plugin wiring. The final focused suite passed 10 files and 63 tests, including preview source/comment mapping, raw delimiter whitespace, and sanitizer marker regressions. Typecheck, lint, production build, Chromium coverage (11 tests across light and dark themes), mobile Chromium coverage (2 tests for chat and the native file preview viewer), specification validation, public docs validation, and whitespace checks all passed on 2026-09-22.
