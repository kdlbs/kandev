# Decision Log

Architecture Decision Records (ADRs) describe durable architecture, ownership,
boundary, contract, and repository decisions for Kandev.

This page is a static entry page. It does not contain a generated decision
table. The decision files are the source of truth.

## Find decisions

Run the catalog command from the repository root:

    python3 scripts/list-docs.py decisions --format markdown

Use filters when you need a smaller result:

    python3 scripts/list-docs.py decisions --status accepted --format paths
    python3 scripts/list-docs.py decisions --area backend --format markdown
    python3 scripts/list-docs.py decisions --text restart --format json

The command sorts numeric and date-prefixed decision IDs in a stable order.
Use paths for shell tools and JSON for structured consumers.

## Validate decisions

Run the repository validation command before you commit documentation changes:

    python3 scripts/list-docs.py validate

The command reads status, date, and area metadata from each ADR. It reports
invalid metadata and duplicate catalog identities. It does not rewrite files.

## Create a decision

Use the record skill or follow the ADR format in
0001-file-based-knowledge-system.md. Use the complete filename stem as the
stable decision ID. New decisions use a date-prefixed filename so branches do
not reserve a shared sequence number.


