---
id: "02-status-mutation-client"
title: "Add the Office agent status mutation client"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-AGENT-RECOVERY-001
  - REQ-OFFICE-AGENT-RECOVERY-002
acceptance_criteria:
  - AC-OFFICE-AGENT-RECOVERY-001.3
  - AC-OFFICE-AGENT-RECOVERY-001.8
  - AC-OFFICE-AGENT-RECOVERY-002.5
system_design:
  - ../../specs/office/system-design/agent-recovery.md
---

# Task 02: Add the Office agent status mutation client

## Summary

Add the web API client function for `PATCH /api/v1/office/agents/:id/status`,
the endpoint that validates the transition. The web tree has no caller today,
and the general agent update client cannot serve because its payload builder
never emits `status`.

## In scope

- `updateAgentStatus(id, status, options?)` in the Office API domain client,
  posting `{ status }` and, for guarded recovery, an `expected_status`
  precondition; it returns the normalized agent from the response.
- Unit coverage over the request the function issues and the agent it returns.

## Out of scope

- Any UI, store, or locale change.
- Widening `agentPayload` or routing status through `updateAgentProfile`.
- Sending `pause_reason`; the recovery request omits it, which is what clears
  the stored value.

## Acceptance

- The function issues exactly one request, `PATCH` to
  `/api/v1/office/agents/<id>/status`, with the body `{"status":"idle"}` and no
  other state field, when called with the target `idle`. A guarded recovery
  may add its expected source status as a concurrency precondition.
- The target status is the caller's argument, never read from a prior response
  or from module state.
- The resolved value is the response body's agent passed through the module's
  existing normalization, so `status` and `pauseReason` come from the server.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web test -- --run lib/api/domains/office-agent-status-api.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
```

The install line applies only in a fresh worktree where `apps/node_modules/` is
absent.

## Files likely touched

- `apps/web/lib/api/domains/office-api.ts`
- `apps/web/lib/api/domains/office-agent-status-api.test.ts`

## Dependencies

None.

## Risks

- `updateAgentProfile` is the obvious-looking reuse and is wrong here: the
  general `PATCH /agents/:id` endpoint assigns `status` without consulting the
  transition table, so an illegal transition would be written rather than
  refused. The test must assert the URL, not just the outcome.
- The endpoint answers a missing agent and an illegal transition with the same
  `400`. The client must not try to distinguish them.

## Parallelism

`parallel-safe`

Disjoint from Task 01: web-only files against no shared schema, migration,
generated contract, lockfile, or package configuration.

## Inputs

- `docs/specs/office/system-design/agent-recovery.md`, "Data and contracts".
- `apps/web/lib/api/domains/office-api.ts`, `normalizeAgent` and
  `updateAgentProfile` for the module's existing request shape.
- `apps/web/lib/api/domains/agent-update-api.test.ts` for the fetch-assertion
  pattern used by sibling domain clients.

## Results

Implemented in `office-api.ts` and `office-agent-status-api.test.ts`. The
client sends the constant target and can include the rendered status as the
`expected_status` recovery precondition. The response still passes through
the existing agent normalizer.
