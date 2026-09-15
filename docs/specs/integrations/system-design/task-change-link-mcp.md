---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001
created: 2026-09-14
owners:
  - kandev
---

# Manage task change requests through MCP System Design

## Purpose and boundaries

This design gives agents one provider-neutral MCP surface for managing a
task's GitHub pull request and GitLab merge request associations. The MCP
tool surface and the task-facing handler are owned here; provider stores,
webhook discovery, and automation handling stay owned by their existing
GitHub and GitLab designs, and task identity, workspace reach authorization,
and list/serialization surfaces stay owned by the task system.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001` | [Components and responsibilities](#components-and-responsibilities), [Control flow](#control-flow) |

## Components and responsibilities

- `apps/backend/internal/mcp/handlers/task_change_link.go` validates the
  request shape, binds the caller task identity through the ordinary task
  reach path, and forwards one provider-neutral request to the coordinator.
- `apps/backend/internal/backendapp/task_change_link_coordinator.go`
  resolves the canonical repository from the task's workspace, rejects
  unknown or unverified provider hosts, dispatches to the provider service,
  and returns the resulting linked set. Replacement compensates a failed
  post-establishment exchange by removing the newly created association and
  reports both errors when compensation itself fails.
- `apps/backend/internal/github` and `apps/backend/internal/gitlab` own
  association persistence, idempotent link/removal, and exact automation
  retirement scoped to one provider, repository, and change-request number.
  GitHub detach keeps durable tombstones; GitLab unlink publishes a
  workspace-scoped deletion event after persistence.
- `apps/backend/internal/mcp/server/server.go` registers
  `link_task_pr_kandev`, `unlink_task_pr_kandev`, and
  `replace_task_pr_kandev` with provider-aware availability.

## Data and contracts

Every mutation requires explicit `task_id`, a provider discriminator
(`github` or `gitlab`), the canonical repository identity (`repository_id`),
and the change-request number. A bare number is invalid. Replacement adds the
old association triple (`old_provider`, `old_repository_id`,
`old_number`). Responses return the current canonical linked set for the
task.

Unlinking changes only the active association row. Conversation history and
terminal receipts remain untouched, and re-linking the same identity restores
an active association.

## Control flow

1. The MCP server receives the tool call and resolves the caller task.
2. The handler authorizes workspace reach for that task.
3. The handler validates provider, repository identity, and number, rejecting
   a bare number or unresolved repository before any store mutation.
4. The coordinator loads the task's repositories, verifies the requested
   canonical repository belongs to the task workspace and carries a verified
   provider host, and dispatches link, unlink, or replace.
5. The provider service applies idempotent persistence, scoped automation
   cleanup, and deletion-event publication.
6. The coordinator assembles the resulting linked set and the handler returns
   it. List/query surfaces read the same stores, so no extra invalidation is
   required for restart-consistent state.

## GitHub caller identity

GitHub associations use the existing workspace-scoped personal-read resolver.
The user identity comes from trusted host context, never tool arguments.
An existing non-empty caller identity is preserved. When an in-session MCP
request has no identity, host wiring may supply the same synthetic single-user
identity used by HTTP requests only when the auth service explicitly reports
disabled mode. Setup, enabled, and unavailable auth state do not permit this
fallback. An explicitly present but empty identity is rejected.

This fallback is local to the GitHub association operation. It does not attach
an identity to all MCP dispatches or change task reach, repository validation,
provider credential selection, or the auth scope resolver's disabled-mode
contract. The coordinator consumes a host-provided identity resolver; auth mode
selection remains in backend composition. Link and replacement use the same
resolution path before provider work. Failed resolution leaves associations
unchanged.

This completes the technical path for
`AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.1` while preserving `.2` and `.4`.
It follows the existing
[opt-in authentication decision](../../../decisions/2026-07-24-opt-in-authentication.md)
and [GitHub identity ownership](../../../decisions/0047-github-authentication-ownership.md).
Delivery is tracked in the
[single-user PR-link repair plan](../../../plans/mcp-pr-link-single-user/plan.md).

## Tool example

```json
{
  "task_id": "<task-id>",
  "provider": "github",
  "repository_id": "<repository-id>",
  "number": 3506
}
```

`link_task_pr_kandev` and `unlink_task_pr_kandev` accept this shape;
`replace_task_pr_kandev` additionally requires the old association triple
(`old_provider`, `old_repository_id`, `old_number`). See
[docs/public/automation-and-mcp.md](../../../public/automation-and-mcp.md)
for the operator-facing contract.
