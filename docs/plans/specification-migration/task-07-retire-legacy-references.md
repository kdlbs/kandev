---
id: "07-retire-legacy-references"
title: "Retire legacy specification references"
status: completed
wave: 7
depends_on:
  - "06-migrate-ui-standalone"
plan: "plan.md"
requirements: []
acceptance_criteria: []
system_design: []
---

# Task 07: Retire legacy specification references

## Summary

Repair every plan, decision, index, and cross-specification link that points at
the old paths. Remove obsolete legacy size exceptions and convert the catalog
into a concise record of the completed migration.

## In scope

- `docs/plans/**`
- `docs/decisions/**`
- `docs/specs/INDEX.md`
- Cross-references in `docs/specs/**` and affected public docs.
- `legacy_size_exceptions` entries for removed files.

## Out of scope

- Rewriting unrelated plan or decision content.
- Changing stable requirement or acceptance IDs.

## Acceptance

- No tracked Markdown file references a removed legacy specification path.
- Every `spec:` frontmatter reference targets an existing canonical document.
- The catalog and each system README agree on authoritative ownership.

## Verification

```bash
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/decisions docs/plans
```

## Files likely touched

- `docs/specs/INDEX.md`
- `docs/specs/spec-lint.json`
- `docs/plans/**`
- `docs/decisions/**`

## Dependencies

Tasks 02 through 06.

## Risks

- Blind path replacement can break relative links or point a plan at a design
  instead of a requirement. Validate each link target and document type.

## Parallelism

`sequential`

## Inputs

- All completed system READMEs.
- `docs/specs/guide/traceability-and-lifecycle.md`.
- `rg` inventories of old `spec.md` and `plan.md` references.

## Results

Completed. Repaired 1,195 Markdown files containing relative or repository-root
specification references, removed all legacy size exceptions, and updated
historical decision references that pointed at removed specification paths.
