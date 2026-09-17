---
id: "07-input-resolution"
title: "Resolve native input from the assistant"
status: pending
wave: 5
depends_on: ["03-memory-context","05-tool-authority","06-attention"]
plan: "plan.md"
requirements:
  - REQ-ORCHESTRATION-ASSISTANT-006
acceptance_criteria:
  - AC-ORCHESTRATION-ASSISTANT-006.1
  - AC-ORCHESTRATION-ASSISTANT-006.2
  - AC-ORCHESTRATION-ASSISTANT-006.3
  - AC-ORCHESTRATION-ASSISTANT-006.4
system_design:
  - ../../specs/orchestration/system-design/personal-assistant.md
---

# Task 07: Resolve native input from the assistant

## Inputs

Read the [requirements](../../specs/orchestration/requirements/personal-assistant.md) and [design](../../specs/orchestration/system-design/personal-assistant.md); legacy scenarios S11, S12, and [plan](plan.md), Backend 6. Read applicable AGENTS.md and implementation skills before editing. The [baseline experiments](experiments.md) are continuation evidence, not completed implementation.

## Acceptance

1. Human central-chat actions resolve the original native question/permission with current task/session/request/revision checks and update both UIs' source state.
2. Assistant-provided known answers are attributed, scoped and only allowed for delegable questions; runtime cannot approve permissions or fabricate a human actor.
3. Duplicate, expired and timed-out responses preserve accurate receipts and do not start duplicate turns.

## Likely files

- apps/backend/internal/orchestration/runtime/attention_resolution.go, attention_resolution_test.go (new)
- apps/backend/internal/backendapp/adapters_assistant_input.go, adapters_assistant_input_test.go (new)
- apps/backend/internal/clarification/handlers.go, store.go (extract/reuse shared resolution service if needed)
- apps/backend/internal/orchestrator/executor/executor_interaction.go (existing permission response)
- apps/backend/internal/mcp/handlers/parent_question.go (reuse semantics where applicable, do not auto-enable autopilot)

## Implementation sequence

Factor the smallest reusable core resolution interface that retains native authorization/expiry. Add human and runtime endpoints with different capabilities. Route known-context answers through a durable operation tied to the question ID. A generic prompt must not stand in for resolution of a blocked native request. Keep expired requests expired after restart.

## Verification

Run each parenthesized command from the repository root. Use the repository Go/Node/pnpm toolchains. Scoped Go tests are intentional: the available make test target runs the entire backend. New test filters must select the named new tests; a no-tests-to-run result does not satisfy acceptance.

```sh
(cd apps/backend && go test -tags fts5 -count=1 ./internal/orchestration/runtime ./internal/backendapp -run 'TestAssistant(Input|Resolution|KnownAnswer)')
(cd apps/backend && go test -count=1 ./internal/clarification)
(cd apps/backend && go test -count=1 ./internal/mcp/handlers -run 'Test.*ParentQuestion')
```

## Dependencies and risks

Dependencies: `03-memory-context`, `05-tool-authority`, `06-attention`. Execute in the primary session unless the user explicitly authorizes subagents.

Permission response handles may exist only in a live provider process. Reconstituting a request from an attention record would invent authority; resolve current canonical state first.

## Output

A typed central resolution path with native gate parity and truthful delivery states.

## Detailed implementation checklist

1. Extract/reuse the smallest canonical clarification/permission resolution port
   without changing native task UI semantics. Resolve the live request by full
   task/session/request identity and revision; never reconstruct provider authority
   from an old durable attention row.
2. Add separate human and runtime operations. The human action rechecks owner,
   workspace visibility, native options, current request revision and human actor.
   Runtime may answer only explicitly delegable questions using scoped cited
   context; it cannot approve permissions/authentication or impersonate a click.
3. Bind each attempt to a stable operation key/payload digest. Use native CAS/
   response receipts to make duplicate submissions idempotent or conflicting.
   On timeout record unknown; query native state before deciding whether retry
   is safe. A queued chat message is not an answered native request.
4. Propagate the resulting source update to both attention and task views.
   Expired/restarted-away handles display expired and link to the native task;
   do not silently start another provider turn to manufacture a new request.
5. Implement explicit pause and stop-managed-work operations. Pause blocks new
   intake/dispatch/wakes without cancelling workers. Stop enumerates only currently
   managed authorized sessions, uses native cancellation and returns per-session
   stopped/already-finished/failed/unknown receipts. Neither action deletes history
   or claims to undo external changes.
6. Race human/task and central responses, grant revocation, worker completion and
   restart. Preserve existing core permission handling and attribution behavior;
   display partial cancellation rather than an all-stopped success on timeout.

## Detailed evidence map

| Criterion | Planned test | Required edge cases |
| --- | --- | --- |
| AC-ORCHESTRATION-ASSISTANT-006.1 | `TestAssistantInputHumanNativeParity` | Wrong session/owner/revision, actual options, task UI convergence |
| AC-ORCHESTRATION-ASSISTANT-006.2 | `TestAssistantKnownAnswerScope` | Cited known context, nondelegable question, permission/auth deny, honest actor |
| AC-ORCHESTRATION-ASSISTANT-006.3 | `TestAssistantResolutionIdempotencyAndExpiry` | Double submit, changed payload, lost reply, restart-expired handle |
| AC-ORCHESTRATION-ASSISTANT-006.4 | `TestAssistantInputPauseAndStop` | Pause leaves worker, stop only managed sessions, cancellation timeout, preserved history |

Use actual clarification store/service test fixtures plus a fake live permission
port. Expand the existing work-order filter with these names and run race tests
on runtime/backend adapters. Task 08/11 provides the human browser flow.

## Scope boundaries and delivery

Do not auto-enable autopilot, grant permissions by interpreting freeform chat,
or treat a memory preference as permission. This work order owns the backend
control semantics consumed by the assistant UI.

## Parallelism

`sequential`

## Results

Pending. Record red/green test evidence, exact commands and counts, relevant artifacts, owned changes and cleanup here; synchronize the plan checkbox only after acceptance is met.
