---
id: "08-session-runtime-streams"
title: "Session runtime streams"
status: done
wave: 3
depends_on: ["03-query-options-taxonomy", "04-query-bridge-audit"]
plan: "plan.md"
spec: "../../specs/ui/tanstack-query-server-state.md"
---

# Task 08: Session Runtime Streams

## Acceptance

- Git status, commits, prepare progress, context window, available commands,
  session mode, models, capabilities, prompt usage, todos, and agentctl status
  use TanStack Query where they are server-owned.
- High-frequency shell/process/terminal output uses bounded stream buffers or
  retained terminal transport state, not per-chunk query cache writes.
- Bridge skipped actions document every stream/control-plane exception.

## Verification

- `cd apps/web && rtk pnpm typecheck` passed.
- `cd apps && rtk pnpm --filter @kandev/web test -- hooks/domains/session components/session components/task lib/query`
  passed 153 files / 1264 tests / 4 skipped.
- `cd apps/web && rtk pnpm e2e:docker tests/terminal/terminal-hanging-on-boot.spec.ts tests/terminal/terminal-dockview-ui.spec.ts tests/git/git-changes-panel.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 30 desktop Docker tests.
- `cd apps/web && rtk e2e/scripts/run-e2e.sh --docker --no-build --project mobile-chrome -- tests/terminal/mobile-terminal-keybar.spec.ts tests/terminal/mobile-terminal-scroll.spec.ts`
  passed 16 mobile Docker tests.

No failure artifacts were produced.

## Files Likely Touched

- `apps/web/hooks/domains/session/use-session-git*.ts`
- `apps/web/hooks/domains/session/use-terminals*.ts`
- `apps/web/hooks/domains/session/use-prepare-summary.ts`
- `apps/web/lib/query/query-options/session-runtime.ts`
- `apps/web/lib/query/bridge/session-runtime.ts`
- `apps/web/lib/query/bridge/session-runtime-streams.ts`
- `apps/web/lib/query/streams/ring.ts`
- `apps/web/lib/ws/handlers/{git-status,executor-prepare,executors,executor-profiles,session-mode,session-poll-mode,session-models,agent-capabilities,prompt-usage,session-todos,terminals}.ts`
- `apps/web/lib/state/slices/session-runtime/*`

## Dependencies

- Tasks 03 and 04.

## Inputs

- Old PR session-runtime query/bridge/stream files.
- Current terminal/mobile tests and stream performance constraints.

## Output Contract

Done.

## Implementation Notes

Server-owned runtime state now has runtime query keys/options, initial query
seeding, and WS bridge coverage for:

- git status and commit snapshots
- prepare progress
- session context-window metadata
- available commands
- session mode and poll mode
- agent capabilities
- models and current model metadata
- prompt usage
- session todos
- agentctl readiness/error state
- low-frequency process status

The migrated readers use TanStack Query first and keep Zustand as a compatibility
mirror until Task 10 removes legacy server-state slices.

## Query Exceptions

The following streams intentionally stay out of TanStack Query:

- `terminal.output`: terminal renderer/buffer transport, high-volume per-chunk
  stream.
- `session.shell.output`: bounded shell-output buffer, high-volume per-chunk
  stream.
- `session.process.output`: bounded process-output buffer, high-volume per-chunk
  stream.

The following request/control-plane actions remain outside query-cache mutation:

- `session.git.commits`, `session.commit_diff`, and `session.cumulative_diff`
  are explicit request/response fetch paths.
- `user_shell.*`, terminal input/subscription actions, and shell/process
  subscribe/input actions drive terminal lifecycle or transport state.
- `session.subscribe`, `session.unsubscribe`, `session.focus`, agent request
  actions, queue request actions, task/run subscribe actions, and permission/input
  effects are not durable server-state notifications.

Retained Zustand/client-only paths are limited to compatibility mirrors,
aggregate sidebar reads that Task 10 will remove, local terminal UI buffers, and
local transport/lifecycle state.
