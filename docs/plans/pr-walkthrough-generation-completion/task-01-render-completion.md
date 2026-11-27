---
id: "01-render-completion"
title: "Report rendering errors and verify completion"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-PR-WALKTHROUGH-001
acceptance_criteria:
  - AC-UI-PR-WALKTHROUGH-001.4
  - AC-UI-PR-WALKTHROUGH-001.7
  - AC-UI-PR-WALKTHROUGH-001.12
  - AC-UI-PR-WALKTHROUGH-001.13
system_design:
  - ../../specs/ui/system-design/pr-walkthrough.md
---

# Task 01: Report rendering errors and verify completion

## Summary

Expose the parser error location and record each successful managed render. Add a fixed verifier that accepts only the trusted, complete output pair.

## In scope

- Preserve `JSONDecodeError.msg`, `lineno`, and `colno` in the renderer error.
- Include the fixed draft path without copying source or environment data into the error.
- Clear the previous receipt before each render invocation.
- Validate `HEAD_SHA` as the exact lowercase 40-character managed event SHA.
- Write the receipt atomically after committing both existing final outputs.
- Add the no-argument, read-only `pr-walkthrough-verify` entry point.
- Reuse metadata binding, schema validation, and `build.build()` without duplicating the schema.
- Teach managed agents to repair the reported position and finish after the first successful render.

## Out of scope

- Process supervision, provider configuration, Git fetching, and workflow integration.
- Renderer layout changes or automatic JSON syntax repair.

## Acceptance

- Regression tests fail first for missing parser details and missing completion validation. They include quotes, raw tabs, and invalid escapes.
- A successful render produces an atomic receipt with PR number, event SHA, fixed paths, and file hashes. Any failed render leaves no current receipt.
- Verification rejects stale identity, symlinks, missing or mutated files, invalid schema, and mismatched HTML. It never changes the draft or final files.

## Verification

```bash
python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-render.test.py
python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-verify.test.py
(cd .agents/skills/pr-walkthrough/references && python3 -m unittest test_build)
python3 .github/scripts/lint-harness-files.py .agents/skills/pr-walkthrough
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `.agents/skills/pr-walkthrough/scripts/pr-walkthrough-render`
- `.agents/skills/pr-walkthrough/scripts/pr-walkthrough-render.test.py`
- `.agents/skills/pr-walkthrough/scripts/pr-walkthrough-verify` (new)
- `.agents/skills/pr-walkthrough/scripts/pr-walkthrough-verify.test.py` (new)
- `.agents/skills/pr-walkthrough/SKILL.md`
- `docs/specs/ui/requirements/pr-walkthrough.md`
- `docs/specs/ui/system-design/pr-walkthrough.md`

## Dependencies

None.

## Risks

The current renderer commits two files separately. The receipt must appear only after both replacements succeed. Standalone `references/build.py` use remains independent of managed event metadata.

## Parallelism

`sequential`

## Inputs

- Requirement AC-UI-PR-WALKTHROUGH-001.4, .7, .12, and .13.
- System-design sections: Generation completion, Rendering errors, and Output verification.
- Existing renderer tests and `references/build.py:build`.
- [Completion decision](../../decisions/2026-09-30-pr-walkthrough-render-completion.md).

## Results

Implemented actionable JSON parser diagnostics, managed completion receipts,
and a read-only verifier for the receipt, event identity, schema, hashes, and
exact HTML renderer output. Managed agents now stop after their first successful
render.

Verification passed:

- `python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-render.test.py` (10 tests)
- `python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-verify.test.py` (7 tests)
- `(cd .agents/skills/pr-walkthrough/references && python3 -m unittest test_build)` (71 tests)
- `python3 .github/scripts/lint-harness-files.py .agents/skills/pr-walkthrough`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`
