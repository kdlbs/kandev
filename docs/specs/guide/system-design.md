# System Design

## Purpose

A system design explains how Kandev satisfies one or more requirements. It is a
living technical specification that maps to real runtime boundaries and code.

## Required content

Every system design identifies its owning system and its requirement IDs. Use
an empty `requirements` list only when the design defines internal technical
infrastructure with no independent product requirement.

A design contains the sections that its subject needs. Common sections include:

- Context and boundaries.
- Components and responsibilities.
- Data models and contracts.
- Control flow and state transitions.
- Failure and recovery behavior.
- Permissions and security boundaries.
- Persistence and migration behavior.
- Observability.
- Related ADRs.

Do not add empty sections.

## Code grounding

Reference real packages, types, endpoints, events, tables, and configuration
keys. Use Markdown links for repository documents. Use code formatting for code
symbols.

Describe stable boundaries and behavior. Do not copy complete source code or
list every helper function.

## Requirement mapping

Map each referenced requirement to the design section that satisfies it. A
short table is sufficient.

Do not copy requirement text into the design. Reference the stable `REQ-*` and
`AC-*` identifiers.

## Vertical designs

Keep a design with the system that owns its requirements. Cover every stable
runtime boundary needed for the outcome. A single integration design can cover
a provider client, storage, HTTP and WebSocket projections, frontend state,
React components, responsive behavior, and tests.

Do not create parallel backend and UI designs for one feature contract. Link to
another system design when that system owns an independent dependency. Do not
list another system's requirement in design frontmatter.

Split a large design by capability, lifecycle, or contract boundary. If a
document needs numbered parts, keep every part under the same owning system and
map each part to specific design sections. Do not split only at the frontend and
backend boundary.

## Decisions

Use an ADR when a choice creates a durable boundary, contract, ownership rule,
or operational invariant with meaningful alternatives. Link the ADR from the
design.

Do not create locally numbered ADR sections inside a system design. The global
decision log is the source of truth for design rationale.

## Design changes

Update the design when implementation changes a documented boundary or
contract. Do not update it for a local refactor that preserves the design.

When code and design disagree, stop and identify the intended source of truth.
Do not silently rewrite the design to match accidental behavior.
