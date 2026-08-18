---
status: building
created: 2026-08-15
owner: kandev
---

# Worktree Branch Templates

## Why

Teams use repository-specific branch names for tickets and other local rules.
Kandev must preserve each saved template across backend restarts and redeployments.

## What

- Each workspace repository has one worktree branch template.
- A saved valid template replaces the template for that repository.
- A backend restart or schema replay does not change a non-empty saved template.
- A new repository uses `feature/{title}-{suffix}` when the request omits the template.
- An upgrade from the legacy prefix field derives the template once from that prefix.
- The legacy upgrade uses `feature/` when the stored prefix is empty.

The template syntax and placeholders follow
[ADR 0032](../../decisions/0032-configurable-worktree-branch-names.md).

## Data Model

The `repositories` table owns these fields:

| Field | Contract |
| --- | --- |
| `worktree_branch_template` | The current template for the repository. |
| `worktree_branch_prefix` | The legacy prefix used only to upgrade a repository that has no template field. |

## API Surface

The existing repository create, update, and read contracts use the
`worktree_branch_template` field. This repair does not change these contracts.

## Persistence Guarantees

The repository store keeps a saved template across process restarts and deployment
replacements. Startup schema work can initialize a legacy row, but it cannot replace
a non-empty template.

## Scenarios

- **GIVEN** a repository with a custom template, **WHEN** Kandev restarts, **THEN**
  the repository still has the exact custom template.
- **GIVEN** a legacy repository with prefix `fix/` and no template field, **WHEN**
  Kandev upgrades the database, **THEN** the template becomes
  `fix/{title}-{suffix}`.
- **GIVEN** a legacy repository with an empty prefix and no template field,
  **WHEN** Kandev upgrades the database, **THEN** the template becomes
  `feature/{title}-{suffix}`.
- **GIVEN** a migrated repository whose template was later changed, **WHEN** Kandev
  replays startup schema work, **THEN** the changed template remains unchanged.

## Out of Scope

- Recovery of a custom template that an earlier Kandev version already replaced.
- Changes to the template syntax, placeholders, validation, default value, or UI.
- Replacement of the current schema migration system.
