---
status: draft
system: costs
specification_version: 1
migration: in_progress
owners:
  - kandev
---

# Cost and usage system

## Purpose

The cost and usage system owns token usage accounting, subscription quotas,
budgets, and cost-aware model routing.

## Ownership

This system owns usage events, cost projections, quota state, budget policy,
and cheap-model profiles used by task and Office execution.
It also owns source-aware session measurements and native usage statistics.
The existing task event ledger retains its immutable event-recording contract.

## Exclusions

- Agent profile identity belongs to the [agent system](../agents/README.md).
- Office-specific routing policy belongs to the [Office system](../office/README.md).

## Migration record

Migration remains in progress while legacy source detail is extracted from the
canonical requirement and system-design documents. Use the catalog command to
find them.

## Related systems

- [Agents](../agents/README.md): supplies model and profile identity.
- [Office](../office/README.md): consumes cost-aware routing.
- [Tasks](../tasks/README.md): owns session identity and the existing event ledger.
- [Plugins](../plugins/README.md): supplies collector permissions and lifecycle.
