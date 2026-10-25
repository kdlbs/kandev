---
id: "01-coordinator-relation-authorization"
title: "Authorize compact related-task reads for the Office Coordinator"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-TASKS-001
  - REQ-OFFICE-AGENTS-001
  - REQ-PLUGINS-PLUGINS-001
acceptance_criteria:
  - AC-OFFICE-TASKS-001.8
  - AC-OFFICE-AGENTS-001.9
  - AC-PLUGINS-PLUGINS-001.7
  - AC-PLUGINS-PLUGINS-001.11
system_design:
  - ../../specs/office/system-design/tasks-01.md
  - ../../specs/office/system-design/agents-01.md
  - ../../specs/plugins/system-design/plugins-04.md
---

# Task 01: Authorize Compact Related-Task Reads for the Office Coordinator

## Goal

The persisted Office CEO/Coordinator can monitor any task tree in its workspace
through `list_related_tasks_kandev` with a compact, read-only projection, while
descriptions, document keys, document bodies, and write access remain governed
by the existing relation-scoped document guard. Plugins gain the same compact
topology through a capability-gated Host RPC.

## Scope

- Split the related-task projection from document authorization in
  `apps/backend/internal/task/service`: relation-scoped callers keep their
  current access; the resolved Office CEO/Coordinator session receives the
  backend-owned `workspace-task-tree-read` capability for unrelated
  same-workspace targets.
- Derive the capability only from persisted task/session and Office CEO data;
  the internal MCP server attests the caller and resolved scope in hidden
  backend payload fields that are absent from public callable arguments.
- Return stable structured denial reasons (`related_task_scope_required`,
  `verbose_document_scope_required`, `target_unavailable`) with no target
  existence, workspace, title, or relationship leakage for unknown and
  cross-workspace targets.
- Emit one structured audit event per authorization decision without
  persisting activity rows; the read path performs no task, relationship,
  blocker, document, session, workflow, or activity mutation.
- Add the `GetTaskRelations` Host RPC (capability `api_read:task_relations`)
  to the frozen plugin contract, the Go SDK, and generated stubs.
- Record the decision, Office specifications, Office context prompt, public
  coordination documentation, and the callable schema policy consistently.

## Out of scope

- Any broadening of document read/write rules or mutation paths.
- Ambient same-workspace visibility for ordinary worker tasks.

## Verification evidence

Focused suites from the plan all passed at the reviewed head: task-service
authorization tests (including the production-like integration reproduction
that creates self/tree/unrelated/cross-workspace/unknown fixtures), MCP handler
and dispatcher tests, plugin host/SDK tests, spec lint, and the public docs
validation suite.
