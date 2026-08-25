---
status: draft
system: cli
created: 2026-05-16
updated: 2026-08-16
owners:
  - cfl
---
# CLI-Mode Task Parity (Kanban) Requirements

## Overview

Anthropic has announced that **agent-SDK / `claude -p` usage will draw from a paid API budget**, while the interactive `claude` CLI continues to draw from the user's Pro/Max subscription quota. Today kandev drives Claude almost exclusively through the ACP bridge (`@agentclientprotocol/claude-agent-acp`), which is an SDK-mode integration. After the change, every kandev task run by a subscription user would burn API dollars they do not have.

## Requirements

### REQ-CLI-CLI-MODE-PARITY-001: CLI-Mode Task Parity (Kanban)

**Intent:** Anthropic has announced that **agent-SDK / `claude -p` usage will draw from a paid API budget**, while the interactive `claude` CLI continues to draw from the user's Pro/Max subscription quota. Today kandev drives Claude almost exclusively through the ACP bridge (`@agentclientprotocol/claude-agent-acp`), which is an SDK-mode integration. After the change, every kandev task run by a subscription user would burn API dollars they do not have.

#### Acceptance criteria

- **AC-CLI-CLI-MODE-PARITY-001.1:** GIVEN a Pi profile with `cli_passthrough: true` and a `pi` executable on the Kandev process `PATH` that passes `pi --version`
- **AC-CLI-CLI-MODE-PARITY-001.2:** WHEN Kandev starts its passthrough PTY
- **AC-CLI-CLI-MODE-PARITY-001.3:** THEN the command starts with `pi`
- **AC-CLI-CLI-MODE-PARITY-001.4:** AND Kandev does not launch `pi-acp` or `npx` as the passthrough process
- **AC-CLI-CLI-MODE-PARITY-001.5:** GIVEN a Pi profile with `cli_passthrough: false`
- **AC-CLI-CLI-MODE-PARITY-001.6:** WHEN Kandev starts structured chat or one-shot inference
- **AC-CLI-CLI-MODE-PARITY-001.7:** THEN the command remains `npx -y pi-acp`
- **AC-CLI-CLI-MODE-PARITY-001.8:** GIVEN Pi is unavailable on the Kandev host

## Migrated source detail

## Why

Anthropic has announced that **agent-SDK / `claude -p` usage will draw from a paid API budget**, while the interactive `claude` CLI continues to draw from the user's Pro/Max subscription quota. Today kandev drives Claude almost exclusively through the ACP bridge (`@agentclientprotocol/claude-agent-acp`), which is an SDK-mode integration. After the change, every kandev task run by a subscription user would burn API dollars they do not have.

Kandev already supports a per-profile **CLI Passthrough** mode that launches the agent CLI under a PTY (`apps/backend/internal/agentctl/server/process/interactive_runner.go`). Users who enable it keep their subscription billing — but the experience is bare:

1. The task-create dialog **disables the prompt textarea** when a CLI agent is selected (legacy assumption: "you'll type your prompt directly into the terminal"). So the user can't even attach a description to a CLI-mode task.
2. The task description that kandev would have sent through ACP is **not delivered to the CLI**. The user has to retype it.
3. The chat compose box on a CLI-mode session does nothing — there is no path from kandev's UI into the agent's stdin for follow-up messages.

This spec brings CLI-passthrough mode up to feature parity with ACP for the **kanban** task-execution surface. Office (autonomous) mode is explicitly deferred.

## What

### ACP and passthrough commands remain distinct

A built-in agent that uses a separate ACP adapter and interactive CLI keeps
those execution surfaces separate. Structured chat and one-shot inference use
the ACP adapter. CLI passthrough launches the interactive CLI directly under
the PTY; managed ACP runtime resolution does not replace that passthrough
command.

For Pi, structured chat and inference use `npx -y pi-acp`, while CLI
passthrough launches the globally installed `pi` executable. Kandev's Pi
install action runs
`npm install -g --ignore-scripts @earendil-works/pi-coding-agent`, and agent
discovery treats a `pi` executable on the Kandev process `PATH` that passes its
non-interactive `--version` check as the installation signal.

### Prompt allowed at task creation in CLI mode

The task-create dialog's prompt textarea is **enabled** when a CLI/passthrough-capable agent is selected. Users can write the prompt the same way they would for ACP. The control flow that today gates this off when `cli_passthrough` is true is removed.

### Task-prompt injection (idle-based, agent-agnostic)

When a passthrough session starts for a task that has a description:

1. Kandev launches the PTY as today.
2. After the existing **idle detector** in `InteractiveRunner` fires for the first time (i.e. the CLI has stopped emitting output and is presumed to be at its input prompt), kandev writes the task description plus a configurable **submit sequence** to the PTY's stdin when the agent has no `PromptFlag`.
3. Agents that use `PromptFlag` (headless one-shot mode) are unaffected — their prompt is already on the CLI before launch. `PassthroughConfig.AutoInjectPrompt` remains in discovered metadata for compatibility, but the presence of `PromptFlag` is the runtime delivery boundary.

No per-agent pattern matchers. The existing idle window is the only readiness signal. If an agent's CLI is unusual enough that an idle window misfires (writes a banner, then waits 5 seconds, then prompts), we make the idle window per-agent-configurable in `PassthroughConfig` (already exists as `IdleTimeout`). No new detection machinery.

For the Claude case: `claude_acp.go` retains `AutoInjectPrompt: true` as compatibility metadata, sets `DisableBracketedPaste: true` (Claude Code already enables bracketed-paste *mode* in its Ink TUI — injecting `ESC[200~`…`ESC[201~` delimiters breaks the prompt), and `SubmitViaBackslashEnter: true` (PTY writes: prompt, then `\`, then `\r` per [Claude terminal docs](https://code.claude.com/docs/en/terminal-config)). Ink may still treat programmatic Enter as newline only ([anthropics/claude-code#15553](https://github.com/anthropics/claude-code/issues/15553)) — if auto-submit fails, the user confirms with Enter. Other passthrough-capable agents without a `PromptFlag` use the same idle-based stdin route.

### Follow-up prompts via PTY stdin

The orchestrator's `PromptTask` handler branches on `IsPassthroughSession(sessionID)` and writes the prompt text + `SubmitSequence` to the agent's PTY stdin instead of sending over ACP. This is the same path used by auto-injection and is reachable today from any caller that hits `agent.prompt` for a passthrough session.

`PassthroughToolbar` provides the UI surface for this route. Its chat control opens `PassthroughComposerPanel`, which wraps the shared `ChatInputContainer` so passthrough follow-ups use the same rich input, context chips, attachment control, and plan-mode toggle as ACP task chat. Submissions go through `message.add` for the current passthrough session and are written to PTY stdin by the backend.

### Kandev MCP tools are wired for CLI mode

When a passthrough agent supports an MCP config flag, kandev generates a per-session MCP config file that points at the task's local agentctl MCP endpoint and launches the CLI with that config. The generated config exposes the same kandev tool server used by ACP sessions, scoped by the already-known `KANDEV_TASK_ID` / `KANDEV_SESSION_ID` context.

For the Claude case, the passthrough command includes `--mcp-config <generated-file>` with an `mcpServers.kandev` HTTP entry for the local agentctl `/mcp` endpoint.

### Stop sends Ctrl-C

The orchestrator's `CancelAgent` handler branches on `IsPassthroughSession(sessionID)` and writes `\x03` to the PTY's stdin instead of sending an ACP cancel. DB reconciliation still runs so the UI unsticks regardless of the write outcome.

Users can still press Ctrl-C directly inside the xterm terminal. A dedicated toolbar button that calls the same cancel route remains a follow-up.

## Scenarios

### Pi uses its interactive CLI in passthrough mode
- GIVEN a Pi profile with `cli_passthrough: true` and a `pi` executable on the
  Kandev process `PATH` that passes `pi --version`
- WHEN Kandev starts its passthrough PTY
- THEN the command starts with `pi`
- AND Kandev does not launch `pi-acp` or `npx` as the passthrough process

### Pi keeps its ACP adapter for structured execution
- GIVEN a Pi profile with `cli_passthrough: false`
- WHEN Kandev starts structured chat or one-shot inference
- THEN the command remains `npx -y pi-acp`

### Pi installation provisions the passthrough executable
- GIVEN Pi is unavailable on the Kandev host
- WHEN the operator runs the Pi install action and rescans agents
- THEN Kandev runs
  `npm install -g --ignore-scripts @earendil-works/pi-coding-agent`
- AND discovery finds the installed `pi` executable after its version check

### CLI-mode task can have a prompt
- GIVEN a Claude profile with `cli_passthrough: true`
- WHEN the user opens the task-create dialog and types in the prompt field
- THEN the prompt textarea accepts input (today it is disabled / hidden)

### Prompt injection on fresh start
- GIVEN a CLI-mode task whose agent has no `PromptFlag` and a non-empty description
- WHEN the task starts
- THEN the PTY launches, the idle detector fires once after the CLI settles, and the task description is written to stdin followed by `SubmitSequence`
- AND the description appears in the terminal output

### Resume does NOT re-inject
- GIVEN a CLI-mode task whose PTY exited (backend restart, crash) and is resumed via `--resume`
- WHEN the user reopens the terminal and the session resumes
- THEN no auto-injection occurs (the conversation is already in the agent's history)

### Fresh-start fallback DOES inject
- GIVEN a CLI-mode task whose resume launch fast-failed and the fresh-start fallback ran (existing `attemptResumeFallback` path)
- THEN auto-injection runs against the new fresh session — because that fallback is functionally a new conversation

### Follow-up prompt route is in place (UI surface follow-up)
- GIVEN a CLI-mode task running
- WHEN any caller invokes `PromptTask` for the session (today: future kandev compose surface, integration tests)
- THEN the text + `SubmitSequence` is written to the PTY
- AND no ACP prompt is sent

### Kandev MCP config is present for Claude passthrough
- GIVEN a Claude profile with `cli_passthrough: true`
- WHEN kandev starts or fresh-starts its passthrough command
- THEN the argv includes `--mcp-config <generated-file>`
- AND the generated file contains `mcpServers.kandev` pointing at the local agentctl `/mcp` endpoint

### Stop / cancel route is in place (UI surface follow-up)
- GIVEN a CLI-mode task running
- WHEN any caller invokes `CancelAgent` for the session
- THEN `\x03` is written to the PTY
- AND DB reconciliation still completes so the session unsticks

### Passthrough agent without PromptFlag
- GIVEN a passthrough-capable agent with no `PromptFlag`, regardless of its compatibility metadata
- WHEN a task starts
- THEN the task description is written to stdin after the first idle window

## Kandev toolbar above the passthrough terminal

### Where it appears

`PassthroughToolbar` (`apps/web/components/task/passthrough-toolbar.tsx`) wraps `PassthroughTerminal` with the same kandev controls that ACP sessions get from `ChatStatusBar` + `ChatInputArea`. It is mounted:

- Full-page task view: `dockview-shared.tsx` (`DockviewSharedContent`) renders it for passthrough sessions in place of the chat input area; the dockview desktop layout in `task-center-panel.tsx` uses the same shared content.
- Kanban Preview tab: `preview-session-tabs.tsx` renders `PassthroughToolbar` for passthrough sessions in the side-peek surface, so the preview and full-page task views share the same terminal, status row, and composer behavior.

### What it surfaces

The status row at the bottom of `PassthroughToolbar` contains, left to right:

- `PRStatusChip` — PR open/merged/draft indicator for the task.
- `PRMergedBanner` — banner shown when the PR is merged.
- `passthrough-proceed-next-step` button — moves the task to the next workflow step. Only shown when `nextStepName` is non-null and the agent is not busy (`sessionState` is not `RUNNING` or `STARTING`).
- `ChatToggleButton` — toggles `PassthroughComposerPanel` open and closed. Shows `variant="default"` when the composer is open or when there are pending review comments; shows a numeric chip (`data-testid="passthrough-pending-count"`) when collapsed with pending comments.

### Default focus contract

The terminal (`PassthroughTerminal`) renders above the status row and fills the remaining height. The compose box is collapsed by default so xterm retains keyboard focus for raw PTY interaction. The user opts in to the kandev composer explicitly via the Chat button.

Passthrough sessions have a dedicated configurable focus shortcut, `FOCUS_PASSTHROUGH_INPUT` (`Focus CLI Chat Input`, default `Cmd/Ctrl+Shift+Y`). It opens the composer and focuses the rich input when the user is not already typing in another input. The global ACP chat focus shortcut remains separate so pressing `/` while the terminal has focus continues to go to the PTY.

### Composer assistance

`PassthroughComposerPanel` delegates authoring to `ChatInputContainer` with passthrough-specific command behavior:

- Slash command suggestions are disabled (`hasAgentCommands={false}`) because passthrough sessions do not receive ACP-reported slash-command capabilities. A literal `/` typed in the composer remains text, and a `/` typed while the terminal has focus goes to the PTY.
- `@` file, prompt, plan, and task mentions use the shared rich-input lookup. Selected items render as the same context chips used by ACP chat.
- The top chip bar shows selected context items. The bottom toolbar provides plan mode, attachment upload, and context-item add controls.
- Uploaded attachments are sent through `message.add`; for passthrough sessions the backend materializes them under `.kandev/attachments/<session>/...` and appends their file paths to the PTY prompt.
- File and prompt context items are expanded into `<kandev-system>` context blocks before sending. Real filesystem-backed files are also sent as `context_files` metadata.

### How the composer formats and sends pending review comments

When `PassthroughComposerPanel` submits:

1. `usePendingDiffCommentsByFile(sessionId)` provides comments grouped by file path.
2. Non-empty pending diff comments are formatted via `formatReviewCommentsAsMarkdown` or the shared `buildSubmitMessage` path, then included with the typed text.
3. Selected context files, prompt mentions, and task mentions are appended as system context blocks. Real context files are included in `context_files`; uploaded attachments are included in `attachments`.
4. The combined payload is sent via `client.request("message.add", { task_id, session_id, content, attachments?, context_files? }, timeout)`.
5. `markCommentsSent(ids)` is called only on WS success — a failed send leaves both the composer open (typed text preserved for retry) and the comments pending.
6. A `PendingCommentsBanner` (`data-testid="passthrough-pending-comments-banner"`) renders inside the open composer when `pendingCount > 0`.

### How Stop maps to Ctrl-C

Pressing Ctrl-C inside the passthrough terminal writes `\x03` to PTY stdin. A future Stop affordance in `PassthroughTerminal` can call `client.request("agent.cancel", { session_id }, 10_000)` over WS; that route reaches `Service.CancelAgent` -> `agentManager.CancelAgentBySessionID` in `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`, which branches on `execution.PassthroughProcessID != ""` and writes `\x03` to PTY stdin. DB reconciliation completes regardless of the write outcome so the UI unsticks.

### Not included

- `TodoIndicator` — ACP-only (requires message stream); not rendered in `PassthroughToolbar`.
- Per-message model/mode picker — ACP-only feature; not applicable to PTY sessions.

## Out of scope

- Office / autonomous-agent CLI mode — explicitly deferred. Office launches stay on ACP for now.
- Parsing PTY output into `task_messages` (transcriber). The terminal panel shows live output; the chat transcript only shows user-sent text in CLI mode.
- Billing-type / subscription-quota badge or DTO surface. That belongs to `subscription-usage.md`.
- Adding passthrough support to agents that don't have it (`Supported: false`).
- Adding `pi-acp` to the managed runtime update catalogue.
- Migrating away from the current ACP bridge.
- Headless `-p` mode for Claude (drains API credit; we deliberately do not use it).

## Follow-ups

- **Stop button overlay in `PassthroughTerminal`.** Small affordance that calls the existing cancel route. Today users press Ctrl-C inside xterm directly.
- **Agent-specific prompt-injection tuning** — passthrough agents without a `PromptFlag` use the idle-based stdin route; adding agent-specific submit behavior requires verifying its submit sequence and idle behavior.

## Open questions

- **Submit sequence for Claude TUI**: `"\r"` is the expected default. If the TUI swallows the first `\r` as a focus-acquisition event we may need a tiny pre-write delay or `"\r\n"`. Resolve empirically during dogfooding; the field is configurable per agent.
- **Idle window for Claude TUI**: today's default (3s) should be enough — the TUI prints its banner and then sits at the prompt. Tune the per-agent `IdleTimeout` only if real sessions misfire.
