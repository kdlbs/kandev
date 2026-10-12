---
status: active
system: cross-workspace-task-handoff
created: 2026-09-02
updated: 2026-09-15
owners:
  - nova28
---

# Authorization and target-workspace resolution Requirements

## Overview

Two gates protect the handoff, and both are load-bearing: an execution gate
(a per-agent-or-role permission, re-derived from live state on every call) and
an ownership gate (the target workspace must belong to the caller's own
owner). Role instructions are advisory discoverability, never a third gate.
Every target-resource lookup this action performs must distinguish "resolved
to nothing" (a caller mistake) from "failed to execute" (a transient backend
failure), because folding the second into the first tells an automated caller
to give up on a call it should retry, and running any resource lookup before
both gates would let an unauthorized agent distinguish a real resource id
from a fabricated one.

### REQ-CROSS-WORKSPACE-TASK-HANDOFF-AUTHORIZATION-001: Permission, capability, and target-workspace gates

#### Acceptance criteria

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-AUTHORIZATION-001.1:** The system shall
  define an Office permission key granting the handoff, defaulting to granted
  for the CEO role and denied for every other role, including any unknown
  role. The key shall appear, at the same index, in both lists that back the
  agent permission editor, so the editor's own consistency test continues to
  pass; its label and description are fixed user-visible English copy, are
  not present in any locale catalog today, and are not translated by this
  requirement.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-AUTHORIZATION-001.2:** The system shall
  define a runtime capability derived from that permission by the same
  resolution every other permission-derived runtime capability uses, so role
  defaults merged with per-agent overrides remain the single source of the
  grant. The withdrawn MCP mechanism's capability value (see the command and
  route surface requirements) is replaced outright, not paralleled: no MCP
  surface shall be able to gate, advertise, or reach this feature.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-AUTHORIZATION-001.3:** A run's signed
  capability snapshot shall carry the handoff capability when, and only when,
  it was granted at the moment the run token was minted. That snapshot is not
  the authorization source for this one capability — see criterion .4 — so it
  records only what was granted when the run began.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-AUTHORIZATION-001.4:** The route shall
  authorize from the trusted run token and from the agent's current
  permissions, and from nothing else. For the handoff capability
  specifically, the value re-derived from the agent's live permissions at
  request time shall take precedence over the signed snapshot, so a grant
  revoked after the token was minted refuses immediately rather than being
  honoured for the remainder of the token's life, and a grant added after
  minting is honoured immediately rather than waiting for a new token. The
  route shall refuse with HTTP 403 when the resolved value is false, and no
  write shall occur, regardless of how the calling agent learned the command
  exists.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-AUTHORIZATION-001.5:** A request with no
  bearer token, an invalid token, or a token naming an agent that cannot be
  loaded shall be refused with HTTP 401, and no write shall occur. Neither
  refusal shall fall through to an unscoped internal caller.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-AUTHORIZATION-001.6:** An unauthorized
  call shall return a message naming the missing permission and stating that
  it is granted per agent or per role. It shall not disclose whether
  `target_workspace_id` exists.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-AUTHORIZATION-001.7:** The target
  workspace shall be authorized by the platform's existing per-user
  workspace scoping before any write. A workspace whose owner is not the
  source task's owner shall be refused indistinguishably from a workspace
  that does not exist. This check shall not be bypassed, duplicated, or
  reimplemented.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-AUTHORIZATION-001.8:** `workflow_id`
  shall be validated to belong to `target_workspace_id`. A workflow that does
  not exist and a workflow that exists in another workspace shall both be
  refused with HTTP 400 and the same message, naming only `workflow_id` and
  stating that it is not a workflow of the target workspace. The message
  shall not name, echo, or otherwise disclose which workspace the workflow
  actually belongs to, and the two cases shall not be distinguishable by
  message, status code, or timing.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-AUTHORIZATION-001.9:** `workflow_id`
  shall additionally be refused with HTTP 400 when it equals the target
  workspace's own office workflow, with a message stating that the target
  must be a delivery workflow, not the workspace's office workflow. A
  workspace with no configured office workflow has nothing to compare against
  and shall not be refused on this ground.

- **AC-CROSS-WORKSPACE-TASK-HANDOFF-AUTHORIZATION-001.10:** Every
  target-resource check this action performs — the target workspace,
  `workflow_id`, the agent and executor profiles, destination-step
  resolution, and `repository_id` — shall distinguish absence from failure: a
  read that executes and finds nothing, or finds a row belonging to another
  workspace, is HTTP 400; a read that fails to execute is HTTP 500 and shall
  be safe to retry. In both cases no write shall occur, and a transient
  backend failure shall never be reported as if it were a validation refusal.
  The target-workspace-ownership refusal's failure branch shall likewise
  disclose no existence information.

## Rationale

The two gates are independently necessary: the capability gate alone would
let an agent act in a workspace its owner cannot see, and the ownership gate
alone would let any Office agent hand off inside its own owner's estate. This
requirement's authorization and resolution order — authentication, then the
capability gate, then target-workspace ownership, then the target-resource
checks — is a total order enforced across this requirement and the
profile-resolution and same-workspace-refusal requirements; see the handoff
mechanism system design for the exact sequence and the reasoning tying it
together.

## Out of scope

A dedicated settings toggle for the runtime capability. The runtime
capability is derived from the Office permission and is not separately
editable; runtime capabilities have no operator surface today, and adding one
for a single key would create a second place to grant the same thing.
