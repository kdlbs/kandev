---
id: "08-verify-complete-migration"
title: "Verify complete specification migration"
status: completed
wave: 8
depends_on:
  - "07-retire-legacy-references"
plan: "plan.md"
requirements: []
acceptance_criteria: []
system_design: []
---

# Task 08: Verify complete specification migration

## Summary

Run the repository's structural and link checks, inspect the final diff for
lost source detail, and record the verification results in the migration plan.

## In scope

- Specification linter unit tests and full lint.
- Markdown link and `spec:` reference audits.
- Size-limit, duplicate-ID, frontmatter, and migration-state checks.
- Final diff review for deleted or unintentionally changed source content.

## Out of scope

- Broad application tests unrelated to documentation.
- Semantic redesign of an existing product contract.

## Acceptance

- All specification tests and lint checks pass.
- No legacy `spec.md` or `plan.md` remains in an editable system tree.
- Every completed system has `migration: complete` and an authoritative map.

## Verification

```bash
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
make lint-specs
git diff --check -- docs/specs docs/decisions docs/plans
```

## Files likely touched

- `docs/plans/specification-migration/plan.md`

## Dependencies

Task 07.

## Risks

- A passing structural linter does not prove that a migrated document kept all
  behavior detail. Compare each moved source and its canonical replacement.

## Parallelism

`sequential`

## Inputs

- The completed migration diff.
- The specification guide and linter output.

## Results

Completed. The specification test suite, full specification lint, size and
duplicate-ID checks, frontmatter checks, migration-state checks, and diff
whitespace audit all passed. The final inventory contains no editable legacy
`spec.md` or `plan.md` under `docs/specs/`.
