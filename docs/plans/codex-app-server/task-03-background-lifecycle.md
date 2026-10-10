---
id: "03-background-lifecycle"
title: "Child and background lifecycle"
status: done
wave: 3
depends_on:
  - "02-native-conversations"
plan: "plan.md"
requirements:
  - REQ-AGENTS-CODEX-NATIVE-002
  - REQ-AGENTS-CODEX-NATIVE-003
  - REQ-AGENTS-CODEX-NATIVE-006
acceptance_criteria:
  - AC-AGENTS-CODEX-NATIVE-002.3
  - AC-AGENTS-CODEX-NATIVE-003.1
  - AC-AGENTS-CODEX-NATIVE-003.2
  - AC-AGENTS-CODEX-NATIVE-003.3
  - AC-AGENTS-CODEX-NATIVE-003.4
  - AC-AGENTS-CODEX-NATIVE-006.1
  - AC-AGENTS-CODEX-NATIVE-006.2
  - AC-AGENTS-CODEX-NATIVE-006.3
system_design:
  - ../../specs/agents/system-design/codex-app-server.md
---

# Task 03: Child and background lifecycle

## Summary and scope

Persist native thread/item bindings, normalize child activity and background commands, and render available history.
Keep the event stream alive after the parent reply and reconcile it on reconnect.
Extend existing subagent persistence without creating Kandev tasks for native children.

## Exclusions

Do not create a generic distributed task scheduler or promise child stop controls absent from the tested protocol.

## Likely files and ownership

- Native adapter child, item, and lifecycle handlers; `agentctl/types/streams/agent.go`.
- `internal/orchestrator/` normalized event handling and child persistence.
- `internal/task/models/`, `internal/task/repository/sqlite/` thread-binding migrations and tests.
- Shared web message/state types, WS handlers, `tool-subagent-message.tsx`, child history and background summary.
- Fake-server child/background/reconnect scenarios and UI-02 E2E files.

## Acceptance

1. Duplicate spawn/activity events produce one logical child; early or resumed events preserve source-turn ownership.
2. A child can remain active and report output after root completion. Foreground cancel, full stop, and disconnected unknown state remain distinct.
3. Desktop and phone expose available child history and background commands with the required single-scroll presentation.

## ASCII UI preview

UI-02 from [the plan](plan.md#ui-02-native-activity):

```text
Desktop: Background work: 1 agent, 1 command [View]
         Explore parser   Running           [Open]
Phone:   [Background work: 2 >]
         [Back] Explore parser
         | child history (one scroll body) |
```

Phone child history is a full-height surface, not a nested desktop pane.
Root completion leaves running rows visible. Reconnect initially shows unknown where evidence is missing.

## TDD and verification

Create `TestChildOutlivesParent`, `TestChildReconnect`, and `TestCancellationScope` before implementation.
Add persistence coverage for stable identity across execution replacement and out-of-order child discovery.
Run from the repository root:

```bash
(cd apps/backend && go test ./internal/agentctl/server/adapter/transport/codexappserver/... ./internal/orchestrator/... ./internal/task/repository/sqlite/...)
(cd apps/backend && go test -race ./internal/agentctl/server/adapter/transport/codexappserver/...)
(cd apps/web && pnpm test -- components/task/chat/messages/tool-subagent-message.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/subagent.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-subagent.spec.ts)
git diff --check
```

## Risks

A child thread can emit events before its spawn row.
Process exit and parent completion are not equivalent evidence of child completion.

## Results

Native fake-server tests passed for child output after parent completion, child binding restoration from thread history, and background-terminal completion after disappearance. The transport race tests passed. The desktop and mobile subagent E2E tests passed for the shared child card. Those tests use the mock agent, not the Codex app server.

There is no native child-to-browser E2E scenario yet. Real child/background event compatibility remains part of Task 07.
