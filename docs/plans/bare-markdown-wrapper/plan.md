---
created: 2026-09-11
status: blocked
requirements:
  - REQ-UI-COMMENT-MARKDOWN-001
system_design:
  - ../../specs/ui/system-design/comment-markdown.md
legacy_specs: []
---

# Implementation Plan: Bare Markdown Wrappers

## Overview

This package records the requested repair, the counterexamples that make raw
Markdown wrapper intent ambiguous, and the resulting blocked implementation.

The existing normalizer remains unchanged for bare wrapper candidates. A future
implementation needs a producer marker, parser-level intent signal, or another
accepted product contract before it can safely repair the reported document.

The [analysis](analysis.md) contains concrete counterexamples, renderer evidence,
and the written-analysis fallback.

## Requirement and design reconciliation

UI owns reusable Markdown presentation independently of task or agent state.
The existing [requirements](../../specs/ui/requirements/comment-markdown.md)
define GFM rendering under `AC-UI-COMMENT-MARKDOWN-001.1` and `.2`.
The existing [design](../../specs/ui/system-design/comment-markdown.md) names
`MemoizedMarkdown` as the shared renderer.

The existing requirements and design define tagged whole-message wrapper repair,
but they do not define a safe raw-Markdown signal for a bare wrapper. The proposed
thematic-break contract was rejected because it merges valid independent blocks
and can rewrite literal content inside an enclosing code block. No new acceptance
criterion or system-design behavior is recorded until that product decision is
resolved.

## Scope

### In scope

- Record the ambiguity and renderer evidence in the analysis.
- Add regression coverage for the exact preservation cases.
- Keep the existing normalizer behavior until a safe contract is accepted.

### Out of scope

- Any bare-wrapper repair without an explicit intent signal.
- Automatic changes to message producers, renderer APIs, or stored messages.
- New parser dependencies, cache keys, cache limits, or telemetry.
- Changes to tagged-wrapper eligibility or existing trailing-prose behavior.
- Commits, pushes, PRs, and delegated implementation.

## Technical approach

`normalizeMarkdown` calls `strengthenMarkdownWrapperFence` before its glued-close
repair. Tagged wrappers keep their existing whole-message eligibility. Bare
wrapper candidates remain unchanged because the raw input does not identify
whether matching fences are a wrapper or separate Markdown blocks.

The supplied reproduction has a bare opener and surrounding prose. The normalizer
must preserve its current Markdown interpretation until a product-level signal
resolves the ambiguity.

Keep any future repair inside `apps/web/lib/markdown/normalize-cache.ts`. Reuse the
existing tagged-wrapper path and do not weaken separate-block preservation. If a
future contract is accepted, it must preserve indentation, surrounding prose,
interior text, and newline behavior.

`normalizeCached` remains a raw-input-string LRU with a hard cap of 500 entries.
No task, session, message, or viewport information enters its key.

## ASCII UI preview

UI-01: Existing chat message, desktop and phone. The requested grouping remains
blocked because the raw input has two valid interpretations.
The count and content of code blocks are structural requirements, not pixel sizes.

```text
Current interpretation             Requested but unresolved
Intro and rule                    Intro and rule
[code: first portion]             [one code block:
Middle prose                        heading + literal fences
[code: second Go example]            both Go examples + prose]
Final prose                       Rule and outro
[code: rule and outro]
```

The same ambiguity applies on desktop and phone. This utility introduces no
controls, navigation, touch behavior, scroll ownership, or viewport branches. The
mobile-parity normalization exception permits targeted renderer coverage in the
existing unit test file. No new Playwright file is planned.

## Tests

All permanent cases belong in `apps/web/lib/markdown/normalize-cache.test.ts`.
The tests preserve the ambiguous inputs and prove that valid code content remains
literal.

| Requested behavior | Evidence |
| --- | --- |
| Bare wrapper candidate with two tagged Go blocks and surrounding prose | Byte-identical normalization and rendered Markdown boundaries remain unchanged |
| Two independent tagged Go blocks | Byte-identical normalization and exactly two rendered code blocks with prose between |
| Existing tagged Markdown wrappers | Existing suite passes, including sample boundaries and trailing-prose exclusions |
| Glued closing fences | Standalone `func f() {}` plus glued fence, nested glued closes, and glued wrapper closes retain existing results |
| Idempotence | Normalize the two-region bare fixture twice and compare with the unchanged first result |
| Raw-key bounded LRU | Existing hit, miss, overflow, and recency tests pass; assert cap equals 500 and distinct raw inputs remain distinct cache entries |
| Ambiguous syntax samples | Rule-bounded independent blocks, five-backtick literal content, and variants remain byte-identical |

Also cover CRLF, leading/trailing blank lines, indentation from zero to three
spaces, longer runs, missing closes, and bare blocks without nested tags.
Reuse fixture constants to respect duplicate-string limits.

### Rendered verification

Use React `createElement`, `react-dom/server`, and `react-markdown` with
`remark-gfm`, `remark-breaks`, and `remark-gemoji` inside the existing `.test.ts`.
Assert code text and surrounding elements, not only fence counts.
This is a parser/render integration check, not a claim of full browser E2E coverage.
It exercises the viewport-independent transformation used by desktop and phone.

## Work orders

- [ ] [Task 01: Resolve bare-wrapper eligibility](task-01-resolve-wrapper-eligibility.md) (`blocked`)

There is one sequential work order.

## Verification results

Read-only renderer probes on 2026-09-11 found:

- Compact reproduction: normalization unchanged, three code blocks.
- Counterexample A: normalization unchanged, two separate code blocks.
- Two independent tagged Go blocks: normalization unchanged, two code blocks.
- Two nested-looking pairs: normalization unchanged, three code blocks.

These probes used installed React and react-markdown packages. The latter two
also used all three production remark plugins. Permanent regression tests cover
the preserved interpretations in the existing normalizer test file.
Documentation validation on 2026-09-11:

- `rtk proxy python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `rtk proxy python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `rtk git diff --check -- docs/plans/bare-markdown-wrapper`: passed.
- `rtk git status --short -- docs/plans/bare-markdown-wrapper`: new package present, untracked.

Verification after restoring the safe boundary on 2026-09-12:

- `pnpm --filter @kandev/web test -- --run lib/markdown/normalize-cache.test.ts`: 52 tests passed.
- `pnpm --filter @kandev/web lint`: passed with zero warnings.
- `pnpm --filter @kandev/web typecheck`: passed.
- `pnpm --filter @kandev/web build:vite`: passed. Vite emitted existing chunk-size and ineffective dynamic-import warnings.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check`: passed.

The full `pnpm --filter @kandev/web test` suite was started with the configured
worker budget but produced no terminal report after 13 minutes, so it was
stopped. The focused suite completed independently with a captured exit code of
zero.

The package remains uncommitted and blocked pending a product-level intent signal.

## Risks and unresolved decision

The thematic-break contract cannot be accepted as a compatibility boundary. A
message with the same bytes can represent one wrapper, two independent blocks, or
literal content inside a larger fence. Correct longer outer fences or an explicit
producer marker remain possible future solutions, but producer changes remain
outside this task.
