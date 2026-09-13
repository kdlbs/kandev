---
id: "01-fence-budget-claims"
title: "Fence budget claims to policy revisions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-COSTS-002
acceptance_criteria:
  - AC-OFFICE-COSTS-002.1
  - AC-OFFICE-COSTS-002.2
  - AC-OFFICE-COSTS-002.3
  - AC-OFFICE-COSTS-002.5
  - AC-OFFICE-COSTS-002.8
  - AC-OFFICE-COSTS-002.10
  - AC-OFFICE-COSTS-002.11
  - AC-OFFICE-COSTS-002.13
  - AC-OFFICE-COSTS-002.14
system_design:
  - ../../specs/office/system-design/costs-03.md
---

# Task 01: Fence Budget Claims to Policy Revisions

## Scope

- Add a server-owned revision counter to each budget policy.
- Include the evaluated revision in the durable claim key and fence inserts to
  the current policy row.
- Recreate the legacy three-column claim table once when an upgraded database
  needs the four-column key.
- Claim the exceeded level and its alert companion atomically.
- Return the stored revision from create and update operations and cover the
  SQLite and PostgreSQL persistence paths.

## Verification

- Office cost, repository, and store conformance tests.
- PostgreSQL-gated revision and migration tests when a PostgreSQL DSN is
  available.
