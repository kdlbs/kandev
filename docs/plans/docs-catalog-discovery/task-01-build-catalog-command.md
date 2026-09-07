---
id: "01-build-catalog-command"
title: "Build the documentation catalog command"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements: []
acceptance_criteria: []
system_design: []
---

# Task 01: Build the documentation catalog command

## Summary

Add one tested Python command that discovers architecture decisions and
specifications from repository source files.

## In scope

- Add `scripts/list-docs.py` with `decisions`, `specs`, and `validate`
  subcommands.
- Add decision filters for status, area, and case-insensitive text.
- Add specification filters for system, kind, status, and case-insensitive
  text.
- Add Markdown, path-only, and JSON output.
- Add deterministic sorting and actionable validation errors.
- Add focused tests in `scripts/list-docs.test.py`.

## Out of scope

- Editing any tracked documentation catalog.
- Changing the metadata format of existing documents.
- Adding a Python package dependency.

## Implementation details

Use only the Python standard library. Accept an internal root override in tests
so fixtures use temporary directories. Keep repository paths relative in all
output and errors.

Decision parsing must support metadata forms already present in the repository.
Use the filename stem as the identifier. Use the first ADR heading as the title.
Read status, date, and area from the metadata block. Filter status by its leading
class, such as `accepted`, while displaying the complete value.

Specification parsing derives `system` and `kind` from the path. Recognized
kinds are `system`, `requirement`, `system-design`, and `legacy`. Read titles and
metadata with the same meaning as the specification linter.

The JSON output contains a top-level schema version and an ordered `documents`
array. Each item contains the common path, title, and type fields plus the
type-specific metadata. Empty filters return an empty successful result.

## Acceptance

- The command finds every valid decision and specification source file.
- Repeated runs on the same tree produce byte-identical output.
- Every documented filter works alone and with other filters.
- Each output format represents the same ordered result set.
- Invalid metadata returns a nonzero exit status and identifies the file.
- Tests cover existing ADR metadata variants and specification frontmatter.

## Verification

```bash
python3 scripts/list-docs.test.py
python3 scripts/list-docs.py validate
python3 scripts/list-docs.py decisions --status accepted --format markdown
python3 scripts/list-docs.py specs --system ui --kind requirement --format paths
```

## Files likely touched

- `scripts/list-docs.py`
- `scripts/list-docs.test.py`

## Dependencies

None.

## Risks

- Loose ADR parsing can hide malformed files. Keep accepted variants explicit
  and reject ambiguous metadata.
- Markdown titles can contain table delimiters. Escape cells in the formatter.

## Parallelism

`sequential`

## Inputs

- `docs/decisions/*.md`
- `docs/specs/**/*.md`
- `scripts/lint-spec-files.py`
- `docs/decisions/2026-09-07-on-demand-document-catalogs.md`

## Results

Done. The catalog command finds 247 decisions and 708 specifications. Focused
tests, repository validation, and the documented filter examples pass.
