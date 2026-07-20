---
id: "06-public-docs"
title: "Public stop documentation"
status: done
wave: 5
depends_on: ["03-mcp-stop-handler", "04-task-mcp-tool"]
plan: "plan.md"
spec: "../../specs/tasks/parent-child-task-stop.md"
---

# Task 06: Public stop documentation

## Acceptance

- Public task MCP and coordination docs recommend interrupt for stop-and-steer,
  reserve stop for halt-only intent, and explain direct-child authority,
  all-session scope, eligible active-Kanban `REVIEW`, and async teardown.
- Internal `mcp.stop_task` appears in the WebSocket transport catalog, which
  clearly says raw `/ws` rejects every `mcp.*` action and supported access is
  through MCP adapters. `stop_task_kandev` is assigned exactly once in public
  coverage metadata, with its backend handler and gateway guard source/tests
  mapped to the same area.
- Public docs validators pass; External MCP feature-status text explicitly says
  stop and other live-session tools are omitted.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files Likely Touched

- `docs/public/automation-and-mcp.md`
- `docs/public/coordination.md`
- `docs/public/sessions-and-review.md`
- `docs/public/feature-status.md`
- `docs/public/websocket-api.md`
- `docs/public/coverage.json`

## Dependencies

- Tasks 03-04 fix the final action, tool, permission, and response contracts.

## Inputs

- Spec: complete public behavior contract.
- Existing task MCP sections and `task-lifecycle` coverage area.
- Docs-maintainer validation workflow.

## Output Contract

Update this task to `done`, update `plan.md`, and report docs changed,
validation commands, blockers, and any intentionally undocumented internals.

## Completion

- Documented the queued / interrupt-and-steer / halt-only choice, direct-child
  authority, all-session scope, guarded `REVIEW`, idempotency, async teardown,
  and preserved resources across the public MCP, coordination, and session docs.
- Documented raw `/ws` rejection for every `mcp.*` action and External MCP's
  omission of `stop_task_kandev`.
- Assigned `stop_task_kandev` exactly once to `task-lifecycle` coverage with the
  handler and gateway sources/tests.
- Both public-doc validators pass; no internal implementation detail beyond the
  durable behavior and transport boundary was added to public guidance.
