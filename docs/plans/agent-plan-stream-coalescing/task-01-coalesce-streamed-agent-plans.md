---
id: "01-coalesce-streamed-agent-plans"
title: "Coalesce streamed agent plans"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-PLAN-STREAM-COALESCING-001
acceptance_criteria:
  - AC-AGENTS-AGENT-PLAN-STREAM-COALESCING-001.1
  - AC-AGENTS-AGENT-PLAN-STREAM-COALESCING-001.2
  - AC-AGENTS-AGENT-PLAN-STREAM-COALESCING-001.3
  - AC-AGENTS-AGENT-PLAN-STREAM-COALESCING-001.4
  - AC-AGENTS-AGENT-PLAN-STREAM-COALESCING-001.5
system_design:
  - ../../specs/agents/system-design/agent-plan-stream-coalescing.md
---

# Task 01: Coalesce streamed agent plans

## Summary

Correlate ACP plan snapshots to their source tool call, persist one updating
`agent_plan` message per invocation, and collapse conservative legacy snapshot
chains during transcript projection.

## In scope

- Propagate source `ToolCallID` on ACP agent-plan events.
- Add a task-service agent-plan upsert keyed by session, turn, and source tool
  call, and route correlated orchestrator events through it.
- Preserve append behavior for uncorrelated plan events.
- Normalize correlated and qualifying legacy snapshots before message grouping.
- Add red-first backend and frontend tests and one replay-focused Chromium
  Playwright regression.

## Out of scope

- Database migrations or historical-row rewrites.
- Task-plan document history or safe-edit behavior.
- Plan-card markup, localization, layout, responsive branches, controls,
  scrolling, or touch behavior.
- Deduplicating other message types.

## Acceptance

- Repeated snapshots from one source tool call create one durable plan message
  whose content is the latest accepted snapshot; retries remain idempotent and
  separate tool calls remain separate.
- Live, replayed, and paginated transcripts project correlated plans once and
  fold only adjacent same-turn strict-prefix legacy chains.
- Focused browser evidence displays one latest plan card from persisted
  duplicates without changing desktop or phone composition.

## ASCII UI preview

`UI-01: Streamed agent plan transcript` from the
[combined plan preview](plan.md#ascii-ui-preview):

```text
Desktop and phone task conversation
+ Agent plan: Batch ES reads       [collapsed]
  latest complete snapshot
```

Card controls and the existing single transcript scroll owner are unchanged.

## Verification

```bash
cd apps/backend
go test ./internal/agentctl/server/adapter/transport/acp -run 'AgentPlan'
go test ./internal/task/service ./internal/orchestrator -run 'AgentPlan'
go test ./internal/task/repository/sqlite -run 'AgentPlan'
cd ../
pnpm install --frozen-lockfile
pnpm --filter @kandev/web test -- hooks/processed-message-filtering.test.ts hooks/use-processed-messages.test.ts
pnpm --filter @kandev/web run typecheck
cd web
pnpm exec eslint hooks/processed-message-filtering.ts hooks/processed-message-filtering.test.ts hooks/use-processed-messages.ts hooks/use-processed-messages.test.ts e2e/tests/chat/agent-plan-coalescing.spec.ts
pnpm e2e:run --host --project chromium tests/chat/agent-plan-coalescing.spec.ts -- --retries=0
cd ../..
python3 scripts/list-docs.py decisions --format paths
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py validate
git diff --check
```

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tools.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tools_test.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming_test.go`
- `apps/backend/internal/task/service/service_messages.go`
- `apps/backend/internal/task/service/service_messages_test.go`
- `apps/web/hooks/processed-message-filtering.ts`
- `apps/web/hooks/processed-message-filtering.test.ts`
- `apps/web/hooks/use-processed-messages.ts`
- `apps/web/hooks/use-processed-messages.test.ts`
- `apps/web/e2e/tests/chat/agent-plan-coalescing.spec.ts`

## Dependencies

None.

## Risks

- Tool-call IDs are not globally unique; session and turn scope must remain part
  of the durable identity.
- The first snapshot and a retry can arrive close together; creation and update
  cannot be separate non-idempotent operations.
- The legacy compatibility rule must fail closed at any ambiguity.

## Parallelism

`sequential`

## Inputs

- `REQ-AGENTS-AGENT-PLAN-STREAM-COALESCING-001` and all acceptance criteria.
- The agent-plan stream system design and
  `ADR-2026-09-17-coalesce-streamed-agent-plans`.
- Existing tool-call message update patterns and transcript visibility filters.

## Results

- RED: the ACP adapter emitted plan snapshots with an empty tool-call identity,
  the orchestrator appended a new session message, and transcript filtering
  retained both correlated and legacy cumulative snapshots.
- GREEN: ACP plan events now carry the emitted source tool-call ID. The
  orchestrator routes correlated plans to a deterministic task-service upsert,
  while uncorrelated adapter events retain append compatibility.
- GREEN: the repository serializes each correlated plan's read/create/update
  and conversation receipt in one identity-locked transaction. Identical
  snapshots skip both the write and update event.
- GREEN: transcript filtering keeps the final delivery for each durable
  correlation and folds only adjacent same-turn strict-prefix legacy chains.
- Focused backend tests passed in the ACP adapter, task service, and
  orchestrator; the task-service and orchestrator cases also passed with
  `-race`. Backend application and integration message adapters compiled.
- Focused frontend tests passed: 87 tests across the filtering and processed
  message suites. TypeScript typecheck and changed-file ESLint passed.
- The production-build Chromium replay scenario passed: 1 test confirmed one
  latest plan card before and after page reload.
