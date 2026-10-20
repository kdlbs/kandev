---
status: done
system: platform
requirements:
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-004
system_design:
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
---

# PostgreSQL ACP session metadata repair

Status: done. Issue: [#4525](https://github.com/kdlbs/kandev/issues/4525).

## Scope and applicability

Base: `941b03ceabce82191b5d062d49d76aec7936a369` (freshly fetched main).
The controller authorized bounded regression repair without a separate planning handoff.

The issue reports two distinct operations. Workspace policy attachment creates an
Office workspace group. Version v0.97.0 passed a Go boolean to its integer
`owned_by_kandev` column. Merged [#4461](https://github.com/kdlbs/kandev/pull/4461),
commit `a3bfd7ba5950bc458c266bcfcd2afa01bbb1ac3a`, already converts that value
with `boolToInt` and tests PostgreSQL creation, materialization and membership.
No duplicate workspace production change is needed. Open #4538 changes launch
wrappers and does not modify these persistence methods.

The ACP mirror remains broken: `SetSessionACPSessionID` sends SQLite
`json_patch`, `json_extract` and `IS NOT ?` to PostgreSQL. A real PostgreSQL 16
regression fails with the reported syntax error near `$4`; 10 of 11 equivalent SQLite
cases pass. The remaining empty-string case exposes an unnormalized comparison
inside the same mirror statement. This is independent of workspace boolean encoding.

## Requirement and design

Restore AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.3 and
AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-004.3 in the existing
[requirements](../../specs/platform/requirements/postgres-domain-store-parity.md).
Use the existing [SQL dialect design](../../specs/platform/system-design/postgres-domain-store-parity.md#schema-and-query-contracts).
No new product contract, schema, migration, authorization or ownership policy.

## Delivery

1. [Task 01](task-01-repair-acp-mirror.md): shared dual-engine behavior regression,
   PostgreSQL JSONB statement, existing wiring and workspace regression checks.
2. Report the uncommitted candidate and exact evidence to the controller for
   validation. Commit/PR delivery requires its later authorization.

## Boundaries

Keep the mirror atomic and conditional on the matching `executors_running`
resume token. Preserve unrelated metadata and ACP keys; unchanged IDs and
rotated/missing executions must not update timestamps. Use real disposable
PostgreSQL schemas via `testutil.OpenIsolatedPostgres`; never deployment data.
No generic SQL cleanup, migrations, live ECS/RDS validation or release claim.

Public documentation impact: none. This restores an existing internal metadata
write under the documented PostgreSQL support contract; no commands or operator
configuration change. Reviewed the public backend-development and testing guides.

Implementation and task-defined verification are complete. Controller validation
approved the candidate and authorized protected PR delivery. Current CI/review
and merge evidence remains externally pending until confirmed.
