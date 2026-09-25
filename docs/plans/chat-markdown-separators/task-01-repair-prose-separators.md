---
id: "01-repair-prose-separators"
title: "Repair and verify prose separators"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-COMMENT-MARKDOWN-002
acceptance_criteria:
  - AC-UI-COMMENT-MARKDOWN-002.1
  - AC-UI-COMMENT-MARKDOWN-002.2
  - AC-UI-COMMENT-MARKDOWN-002.3
  - AC-UI-COMMENT-MARKDOWN-002.4
  - AC-UI-COMMENT-MARKDOWN-002.5
  - AC-UI-COMMENT-MARKDOWN-002.6
system_design:
  - ../../specs/ui/system-design/comment-markdown.md
---

# Task 01: Repair and Verify Prose Separators

## Summary

Make eligible prose followed by a glued hyphen rule render as a paragraph and rule.
Prove short-title preservation, protected contexts, byte preservation, and chat behavior.
Implementation authorized by the explicit execution request for this task.

## In scope

- The prose normalization section in the paired design and all six criteria.
- The complete [test matrix and supplied fixture](plan.md#tests).
- Desktop and phone integration, cache regression gates, and result recording.

## Out of scope

- CSS, persistence, new plugins, raw HTML support, and bare-wrapper eligibility.
- Unrelated markdown/file-link behavior and broad frontend refactors.

## Acceptance

1. The supplied excerpt renders one heading and one rule, with body text outside
   headings. Short setext titles and protected content retain their interpretation.
2. Exact-byte, idempotence, cache, fence, threshold, and existing compatibility tests
   pass without weakening assertions or changing existing rule-delimited fixtures.
3. Desktop and phone checks pass on fresh builds, including reload and raw-message
   preservation. Every listed verification command has a recorded result.

## ASCII UI preview

UI-01, shared desktop/phone agent response, from the [full preview](plan.md#ascii-ui-preview):

```text
Before: Skills to change [h2] -> body swallowed by h2
After:  Skills to change [h2] -> body [p] -> separator [hr]
Title control: Skills to change [h2] remains a heading
```

The phone wraps paragraphs in the existing chat surface. The content order and
semantic elements are required by `.1`, `.2`, and `.6`. Pixel spacing is illustrative.

## TDD sequence

1. Read the linked requirements, design, plan, and `apps/web/AGENTS.md`.
2. Mark this work order `in_progress` after implementation authorization.
3. Add the smallest regression in `normalize-separators.test.ts` and run it.
   Record the expected paragraph/rule assertion failure before production edits.
4. Add desktop and phone regression scenarios. Run them against current production
   builds and record semantic failures, not setup or selector failures.
5. Extend the existing line scan minimally. Add each boundary or protection case
   through Red-Green-Refactor. Retain existing wrapper and glued-close expectations.
6. Run the complete targeted verification block after the final code edit.
7. Record results here and in the plan. Mark the work order done only when all
   checks pass. Reconcile the design extension with the implementation.

Use `@covers AC-UI-COMMENT-MARKDOWN-002.N` annotations or test names.
Do not create React component tests solely to assert HTML. Use pure-transform
tests, production-plugin parser integration, and the real chat browser scenarios.

## Verification

Run from the repository root. Install dependencies once if this is a fresh worktree.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/markdown/normalize-separators.test.ts lib/markdown/normalize-cache.test.ts components/shared/markdown-components.test.tsx)
(cd apps/web && pnpm exec eslint lib/markdown/normalize-cache.ts lib/markdown/normalize-separators.test.ts e2e/tests/chat/markdown-paragraph-breaks.spec.ts e2e/tests/chat/mobile-markdown-separators.spec.ts e2e/tests/chat/markdown-separators-helpers.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/markdown-paragraph-breaks.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-markdown-separators.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Managed E2E commands build current assets. Do not override worker budgets or run
the browser projects concurrently. Record discovered test counts and actual results.
Use existing causal waits and scoped locators. Do not add sleeps or broad DOM scans.

## Files likely touched

- `apps/web/lib/markdown/normalize-cache.ts`
- `apps/web/lib/markdown/normalize-separators.test.ts` (new)
- `apps/web/e2e/tests/chat/markdown-paragraph-breaks.spec.ts`
- `apps/web/e2e/tests/chat/mobile-markdown-separators.spec.ts` (new)
- `apps/web/e2e/tests/chat/markdown-separators-helpers.ts` (new)
- The paired requirements/design and this delivery package for status/results.

The existing `normalize-cache.test.ts` and `markdown-components.test.tsx` are
verification inputs. Preserve their existing fixtures and expectations.

## Dependencies

None. The blocked bare-wrapper package is not an implementation dependency.

## Risks

See [compatibility risks](plan.md#risks). A parser plugin or broader Markdown
contract requires a package revision before implementation continues.

## Parallelism

`sequential`

## Inputs

- [Requirement 002](../../specs/ui/requirements/comment-markdown.md#req-ui-comment-markdown-002-prose-section-separators)
- [Design extension](../../specs/ui/system-design/comment-markdown.md#prose-separator-normalization)
- [Plan, source fixture, and evidence](plan.md)
- `apps/web/lib/markdown/normalize-cache.ts` and its existing tests.
- `apps/web/components/shared/memoized-markdown.tsx` and `markdown-components.tsx`.
- Existing paragraph-break and mobile-markdown-wrap E2E patterns.
- `/tdd`, `/e2e`, and `/mobile-parity` skills.

## Results

Implemented the render-time prose separator repair in
`apps/web/lib/markdown/normalize-separators.ts` and composed it into the existing
normalization cache. The transform preserves short setext headings, protected
fences and block contexts, front-matter-like boundaries, line endings, and raw
stored messages. Desktop and phone tests cover reload behavior and the phone
viewport checks document overflow.

Verification from the repository root:

- Dependencies were already installed in the worktree, so the conditional install step was not needed.
- Focused Vitest compatibility suite: passed, 3 files and 96 tests.
- Targeted ESLint: passed with no warnings.
- TypeScript typecheck: passed.
- Vite production build: passed with existing chunk-size and ineffective dynamic-import warnings.
- Desktop Chromium E2E: passed, 2 tests.
- Mobile Chromium E2E: passed, 1 test.
- Follow-up review regressions: passed for mixed fence markers, container-boundary recovery, HTML termination classes, and list-owned tilde fences.
- Follow-up review fixup also covers arbitrary/basic HTML tags, exact front-matter opening, loose-list recovery after a blank, and lone-CR separator repair.
- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
