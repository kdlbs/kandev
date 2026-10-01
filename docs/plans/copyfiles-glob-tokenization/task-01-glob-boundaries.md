---
id: "01-glob-boundaries"
title: "Repair glob boundaries"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-COPYFILES-GLOB-TOKENIZATION-001
acceptance_criteria:
  - AC-WORKSPACES-COPYFILES-GLOB-TOKENIZATION-001.1
  - AC-WORKSPACES-COPYFILES-GLOB-TOKENIZATION-001.2
  - AC-WORKSPACES-COPYFILES-GLOB-TOKENIZATION-001.3
  - AC-WORKSPACES-COPYFILES-GLOB-TOKENIZATION-001.4
system_design:
  - ../../specs/workspaces/system-design/copyfiles-glob-tokenization.md
---

# Task 01: Repair Glob Boundaries

## Summary

Make list tokenization honor character classes and native escape semantics.
Recreate permanent regressions and verify local/remote selection parity.

## In scope

- Tokenizer state, focused parsing and Copy/Plan tests, public grammar guidance.
- Preserve save validation and existing suffix/mode/precedence semantics.

## Out of scope

- Consumers, settings schema/UI, containment policy, and unrelated cleanup.

## Acceptance

- Permanent tests fail before the fix for class commas, class braces, and POSIX
  escaped braces; after the fix both public materializers select all expected
  paths and bytes without warnings.
- Grammar tests preserve escapes, negation, nesting, adjacent suffixes, and
  first-entry precedence; Windows separators do not escape syntax.
- Exact targeted checks pass and the plan records their results.

## Verification

```bash
(cd apps/backend && go test -race ./internal/worktree/copyfiles)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 .github/scripts/backend-tests-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
git diff --check
```

Run focused failing tests first with `go test ./internal/worktree/copyfiles
-run 'Test(ParseSpecs|CopyPlan)_GlobTokenization' -count=1` from `apps/backend`.
Native Windows coverage runs in the existing CI `test-windows` job:

```bash
(cd apps/backend && go test -race -v ./internal/worktree/copyfiles -run '^(TestParseSpecs_GlobTokenization|TestParseSpecs_NativeEscapes|TestCopyPlan_GlobTokenization|TestValidateSpec_GlobAdjacentSuffix)$')
```

When bare Node is absent, resolve the configured runtime through
`pnpm exec node -p process.execPath` from `apps/`, then invoke that executable
from repo root; validators resolve repository files against the working directory.

Run the repository PR documentation `validateCoverage` preflight against changed
files and the full referenced artifact closure before publication. If a PR
finding requires backend edits, run the scoped AGENTS.md lint command too.

## Files likely touched

- `apps/backend/internal/worktree/copyfiles/copyfiles.go`
- `apps/backend/internal/worktree/copyfiles/copyfiles_glob_tokenization_test.go`
- `docs/public/git-operations.md`
- `.github/workflows/backend-tests.yml` (focused Windows native test step).
- The paired specs and this unique plan package for status/results.

## Dependencies

None. A later explicit implementation turn follows the design handoff.

## Risks

Native backslash semantics and class closing delimiters; avoid rewriting bytes
that the matcher needs. Keep test fixtures portable and use POSIX-only skips
only for genuine platform-specific patterns or native links.

## Parallelism

`sequential`; no workers or extra persistent tasks/sessions.

## Inputs

- Paired requirement/design above and ADR 0010.
- `copyfiles.go`, `copyfiles_test.go`, `copyfiles_symlink_test.go`.
- `.agents/skills/tdd/references/backend-tests.md`.

## Results

- Permanent focused regressions failed before the production edit for the
  reported class comma, class brace, and escaped brace; additional delimiter,
  mode/precedence, and suffix validation assertions also failed as expected.
- Focused grammar/Copy/Plan tests pass after the correction.
- `go test -race ./internal/worktree/copyfiles`: passed.
- `python3 scripts/list-docs.py validate`: passed (339 decisions, 1283 specs).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- Public docs validator tests: passed (62 tests), using Node v24.18.0 from root.
- Live public docs validator: passed (47 pages), using Node v24.18.0 from root.
- Backend workflow contract: passed (10 tests); action pinning: passed (9 tests).
- Repository `validateCoverage` preflight: covered, no errors.
- `git diff --check`: passed.
- Native Windows grammar execution is delegated to the focused existing CI job;
  local verification ran on Linux. No live instance/data used.

The first public-doc test invocation from `apps/` failed on a root-relative
plugin-doc path. Running the same tests from repository root passed; no validator
or unrelated document was changed.

