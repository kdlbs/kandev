---
created: 2026-10-01
status: implemented
requirements:
  - REQ-WORKSPACES-COPYFILES-GLOB-TOKENIZATION-001
system_design:
  - ../../specs/workspaces/system-design/copyfiles-glob-tokenization.md
legacy_specs: []
---

# Implementation Plan: Copy-file Glob Tokenization

## Overview

Restore supported patterns with one sequential tokenizer/test work order.
Main verified at `08e4ffdb99caf40b0df5baa67b29cf4313188f15` on 2026-10-01.
The root cause is brace-only delimiter scanning: class commas split entries,
and class/escaped braces incorrectly change alternation depth.

A removed temporary `TestTokenizerTemporaryRepro` reproduced all three reported
settings through save validation, ParseSpecs/Copy, and Parse/Plan: validation
accepted every setting, while both materializers selected zero files instead
of two. Permanent coverage must reproduce these failures before correction.

## Scope

Own the copyfiles tokenizer and focused grammar/materialization tests. Preserve
suffix grammar, precedence, escape bytes, nesting, and native Windows separators.
No UI, schema, matcher, containment, or unrelated refactor is included.
The existing Windows CI job gains a focused native grammar test step because
copyfiles was absent from its package allowlist.

## Technical approach

Keep splitting in `copyfiles.go`, adding character-class and platform-aware
escape state. Keep mode extraction and both consumers unchanged. Public docs
get a short copy-file grammar section in the existing Git operations guide
through `/docs-maintainer`; no new page or diagram is needed.

## Tests

`copyfiles_glob_tokenization_test.go` will contain
`TestParseSpecs_GlobTokenization` (.1, .2, .4),
`TestCopyPlan_GlobTokenization` (.1 through .3), and focused mode/precedence
coverage (.4). Test escaped commas/braces/brackets, escaped closing brackets,
negated classes, class braces, nested alternation, adjacent symlink entries,
and Windows delimiter/separator behavior. Keep existing suite coverage intact.

## End-to-end evidence

Public package pipelines use disposable filesystem fixtures to compare
ParseSpecs/Copy and Parse/Plan paths and bytes. This backend grammar fix requires
no browser test or screenshot.

## Work orders

- [x] [Task 01: Repair glob boundaries](task-01-glob-boundaries.md)

## Verification results

Design checks passed: catalog validation (339 decisions, 1283 specifications),
36 specification linter tests, full specification lint, and diff whitespace check.
The parent reviewed all four package files and requested implementation in a later
turn.

Implementation passed focused RED/GREEN regressions, package race tests, catalog
validation, all 36 spec linter tests, full spec lint, all 62 public validator
tests, the live 47-page public validator, 10 backend workflow contract tests,
9 action pinning tests, documentation coverage preflight, and diff whitespace
checks. See the work order for exact results and the corrected validator
working-directory invocation. Native Windows execution awaits CI.

## Risks

- POSIX backslash escapes must not become Windows escapes.
- Class closing brackets and escaped brackets must not leak delimiter state.
- Main may advance while other approved fixes land; refresh before delivery.

## PR review remediation

PR #4139 exposed an unclosed-class recovery regression and a dependency shortcut
that misses exact escaped-comma filenames. Permanent regressions failed before
the correction. The same work order now includes class-closer recovery and a
contained exact-literal escape fallback after native literal-path priority.
Final remediation checks passed: package race tests, full changed-revision backend
lint (zero issues on warmed-cache retry), all named specification/public-doc and
workflow validators, documentation coverage preflight, and diff whitespace checks.
The work order records the initial cold-cache timeout and exact final results.
