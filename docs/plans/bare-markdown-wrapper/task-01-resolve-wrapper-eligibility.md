---
id: "01-resolve-wrapper-eligibility"
title: "Resolve bare-wrapper eligibility"
status: blocked
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-COMMENT-MARKDOWN-001
acceptance_criteria:
  - AC-UI-COMMENT-MARKDOWN-001.1
  - AC-UI-COMMENT-MARKDOWN-001.2
system_design:
  - ../../specs/ui/system-design/comment-markdown.md
---

# Task 01: Resolve bare-wrapper eligibility

## Summary

Resolve whether a bare wrapper can be repaired safely. The current analysis shows
that raw Markdown does not carry enough intent information, so the work order
records the preservation fallback and remains blocked.

## In scope

- Use the [analysis](analysis.md) to preserve the ambiguous cases.
- Add the exact rule-bounded, enclosing-fence, and multi-region regressions.
- Keep the existing normalizer behavior until a safe contract is accepted.

## Out of scope

- Adding a heuristic that narrows the separate-block preservation guarantee.
- Defining a new requirement or system-design contract without a product decision.
- Changing cache semantics, Markdown plugins, or renderer components.
- New permanent test files or dependency changes.

## Acceptance

1. The analysis records why the reported input is ambiguous and names the missing
   intent signal.
2. The normalizer leaves the exact negative cases byte-identical, including valid
   code content enclosed by a five-backtick fence and two disjoint regions.
3. The plan and work order remain blocked until a product-level contract makes a
   safe repair possible.

## ASCII UI preview

UI-01 uses the [shared desktop/phone preview](plan.md#ascii-ui-preview).

```text
Requested: intro -> one raw document code block -> outro
Current safe behavior: first code block -> prose -> second code block
```

Content grouping supports `AC-UI-COMMENT-MARKDOWN-001.1` and `.2`.
No layout or mobile interaction changes are proposed.

## Verification

Run documentation commands from the repository root after revising the package:

```bash
rtk proxy python3 scripts/lint-spec-files.test.py
rtk proxy python3 scripts/lint-spec-files.py --all
rtk git diff --check -- docs/specs docs/plans/bare-markdown-wrapper
rtk git status --short -- docs/plans/bare-markdown-wrapper
```

Use TDD in the existing test file. First demonstrate each unsafe repair candidate
fails its preservation regression. Then run this complete block from the repository
root:

```bash
(cd apps && rtk pnpm install --frozen-lockfile)
(cd apps && rtk pnpm --filter @kandev/web test -- normalize-cache)
(cd apps && rtk pnpm --filter @kandev/web lint)
rtk git diff --check
```

Installation is required once in a fresh worktree. Do not override the configured
Vitest worker budget or its `NODE_ENV=test` setting. Changed-line warnings fail
acceptance. Preserve the supplied frontend complexity and size limits.

## Files in this package

- `docs/plans/bare-markdown-wrapper/analysis.md`
- `docs/plans/bare-markdown-wrapper/plan.md`
- `docs/plans/bare-markdown-wrapper/task-01-resolve-wrapper-eligibility.md`
- `apps/web/lib/markdown/normalize-cache.test.ts`

## Dependencies

An accepted producer marker, parser-level intent signal, or product decision is
required. No dependency work order exists.

## Risks

The existing inner scan cannot establish author intent even inside the proposed
boundary. Avoid changing existing tagged-wrapper behavior, and preserve separate
bare blocks plus literal content inside enclosing fences.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/comment-markdown.md)
- [System design](../../specs/ui/system-design/comment-markdown.md)
- `apps/web/lib/markdown/normalize-cache.ts`
- [Counterexamples and source evidence](analysis.md)
- Existing normalizer helpers and tests, especially wrapper unchanged boundaries.
- The user's explicit implementation request and the written-analysis fallback.

## Results

The rule-delimited repair was rejected after the exact preservation counterexample
and enclosing-fence case failed. The production normalizer remains unchanged for
bare candidates. Preservation regressions and final validation results are recorded
in the plan; the work order is blocked pending a safe intent signal.
