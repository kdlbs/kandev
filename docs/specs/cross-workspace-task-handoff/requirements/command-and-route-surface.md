---
status: active
system: cross-workspace-task-handoff
created: 2026-09-02
updated: 2026-09-15
owners:
  - nova28
---

# Command and route surface Requirements

## Overview

The handoff is reachable exactly one way: a `kandev task handoff` CLI
subcommand that POSTs to a new authenticated Office runtime route. No MCP tool
backs this capability on any surface, and every criterion governing the
request is enforced at the route, not only in the command, because the
command is not a trust boundary — an agent can reach the route directly.

### REQ-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001: One command, one route, no MCP tool

#### Acceptance criteria

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001.1:** The system shall expose
  the handoff as `kandev task handoff`, a subcommand of the existing Office
  CLI `task` group, using the same credential and transport path as its
  sibling subcommands. No new transport, client, or credential path shall be
  introduced.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001.2:** No MCP tool shall back
  this capability on any surface. The withdrawal is complete, not partial:
  every MCP tool, handler, capability value, and system-prompt reference for
  the withdrawn mechanism shall be removed, together with every call site that
  threaded the withdrawn permission check. A predicate needed by the new route
  and a compare-and-set primitive that is transport-independent are not part
  of the withdrawn surface and shall survive unchanged.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001.3:** The command shall POST
  to a new authenticated Office runtime route,
  `POST /api/v1/office/runtime/handoffs`, registered alongside the existing
  task-creation runtime route. The route shall derive every identity field
  (caller, workspace, agent, role, capability) from the signed run token and
  shall not accept, read, or honour any such field from the request body,
  path, or query string.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001.4:** The route shall decode
  its body with a closed-JSON decoder that rejects unknown fields and a body
  carrying more than one JSON value. A field outside the accepted set shall be
  refused with HTTP 400 naming the offending field, before any write.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001.5:** The Office system
  prompt shall not mention a handoff MCP tool, and shall again state,
  unqualified, that Office state changes go through the CLI and that no
  further Kandev MCP tools are to be sought. Existing tests asserting the
  Office MCP tool inventory shall pass unchanged, with no new "capability
  granted" assertion, because the granted and ungranted MCP inventories are
  now identical.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001.6:** Discoverability shall
  be per role and shall live in the role's own instructions, not in a tool
  registry. Only the CEO role's instructions shall document the command, its
  required flags, and the fact that it targets a different workspace. Because
  discovery is advisory and not a boundary, an agent whose instructions omit
  the command is refused by the authorization requirement, not by its
  ignorance of the command.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001.7:** The command shall
  accept exactly these flags, each mapping one-to-one onto a request-body JSON
  field, and shall reject an unknown flag and any positional argument:

  | Flag | JSON field | Required | Notes |
  |---|---|---|---|
  | `--target-workspace-id` | `target_workspace_id` | yes | must differ from the caller's workspace |
  | `--workflow-id` | `workflow_id` | yes | must belong to the target workspace |
  | `--title` | `title` | yes | 60 runes or fewer |
  | `--prompt` | `prompt` | yes | the delivery agent's first user message |
  | `--agent-profile-id` | `agent_profile_id` | yes | see profile-resolution requirements |
  | `--executor-profile-id` | `executor_profile_id` | yes | see profile-resolution requirements |
  | `--repository-id` | `repository_id` | no | must already exist in the target workspace |
  | `--base-branch` | `base_branch` | no | only meaningful with `--repository-id` |
  | `--start-agent` | `start_agent` | no | boolean, default false |
  | `--external-id` | `external_id` | no | create-idempotency key |

  A required field that is absent, empty, or whitespace-only shall be
  rejected with HTTP 400 naming it, before any write.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001.8:** An optional string
  argument that is present but empty or whitespace-only shall be rejected
  with HTTP 400 naming it, rather than treated as absent. `base_branch`
  supplied without `repository_id` shall be refused with HTTP 400 naming
  `base_branch`. `repository_id` supplied without `base_branch` shall use
  that repository's default branch. `repository_id`, when supplied and
  valid, shall attach exactly one repository to the delivery task; when
  omitted, the delivery task shall have no repositories and shall not
  inherit one from the source task.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001.9:** Every criterion that
  governs the shape or content of a request shall be enforced at the route,
  not only in the command, because the command is not a trust boundary — an
  agent can reach the route directly. The command may refuse an obviously
  malformed invocation locally for a faster message, but each such refusal
  shall also hold when the route is called directly with the equivalent
  body. A criterion is exempt from route-level enforcement only when its
  subject is not a request at all — the subcommand's own existence and
  dispatch, a documentation or help-text obligation (the system prompt
  advertising no tool, a role's instructions documenting the command, or a
  flag's help text describing a behaviour), or the command's flag-parsing
  behaviour distinguishing a flag that was not supplied from one supplied
  empty. Each such criterion is verified at its own surface — a route test
  cannot observe help text or instruction content — and no criterion that
  constrains what the request or its response must be is exempt on any other
  ground.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001.10:** Refusals shall use the
  runtime surface's existing error contract: HTTP 400 with a message naming
  the offending field; HTTP 403 with a message; HTTP 500 with a fixed generic
  message that names no resource, except the one case in criterion .12. No new
  error envelope shall be introduced, and the command shall surface the
  route's error message verbatim.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001.11:** The command shall
  distinguish a flag that was not supplied from one supplied with an empty
  value, and shall include a supplied-but-empty flag in the request body so
  the route can refuse it under criterion .8. The command shall not silently
  drop a blank optional value from the payload.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001.12:** A `title` longer than
  60 runes shall be rejected with HTTP 400 stating both the limit and the
  actual rune length, without truncation. `prompt` shall carry no length
  limit imposed by this action; the transport's own message-size limit is the
  only bound, and exceeding it is a transport error, not a validation
  failure.

## Out of scope

- An MCP tool for this capability. Excluded permanently, not deferred:
  reintroducing one would reopen the system-prompt contradiction this
  requirement closes.
- A `workflow_step_id` argument; destination-step resolution is server-side
  (see profile-resolution requirements).
- `blocked_by`, `parent_id`, `workspace_mode`, `autopilot`, `repository_url`,
  `local_path`, and `priority`. Cross-workspace dependencies and parents have
  no defined meaning here; repository-by-URL or by-local-path create a
  repository row as a side effect, a second boundary crossing in one call;
  `priority` has no named consumer on this path and the same-workspace create
  path already rejects it.
