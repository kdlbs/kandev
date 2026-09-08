# ADR-2026-08-22-system-oriented-specifications: Organize Specifications by System

**Status:** accepted
**Date:** 2026-08-22
**Area:** workflow, infra

## Context

Kandev stores product requirements, technical contracts, and implementation
details in `docs/specs`. Many files now contain all three types of information.
This mixture makes the files difficult to find, maintain, and load into an
agent context.

The repository also uses two directory patterns. Some specifications use a
product group such as `tasks/` or `office/`. Other specifications use one
directory for each feature. The mixed structure weakens ownership and creates
large standalone documents.

## Decision

Keep `docs/specs` as the root for durable product and system specifications.
Organize new specifications by an owning system. Each system can contain these
directories:

- `requirements/` defines required and observable behavior.
- `system-design/` defines the technical design that satisfies requirements.

Use `docs/specs/product/` for Kandev-wide purpose, actors, principles, success
measures, constraints, and the product map. Keep plans and work orders in
`docs/plans`. Keep architecture decisions in `docs/decisions`.

Use stable `REQ-*` identifiers for requirements. Use stable `AC-*` identifiers
for acceptance criteria. System designs and work orders reference these
identifiers. Tests reference acceptance criteria when the mapping is useful.

Use `docs/specs/guide/` as the canonical authoring guide. Skills apply this
guide but do not duplicate its complete rules. A repository linter enforces
the mechanical rules, including file-size limits.

Do not move all legacy specifications in one mechanical change. A migration
must separate product intent from technical design. Each migrated system must
name one new source of truth before legacy content becomes a link or archive.

This decision amends the specification layout in ADR-0001. It does not change
the file-based and progressively loaded knowledge model.

No product specification needs an update. This decision changes the repository
authoring system and not Kandev product behavior.

## Consequences

Requirements and system designs stay close because they share an owning
system. Agents can load one system index and select only the necessary files.

The term `specification` becomes an umbrella term. New documents do not use a
generic `spec.md` filename. Authors must identify each document as a product
document, requirement, system design, ADR, plan, or work order.

The repository needs a controlled migration for existing specifications.
Legacy files remain valid until their owning system migrates. Oversized legacy
files cannot grow beyond their recorded size ceiling.

## Alternatives Considered

### Organize by artifact type

This option used separate top-level trees for requirements and technical
designs. It made artifact types easy to list, but it separated documents that
agents usually read together.

### Put each feature in its own directory

This option put `requirements.md` and `system-design.md` in a feature directory.
It gave strong local grouping, but it recreated the existing standalone-feature
sprawl.

### Use the term blueprint

This option followed an external software-factory model. The term did not match
Kandev terminology and implied a more rigid document than a living system
design.

