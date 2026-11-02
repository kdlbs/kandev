---
id: "08-claude-lifecycle"
title: "Settle and resume Claude subagents"
status: done
wave: 3
depends_on:
  - "05-control-labels"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SUBAGENT-LIFECYCLE-003
acceptance_criteria:
  - AC-AGENTS-SUBAGENT-LIFECYCLE-003.1
  - AC-AGENTS-SUBAGENT-LIFECYCLE-003.2
  - AC-AGENTS-SUBAGENT-LIFECYCLE-003.3
  - AC-AGENTS-SUBAGENT-LIFECYCLE-003.4
  - AC-AGENTS-SUBAGENT-LIFECYCLE-003.5
  - AC-AGENTS-SUBAGENT-LIFECYCLE-003.6
system_design:
  - ../../specs/agents/system-design/subagent-provider-lifecycle.md
---

# Task 08: Settle and resume Claude subagents

## Summary

Mark Claude background cards completed at a normal prompt end, add the final report when it can be read, and mark cards resumed when `SendMessage` names their child. Track foreground cards with reported agent IDs for resume, but settle them only after they are resumed.

## In scope

- `adapter_claude_subagents.go`, settlement in `adapter_prompt.go`, and bounded report reading.
- Index any Claude subagent card with an `AgentID` so foreground cards can be resumed.
- Preserve a prior result when no new non-empty report is available.
- Preserve parent links on settlement and resume presentation updates.
- `ResumeCount` and the resumed chip in seven locales.
- Mock-agent resumed flow and E2E.

## Out of scope

- Native `subagent_spawned` frames.
- Per-child finish inference from `task-notification`.

## Acceptance

- A handed-off or cancelled prompt leaves cards unchanged, and another prompt's completion does not touch them.
- An unreadable or malformed report completes the card with no result.
- Session background counts are identical with and without these updates.

## ASCII UI preview

UI-03: Subagent card chips. See the [full preview](plan.md#ascii-ui-preview).

```text
[>] general-purpose  Read a.txt   background  resumed x1  sonnet
    Result: The command output was: apple
    +- Read b.txt
```

## Verification

```bash
(cd apps/backend && go test -trimpath -race -count=1 ./internal/agentctl/server/adapter/transport/acp/... ./internal/agentctl/types/streams/...)
(cd apps/backend && go test -trimpath -count=1 ./internal/orchestrator/... ./cmd/mock-agent/)
(cd apps/web && pnpm exec vitest run components/task/chat/messages/subagent-meta.test.ts components/task/chat/messages/tool-subagent-message.test.tsx && pnpm run i18n:check)
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/chat/subagent.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/chat/mobile-subagent.spec.ts
bash scripts/lint-go-changed <changed-go-files>
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_claude_subagents.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tools.go`
- `apps/backend/internal/agentctl/types/streams/tool_payload.go`
- `apps/backend/cmd/mock-agent/scenarios.go`
- `apps/web/components/task/chat/messages/subagent-meta.ts`
- `apps/web/components/task/chat/types.ts`
- `apps/web/src/locales/*/task.json`

## Dependencies

Task 05

## Risks

- A resumed card's status change passes through a payload with background-work identity; it must not reopen liveness.
- Settlement relies on provider end-of-prompt behavior observed in three captures.

## Parallelism

`sequential`

## Inputs

- [System design](../../specs/agents/system-design/subagent-provider-lifecycle.md)
- `apps/backend/internal/agentctl/AGENTS.md`, `apps/web/AGENTS.md`
- Captures in `/tmp/acp-pr7/`, or a recapture with `acpdbg`

## Results

Complete. The ACP/stream race suite and orchestrator/mock-agent tests passed.
Regression tests cover settlement and resume after background completion,
inactive-turn running payloads, captured foreground resume frames, prior-result
preservation, and parent links. Focused subagent unit tests, web typecheck and lint,
locale checks, desktop and phone subagent E2E, changed-file Go lint, docs/spec
validation, the backend Go build, and production Vite build passed. Task 04 remains
blocked on atomic notice and inbox-cursor persistence; notices are not advertised
or handled.

Settled and resumed Claude subagent card updates no longer record or publish a new
running background-work observation; the shared `isSubagentPresentationUpdate` check
guards both the background accounting and observation paths, with a regression test
covering the async launch and both presentation updates.
