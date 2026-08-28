---
id: "01-live-permission-contract"
title: "Live permission contract"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/external-permission-resolution.md"
---

# Task 01: Live permission contract

## Acceptance

- Every manual agent permission request has a Kandev `request_id` distinct from its provider
  `pending_id`, and list snapshots are immutable, deterministic, bounded, and safely projected.
- Strict resolution atomically validates request ID, pending ID, state, and exact offered option;
  stale/replaced, unknown-option, concurrent, duplicate, and replay attempts send nothing.
- Agentctl exposes typed list/resolve stream actions and the runtime client preserves stable error
  codes without exposing raw action details or secret canaries.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/common/securityutil ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl
```

## Files likely touched

- `apps/backend/internal/common/securityutil/permission.go`
- `apps/backend/internal/common/securityutil/permission_test.go`
- `apps/backend/internal/agentctl/types/streams/agent.go`
- `apps/backend/internal/agentctl/types/streams/permission.go`
- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/manager_permission_test.go`
- `apps/backend/internal/agentctl/server/api/agent.go`
- `apps/backend/internal/agentctl/server/api/agent_test.go`
- `apps/backend/internal/agent/runtime/agentctl/agent.go`
- `apps/backend/internal/agent/runtime/agentctl/agent_test.go`

## Dependencies

None.

## Parallelism

Sequential. It defines the live authority and wire types consumed by every later task.

## Inputs

- Spec: `What`, `Data model > Live pending request`, `API surface`, `State machine`, and redaction
  scenarios.
- Decision: live agentctl authority and dual request identities.
- Existing patterns: `process.Manager.pendingPermissions`, `agent.permissions.respond`, and
  `internal/common/securityutil`.

## Risks

- Sending on the response channel while holding the state mutex must not deadlock cancellation or
  cleanup.
- Provider option metadata must remain available to the internal adapter response while never
  entering the public snapshot.

## Output contract

Report request identity/state behavior, safe projection fields, exact commands/results, files
changed, blockers/risks, then update this task and the plan checkbox/results.

## Results

- Added Kandev UUID request generations, immutable safe snapshots, deterministic bounded listing,
  identity-aware replacement/cleanup, and bounded terminal tombstones in agentctl.
- Strict resolution validates the request/pending tuple and exact immutable option under one lock,
  delivers once, and preserves stable not-found/stale/replay/option/delivery codes through the
  agentctl WebSocket client.
- Permission presentation now allowlists action-type fields, removes environment, headers, MCP
  arguments and option metadata, and redacts credential-bearing text before stream/public use.
- Passed:
  `env GOCACHE=/tmp/kandev-go-build GOMODCACHE=/tmp/kandev-go-mod /home/bazil/.local/share/mise/installs/go/1.26.0/bin/go test -tags fts5 ./internal/common/securityutil ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl`.
