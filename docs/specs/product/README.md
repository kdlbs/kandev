# Product Specifications

This directory contains Kandev-wide product context. These documents apply to
several systems and help readers interpret system requirements.

Add a document only when the team has confirmed its content. Do not create
empty product documents from a template.

The expected subjects are:

- Product overview and boundary.
- Actors and their goals.
- Product map and system relationships.
- Durable product principles.
- Success measures that the team uses.
- Cross-system product constraints.

System-specific behavior belongs in the owning system under `requirements/`.
Technical implementation belongs in `system-design/`.

## Product document index

These documents form the current proposed product baseline. They synthesize
the existing system specifications, ADRs, and public documentation. Statements
marked as open questions still need explicit product confirmation.

- [Product overview](overview.md): purpose, boundary, and the supported product loop.
- [Actors](actors.md): people, agents, services, and extensions that interact with Kandev.
- [Product map](product-map.md): product surfaces, system relationships, and authority boundaries.
- [Principles](principles.md): durable product principles inferred from current decisions.
- [Success measures](success-measures.md): proposed measures and the targets that remain to be set.
- [Product constraints](product-constraints.md): cross-system safety, support, and compatibility constraints.
