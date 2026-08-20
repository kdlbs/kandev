# ADR-2026-08-20-mcp-related-task-read-authorization: Separate compact task-tree reads from document access

**Status:** accepted
**Date:** 2026-08-20
**Area:** backend, protocol, security

## Context

`list_related_tasks_kandev` needs to let an Office Coordinator monitor an unrelated task tree in its own workspace. The existing document guard correctly limits descriptions and task documents to self, tree relations, and blockers, but using it for the compact task projection prevents that monitoring.

Granting every task ambient same-workspace visibility would weaken the document-handoff boundary. Treating unknown and foreign targets differently would also create a target-existence oracle.

## Decision

The relation projection is split from sensitive document data. All callers retain relation-scoped reads. A backend-owned `workspace-task-tree-read` MCP capability grants only the persisted Office CEO/Coordinator session compact reads of unrelated tasks in its workspace. The MCP server derives this scope from its resolved profile and sends it in an internal payload field; callable parameters cannot request it.

Compact results contain task identity, title, state, parent shape, assignee label, relations, and linked pull requests. Descriptions are returned only for `verbose=true` requests that independently pass document-read authorization. Document keys are returned per node only when that node passes the same document guard. The capability never expands document or write access.

Unknown and cross-workspace targets return the same public `target_unavailable` denial. Other denied reads return a stable non-leaking reason. Every decision emits a structured application audit log, without creating activity rows.

## Consequences

Coordinators can monitor workspace task trees without accessing unrelated descriptions or documents. The task service owns projection authorization before loading sensitive fields, while the MCP server remains responsible for caller/session attestation. New MCP capabilities must continue to be backend-derived rather than supplied by agent arguments.

## Alternatives Considered

- Permit all same-workspace tasks to read compact trees. Rejected because ordinary workers do not need ambient visibility into unrelated work.
- Reuse document authorization for every projection. Rejected because it blocks the Coordinator monitoring workflow and couples private document access to task topology.
