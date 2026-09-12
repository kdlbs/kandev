# ADR-2026-09-07-on-demand-document-catalogs: Derive Documentation Catalogs on Demand

**Status:** accepted
**Date:** 2026-09-07
**Area:** workflow, infra

## Context

The decision log and specification catalog contain lists that repeat data from
the documents they describe. Every new architecture decision changes
`docs/decisions/INDEX.md`. Specification changes also update
`docs/specs/INDEX.md` and large maps in system README files.

These shared files change in many pull requests. Concurrent pull requests then
conflict even when they add unrelated documents. The copied rows also become
stale when a document moves or its metadata changes.

The repository already stores the required catalog data in filenames, headings,
paths, and document metadata. A catalog does not need a tracked generated copy.

## Decision

Treat architecture decision files and specification files as the catalog source
of truth. Provide one repository script that lists and filters both document
types on demand.

The command must support these catalog views:

- Decisions, with filters for status, area, and text.
- Specifications, with filters for system, document kind, status, and text.
- Markdown, path-only, and JSON output for people and other tools.

The command must use deterministic ordering. It must validate required metadata
and report malformed source files with actionable errors. Pre-commit and CI must
run validation. They must not rewrite documentation files.

Keep `docs/decisions/INDEX.md` and `docs/specs/INDEX.md` as short entry pages.
They describe the document model and show catalog commands. They do not contain
rows for individual documents.

Keep each specification system README as a durable boundary document. It keeps
the purpose, scope, ownership, exclusions, migration record, and related-system
links. Remove exhaustive requirement and system-design maps from these files.
The catalog command derives those maps from paths and frontmatter.

Keep manually curated information manual when file metadata cannot derive it.
This includes public documentation navigation in `docs/public/meta.json` and
the semantic ownership map in `docs/public/coverage.json`. Plans remain grouped
by initiative and do not need a repository-wide catalog.

This decision amends ADR-0001 and
ADR-2026-08-22-system-oriented-specifications. It changes repository authoring,
not Kandev product behavior. No product requirement or system design changes.

## Consequences

Unrelated document additions no longer edit one shared list. Authors can find
documents with filters instead of scanning long Markdown tables.

Catalog output always reflects the checked-out branch. Consumers that need a
snapshot can request JSON or Markdown from the script without committing the
result.

The repository gains one small Python command and focused tests. Authors must
keep source metadata valid because validation replaces manual list maintenance.

Links to the old exhaustive tables and system maps need an update. Agent skills,
authoring guides, templates, pre-commit, and CI must use the new command.

## Alternatives Considered

### Commit generated catalog files

This option keeps browsable tables on GitHub. It still makes concurrent pull
requests update the same files. Deterministic generation does not remove merge
conflicts.

### Update catalogs in a follow-up bot pull request

This option removes catalog edits from feature branches. It adds automation,
permissions, another pull request, and a delay before the catalog is accurate.

### Generate catalogs in each pull request

A pre-commit hook can rewrite the tables in the same pull request. The rewrite
still creates the shared-file conflict that this decision removes.

### Split catalogs by date or system

Sharding reduces the conflict rate. It keeps duplicated data and manual index
maintenance. On-demand discovery removes both problems.

### Use separate scripts for decisions and specifications

Separate commands avoid shared implementation. One command gives authors one
discovery interface and one output contract. The two catalogs remain separate
subcommands with type-specific parsers and filters.
