# Specification Catalog

Kandev specifications describe product intent, required behavior, and system
design. System README files define durable boundaries. Requirement and
system-design files define the owned contracts.

This page is a static entry page. It does not contain a generated list of
systems or documents. The specification files are the source of truth.

## Find specifications

Run the catalog command from the repository root:

    python3 scripts/list-docs.py specs --format markdown

Use filters for common discovery tasks:

    python3 scripts/list-docs.py specs --system ui --kind requirement --format paths
    python3 scripts/list-docs.py specs --status active --format markdown
    python3 scripts/list-docs.py specs --text workflow --format json

The command recognizes these kinds:

- system: a system boundary README with specification frontmatter.
- glossary: a system glossary.md file.
- requirement: a document under a system requirements directory.
- system-design: a document under a system system-design directory.
- product: a product-wide document under docs/specs/product/.
- legacy: a document that remains outside the migrated layout.

Use paths for shell tools and JSON for structured consumers. The command
sorts results deterministically and searches document text without changing
the source files.

## Validate specifications

Run both repository checks before you commit specification changes:

    python3 scripts/list-docs.py validate
    python3 scripts/lint-spec-files.py --all

The catalog validates the metadata it needs for discovery. The specification
linter remains authoritative for IDs, cross-references, migration rules, and
file-size limits.

## Authoring rule

Keep system purpose, ownership, exclusions, migration history, and related
links in each system README. Do not add catalog rows to this page or repeat
requirement and system-design lists in a system README.
