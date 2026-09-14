---
owners:
  - kandev
requirements:
  - REQ-OFFICE-TASKS-001
  - REQ-OFFICE-AGENTS-001
  - REQ-PLUGINS-PLUGINS-001
system_design:
  - ../../specs/office/system-design/tasks-01.md
  - ../../specs/office/system-design/agents-01.md
  - ../../specs/plugins/system-design/plugins-04.md
created: 2026-09-14
status: complete
---

# Implementation Plan: Coordinator Related-Task Reads

## Overview

Separate the compact task-relation projection that `list_related_tasks_kandev`
returns from private description and document access, so the persisted Office
CEO/Coordinator can monitor unrelated task trees in its own workspace without
gaining document or write authority.

Related-task reads stay relation-scoped for every caller: self, ancestors,
descendants, non-root siblings, and blockers continue to work without any new
permission. The backend derives a `workspace-task-tree-read` MCP capability
only for the persisted Office CEO/Coordinator session whose AgentProfileID
matches the workspace's resolved CEO profile; the internal MCP server attests
the caller and read scope in hidden backend payload fields that callable
arguments cannot supply. Ordinary workers requesting an unrelated same-workspace
target receive a structured, non-leaking denial.

Compact results carry task identity, title, state, relationship shape, assignee
label, and linked pull requests. Descriptions remain visible only through
`verbose=true` requests that independently pass document-read authorization,
and document keys are returned per node only when that node passes the same
guard. Unknown and cross-workspace targets share one public denial reason so
target existence is never disclosed. Every authorization decision emits a
structured audit event without persisting activity rows.

The same projection is exposed to plugins as a capability-gated Host RPC
(`GetTaskRelations`, capability `api_read:task_relations`) so a plugin can
inspect task topology without the broader `api_read:tasks` read surface.

## Requirement coverage

| Requirement | Covered by |
| --- | --- | 
| `REQ-OFFICE-TASKS-001` | Relation-scoped `list_related_tasks_kandev` behavior and the Coordinator compact-tree exception, delivered by [task-01-coordinator-relation-authorization](task-01-coordinator-relation-authorization.md). |
| `REQ-OFFICE-AGENTS-001` | Persisted CEO/Coordinator capability derivation and its independent document-read boundary (`AC-OFFICE-AGENTS-001.9`), delivered by [task-01-coordinator-relation-authorization](task-01-coordinator-relation-authorization.md). |
| `REQ-PLUGINS-PLUGINS-001` | Capability-gated `GetTaskRelations` Host RPC on the frozen plugin contract, delivered by [task-01-coordinator-relation-authorization](task-01-coordinator-relation-authorization.md). |

## Work packages

- [x] [task-01-coordinator-relation-authorization](task-01-coordinator-relation-authorization.md) -
  MCP profile capability, dispatcher attestation, task-service projection
  authorization, plugin Host RPC, audit events, and the specification,
  decision, and public documentation updates.

## Verification

Focused suites:

```bash
cd apps/backend && go test ./internal/task/service/ ./internal/mcp/... ./internal/plugins/ ./pkg/pluginsdk/ ./internal/backendapp/
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
```

The full repository gates (`make -C apps/backend test`, web typecheck, lint)
must pass or be classified against a clean upstream baseline before delivery.
