---
id: "01-codex-acp-bridge"
title: "Add the codex-acp guarded TTY bridge"
status: in_review
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-GUARDED-TTY-EXECUTION-001
  - REQ-AGENTS-GUARDED-TTY-EXECUTION-002
acceptance_criteria:
  - AC-AGENTS-GUARDED-TTY-EXECUTION-001.4
  - AC-AGENTS-GUARDED-TTY-EXECUTION-001.6
  - AC-AGENTS-GUARDED-TTY-EXECUTION-002.2
  - AC-AGENTS-GUARDED-TTY-EXECUTION-002.7
system_design:
  - ../../specs/agents/system-design/guarded-tty-execution.md
---

# Task 01: Add the codex-acp guarded TTY bridge

## Summary

Add the versioned ACP extension that performs one bounded App Server
`command/exec` with `tty: true` inside the active guarded session. This work is
owned by a separate `agentclientprotocol/codex-acp` task and PR.

## In scope

- Exact capability and execution extension methods.
- Active-session cwd/sandbox derivation and generated process identity.
- Matching output deltas, bounds, cancellation, and one receipt.
- Ordinary command and `unified_exec=false` coexistence tests.

## Out of scope

- Kandev backend, MCP, audit, or runtime code.
- Host command execution and interactive terminal controls.

## Acceptance

- The App Server request proves streaming and `tty: true` with no caller-owned
  security fields.
- Every terminal or failure path returns at most one bounded receipt.
- Existing model command execution and deployment launch configuration remain
  unchanged.

## Verification

```bash
npm test -- src/__tests__/CodexACPAgent/guarded-tty-exec.test.ts
npm run typecheck
npm run build
```

## Files likely touched

- `src/AcpExtensions.ts`
- `src/CodexAppServerClient.ts`
- `src/CodexAcpServer.ts`
- `src/__tests__/CodexACPAgent/guarded-tty-exec.test.ts`

## Dependencies

None.

## Risks

- App Server output notifications share a connection and require exact process
  correlation.

## Parallelism

`sequential`

## Inputs

- Guarded TTY requirement and system-design security sections.
- Deployment repair `6fcc88f` preservation contract.

## Results

Sibling task `5c9f515d-e5f9-43c5-bf31-fb42276e5e15` produced upstream commits
`22c17a2` and `1a5d8b9` in its dedicated `agentclientprotocol/codex-acp`
worktree and opened
[`agentclientprotocol/codex-acp#451`](https://github.com/agentclientprotocol/codex-acp/pull/451).
Its unit tests, typecheck, build, real App Server PTY probe, output bounds,
rejected-process termination, and ordinary prompt concurrency checks pass.

The reviewed contract advertises `guardedTtyExec`, probes
`_kandev/guarded_tty/capability`, and executes
`_kandev/guarded_tty/exec`. The execution request contains only `sessionId` and
`argv`; the receipt proves App Server method `command/exec`, requested and
dispatched TTY state, process identity, cwd, bounded stdout/stderr, timestamps,
completion, exit status, or a stable denial. The PR remains open and its forked
workflow runs require upstream repository approval, so no released package is
available yet.
