---
created: 2026-09-07
status: complete
requirements: []
system_design: []
---

# Implementation Plan: On-demand documentation catalogs

## Overview

Replace conflict-prone decision and specification lists with one on-demand
catalog command. Keep static entry pages and durable system boundaries in
Markdown. Derive all document listings from filenames, paths, headings, and
metadata.

This work changes repository authoring only. It does not change Kandev product
behavior. The design package therefore has no product requirement or system
design documents. ADR-2026-09-07-on-demand-document-catalogs records the durable
repository rule.

All work lands in one pull request. The tasks are sequential because they share
the command contract, documentation, and validation workflow.

## Scope

### In scope

- Add one Python command with `decisions` and `specs` catalog views.
- Filter decisions by status, area, and text.
- Filter specifications by system, document kind, status, and text.
- Support Markdown, path-only, and JSON output.
- Add deterministic sorting, metadata validation, and focused unit tests.
- Replace the decision and specification indexes with static entry pages.
- Remove derived specification maps from every system README.
- Update specification guides, templates, agent guidance, and relevant skills.
- Run catalog validation in pre-commit and CI without rewriting files.

### Out of scope

- Generated catalog files committed to the repository.
- A bot or post-merge pull request.
- A complete ADR or specification metadata-format migration.
- Public documentation navigation in `docs/public/meta.json`.
- Public documentation ownership in `docs/public/coverage.json`.
- A repository-wide plans catalog or changes to initiative plan layout.
- Product behavior, application code, or browser end-to-end tests.

## Technical approach

Add `scripts/list-docs.py` as the author-facing command. Keep decision and
specification parsing separate inside the command. Share only filtering,
formatting, and error reporting.

For decisions, derive the identifier from the filename. Read the title from the
first ADR heading. Read status, date, and area from the existing metadata block.
Normalize the leading status class for filtering, but preserve the complete
status text in output. Sort numeric identifiers by number, then sort dated
identifiers by filename.

For specifications, derive the system and document kind from the path. Read the
title and current frontmatter fields. Keep path classification, frontmatter
parsing, and status sets in the shared `scripts/spec_metadata.py` helper used by
both `scripts/list-docs.py` and `scripts/lint-spec-files.py`. The catalog owns
filtering and output formats. It does not become a second source of
specification policy. Recognized catalog kinds are `system`, `glossary`,
`requirement`, `system-design`, `product`, and `legacy`.

The command exits with a nonzero status for malformed metadata, duplicate
catalog identities, unsupported filter values, or unreadable source files. An
explicit validation mode checks both trees without printing a catalog.

Use Markdown for terminal-friendly discovery, paths for shell composition, and
JSON for structured consumers. Escape generated Markdown cells. Keep JSON keys
stable and sort output before formatting.

Static index pages contain purpose, document format, and command examples. A
system README keeps durable ownership and migration information. It does not
repeat the files below its own directory.

## Test strategy

Focused Python tests use temporary document trees. They cover parsing, sorting,
filters, output formats, Markdown escaping, malformed metadata, duplicate
identities, empty results, and parity across all specification source kinds.
Repository validation then proves that the real decision and specification
trees satisfy the command contract.

The existing specification linter stays authoritative for specification
structure and size rules. Harness lint covers changed agent instructions and
skills. No application or end-to-end tests apply.

## Work orders

- [x] [Task 01: Build the documentation catalog command](task-01-build-catalog-command.md)
- [x] [Task 02: Replace tracked derived catalogs](task-02-replace-derived-catalogs.md)
- [x] [Task 03: Wire authoring guidance and validation](task-03-wire-authoring-validation.md)

## Verification

```bash
python3 scripts/list-docs.test.py
python3 scripts/list-docs.py validate
python3 scripts/list-docs.py decisions --status accepted --format markdown
python3 scripts/list-docs.py specs --system ui --kind requirement --format paths
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
python3 scripts/lint-harness-files.test.py
python3 .github/scripts/lint-harness-files.py --all
pre-commit run docs-catalog --all-files
git diff --check
```

All listed commands passed on 2026-09-07.

## Risks

- ADR metadata has older variants. Tests must cover the forms present in the
  repository before the tracked table is removed.
- A catalog parser can drift from the linter. Keep path classification,
  frontmatter parsing, and status sets in the shared helper. Test all source
  kinds against representative files.
- Removing maps can remove useful ownership context. Keep durable boundary and
  migration text in every system README.
- Existing links can target table rows or map sections. Search and update these
  references before removing the sections.

## Rollback

Revert the pull request. The source decision and specification files remain
unchanged, so the previous tracked tables can be restored without data loss.
