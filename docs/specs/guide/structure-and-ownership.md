# Structure and Ownership

## Specification root

`docs/specs` is the root for durable Kandev specifications. The directory uses
systems as its primary organization.

```text
docs/specs/
├── product/
└── <system>/
    ├── README.md
    ├── glossary.md
    ├── requirements/
    └── system-design/
```

A system owns a stable set of concepts, behavior, and technical boundaries.
Examples include the task system, agent runtime, plugin system, and GitHub
integration.

## System test

Create a system directory when at least two statements are true:

- The system owns a stable vocabulary or data model.
- Multiple consumers depend on its behavior or guarantees.
- The system has an independent lifecycle.
- A team can design and review its changes independently.
- Several features or other systems depend on it.

Do not create a system for one helper, component, page, or endpoint. Put that
design in the system that owns the behavior.

## System ownership

Each requirement has one owning system. Another system can reference the
requirement but must not copy it.

Each system has a `README.md`. This file defines the system boundary and links
to its requirements and system designs. Use `glossary.md` only when the system
has terms that need precise definitions.

The system index sets `migration: in_progress` while legacy files remain. Set
it to `complete` only after the index names the new authoritative documents and
all editable legacy sources become links or archives. The linter enforces the
strict system layout after migration is complete.

A system does not need both artifact directories. A low-level system can have
system designs without product requirements. Do not create empty placeholder
documents.

## Product specifications

`docs/specs/product` contains Kandev-wide context. It can contain these files:

- `overview.md` explains the product problem and boundary.
- `actors.md` defines people, agents, plugins, and external systems.
- `product-map.md` maps the systems and their relationships.
- `principles.md` defines durable product principles.
- `success-measures.md` defines measures that the team uses.
- `product-constraints.md` defines cross-system quality and compatibility
  requirements.

Do not put feature requirements, API details, roadmaps, or work status in the
product directory.

## Cross-system design

The system that owns a contract also owns its design. Other system designs link
to that contract. Do not create a separate shared-design tree only because two
systems use the same capability.

Create a first-class system when shared behavior has independent ownership and
guarantees. Event delivery and persistence can become systems under this rule.

## Vertical feature ownership

Choose the owner from the durable contract, not from the code directories that
the implementation changes. A capability can change backend, frontend, mobile,
and test code while one system owns its requirements and design.

User visibility does not make a capability UI-owned. The UI system owns an
interaction contract only when that contract remains useful without the feature
or backend state that first uses it. Provider state, task state, permissions,
and persistence stay with their owning systems. Their requirements can include
desktop, mobile, accessibility, and failure outcomes.

For example, a GitHub merge-queue capability belongs to the integration system.
Its integration requirement describes the visible queue controls and states.
Its integration design describes the GitHub client, persistence, API projection,
and React components. Do not create a second UI requirement or design for those
same controls.

Create separate cross-system artifacts only when each system owns an independent
contract with a different lifecycle. Link the contracts in both system indexes.
Do not repeat acceptance criteria or technical sections.

## File names

Use a short kebab-case capability name. Use the same file name for the paired
requirement and system design when practical.

```text
task-system/requirements/dependencies.md
task-system/system-design/dependencies.md
```

Do not create new files named `spec.md`. The file name must identify the
capability.

## Context limits

The specification linter reads the limits from `docs/specs/spec-lint.json`.
The default limits are:

- System index: 12 KiB.
- Product or guide document: 16 KiB.
- Requirement document: 20 KiB.
- System-design document: 32 KiB.
- Template: 8 KiB.
- Unmigrated legacy specification: 32 KiB.

Split a file before it reaches its limit. Split by capability, lifecycle, or
contract boundary. Do not split by arbitrary line ranges.

An oversized legacy file can have a frozen ceiling in the linter configuration.
The ceiling permits migration work but does not permit growth. Lower the ceiling
in the same change whenever the file shrinks. Remove the exception after the
file falls below the default limit.
