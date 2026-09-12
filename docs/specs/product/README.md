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

## Find product documents

Run the catalog command to find the current product documents:

    python3 scripts/list-docs.py specs --kind product --format markdown

These documents form the current proposed product baseline. They synthesize
the existing system specifications, ADRs, and public documentation. Statements
marked as open questions still need explicit product confirmation.
