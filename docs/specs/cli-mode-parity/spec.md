---
status: draft
created: 2026-05-16
owner: cfl
issue: https://github.com/kdlbs/kandev/issues/906
needs-upgrade: true
---

# CLI-Mode Task Parity (Kanban)

## Why

Anthropic has announced that **agent-SDK / `claude -p` usage will draw from a paid API budget**, while the interactive `claude` CLI continues to draw from the user's Pro/Max subscription quota. Today kandev drives Claude almost exclusively through the ACP bridge (`@agentclientprotocol/claude-agent-acp`), which is an SDK-mode integration. After the change, every kandev task run by a subscription user would burn API dollars they do not have.

Kandev already supports a per-profile **CLI Passthrough** mode that launches the agent CLI under a PTY (`apps/backend/internal/agentctl/server/process/interactive_runner.go`). Users who enable it keep their subscription billing — but the experience is bare:

1. The task-create dialog **disables the prompt textarea** when a CLI agent is selected (legacy assumption: "you'll type your prompt directly into the terminal"). So the user can't even attach a description to a CLI-mode task.
2. The task description that kandev would have sent through ACP is **not delivered to the CLI**. The user has to retype it.
3. The chat compose box on a CLI-mode session does nothing — there is no path from kandev's UI into the agent's stdin for follow-up messages.

This spec brings CLI-passthrough mode up to feature parity with ACP for the **kanban** task-execution surface. Office (autonomous) mode is explicitly deferred.

## What

### Prompt allowed at task creation in CLI mode

The task-create dialog's prompt textarea is **enabled** when a CLI/passthrough-capable agent is selected. Users can write the prompt the same way they would for ACP. The control flow that today gates this off when `cli_passthrough` is true is removed.

### Task-prompt injection (idle-based, agent-agnostic)

When a passthrough session starts for a task that has a description:

1. Kandev launches the PTY as today.
2. After the existing **idle detector** in `InteractiveRunner` fires for the first time (i.e. the CLI has stopped emitting output and is presumed to be at its input prompt), kandev writes the task description plus a configurable **submit sequence** to the PTY's stdin.
3. Auto-injection is opt-in per agent via a new `PassthroughConfig.AutoInjectPrompt` flag (default false). Agents that already use `PromptFlag` (headless one-shot mode) are unaffected — their prompt is already on the CLI before launch.

No per-agent pattern matchers. The existing idle window is the only readiness signal. If an agent's CLI is unusual enough that an idle window misfires (writes a banner, then waits 5 seconds, then prompts), we make the idle window per-agent-configurable in `PassthroughConfig` (already exists as `IdleTimeout`). No new detection machinery.

For the Claude case: `claude_acp.go` sets `AutoInjectPrompt: true`, `DisableBracketedPaste: true` (Claude Code already enables bracketed-paste *mode* in its Ink TUI — injecting `ESC[200~`…`ESC[201~` delimiters breaks the prompt), and `SubmitViaBackslashEnter: true` (PTY writes: prompt, then `\`, then `\r` per [Claude terminal docs](https://code.claude.com/docs/en/terminal-config)). Ink may still treat programmatic Enter as newline only ([anthropics/claude-code#15553](https://github.com/anthropics/claude-code/issues/15553)) — if auto-submit fails, the user confirms with Enter. Other passthrough-capable agents use bracketed-paste delimiters for multi-line prompts and stay default-off for auto-inject unless configured.

### Follow-up prompts via PTY stdin

The orchestrator's `PromptTask` handler branches on `IsPassthroughSession(sessionID)` and writes the prompt text + `SubmitSequence` to the agent's PTY stdin instead of sending over ACP. This is the same path used by auto-injection and is reachable today from any caller that hits `agent.prompt` for a passthrough session.

`PassthroughComposer` provides the UI surface for this route. It can be opened from the passthrough toolbar or via the configured focus-input keyboard shortcut, then submits text through `message.add` for the current passthrough session.

### Kandev MCP tools are wired for CLI mode

When a passthrough agent supports an MCP config flag, kandev generates a per-session MCP config file that points at the task's local agentctl MCP endpoint and launches the CLI with that config. The generated config exposes the same kandev tool server used by ACP sessions, scoped by the already-known `KANDEV_TASK_ID` / `KANDEV_SESSION_ID` context.

For the Claude case, the passthrough command includes `--mcp-config <generated-file>` with an `mcpServers.kandev` HTTP entry for the local agentctl `/mcp` endpoint.

### Stop sends Ctrl-C

The orchestrator's `CancelAgent` handler branches on `IsPassthroughSession(sessionID)` and writes `\x03` to the PTY's stdin instead of sending an ACP cancel. DB reconciliation still runs so the UI unsticks regardless of the write outcome.

Users can still press Ctrl-C directly inside the xterm terminal. A dedicated toolbar button that calls the same cancel route remains a follow-up.

## Scenarios

### CLI-mode task can have a prompt
- GIVEN a Claude profile with `cli_passthrough: true`
- WHEN the user opens the task-create dialog and types in the prompt field
- THEN the prompt textarea accepts input (today it is disabled / hidden)

### Prompt injection on fresh start
- GIVEN a CLI-mode task whose agent has `AutoInjectPrompt: true` and a non-empty description
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

### Agent without AutoInjectPrompt
- GIVEN a passthrough-capable agent with `AutoInjectPrompt: false`
- WHEN a task starts
- THEN no stdin write happens (today's behavior preserved); the user pastes their prompt manually

## Kandev toolbar above the passthrough terminal

### Where it appears

`PassthroughToolbar` (`apps/web/components/task/passthrough-toolbar.tsx`) wraps `PassthroughTerminal` with the same kandev controls that ACP sessions get from `ChatStatusBar` + `ChatInputArea`. It is mounted:

- Full-page task view: `dockview-shared.tsx` (`DockviewSharedContent`) renders it for passthrough sessions in place of the chat input area; the dockview desktop layout in `task-center-panel.tsx` uses the same shared content.
- Kanban Preview tab: `preview-session-tabs.tsx` renders `PassthroughComposer` directly (compose-only, no terminal wrap) for passthrough sessions in the side-peek surface.

### What it surfaces

The status row at the bottom of `PassthroughToolbar` contains, left to right:

- `PRStatusChip` — PR open/merged/draft indicator for the task.
- `PRMergedBanner` — banner shown when the PR is merged.
- `passthrough-proceed-next-step` button — moves the task to the next workflow step. Only shown when `nextStepName` is non-null and the agent is not busy (`sessionState` is not `RUNNING` or `STARTING`).
- `ChatToggleButton` — toggles the `PassthroughComposer` open and closed. Shows `variant="default"` when the composer is open or when there are pending review comments; shows a numeric chip (`data-testid="passthrough-pending-count"`) when collapsed with pending comments.

### Default focus contract

The terminal (`PassthroughTerminal`) renders above the status row and fills the remaining height. The compose box is collapsed by default so xterm retains keyboard focus for raw PTY interaction. The user opts in to the kandev composer explicitly via the Chat button.

The configured focus-input keyboard shortcut also opens the composer and focuses its textarea when the user is not already typing in another input. The shortcut mirrors ACP chat focus behavior without stealing focus from the terminal by default.

### Composer assistance

`PassthroughComposer` supports the same lightweight authoring affordances expected from ACP compose surfaces:

- Slash commands are sourced from the session's available agent commands and filtered by the typed prefix. Bundled implementation details are omitted from the menu.
- `@` file references search workspace files for the active session and insert references as `@path/to/file`.
- Dropping files or directories into the composer inserts `@path` references for filesystem-backed entries and falls back to uploaded file names when paths are unavailable.
- Suggestions are keyboard navigable with Arrow Up, Arrow Down, Tab, Enter, and Escape.

### How the composer formats and sends pending review comments

When `PassthroughComposer` submits:

1. `usePendingDiffCommentsByFile(sessionId)` provides comments grouped by file path.
2. Non-empty `pendingComments` are formatted via `formatReviewCommentsAsMarkdown` (produces `### Review Comments\n…`) and prepended to the typed text.
3. The combined string is sent via `client.request("message.add", { task_id, session_id, content }, 10_000)`.
4. `markCommentsSent(ids)` is called only on WS success — a failed send leaves both the composer open (typed text preserved for retry) and the comments pending.
5. A `PendingCommentsBanner` (`data-testid="passthrough-pending-comments-banner"`) renders inside the open composer when `pendingCount > 0`.

### How Stop maps to Ctrl-C

Pressing Ctrl-C inside the passthrough terminal writes `\x03` to PTY stdin. A future Stop affordance in `PassthroughTerminal` can call `client.request("agent.cancel", { session_id }, 10_000)` over WS; that route reaches `Service.CancelAgent` -> `agentManager.CancelAgentBySessionID` in `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`, which branches on `execution.PassthroughProcessID != ""` and writes `\x03` to PTY stdin. DB reconciliation completes regardless of the write outcome so the UI unsticks.

### Not included

- `TodoIndicator` — ACP-only (requires message stream); not rendered in `PassthroughToolbar`.
- Plan-comments and PR-feedback attachment — same pattern as diff comments if needed in future; deferred.
- Per-message model/mode picker — ACP-only feature; not applicable to PTY sessions.

## Out of scope

- Office / autonomous-agent CLI mode — explicitly deferred. Office launches stay on ACP for now.
- Parsing PTY output into `task_messages` (transcriber). The terminal panel shows live output; the chat transcript only shows user-sent text in CLI mode.
- Billing-type / subscription-quota badge or DTO surface. That belongs to `subscription-usage.md`.
- Adding passthrough support to agents that don't have it (`Supported: false`).
- Migrating away from the current ACP bridge.
- Headless `-p` mode for Claude (drains API credit; we deliberately do not use it).

## Follow-ups

- **Stop button overlay in `PassthroughTerminal`.** Small affordance that calls the existing cancel route. Today users press Ctrl-C inside xterm directly.
- **AutoInjectPrompt on other passthrough agents** (Codex CLI, OpenCode TUI, etc.) — only Claude is opted in for v1. Adding others requires verifying their submit sequence and idle behavior.

## Open questions

- **Submit sequence for Claude TUI**: `"\r"` is the expected default. If the TUI swallows the first `\r` as a focus-acquisition event we may need a tiny pre-write delay or `"\r\n"`. Resolve empirically during dogfooding; the field is configurable per agent.
- **Idle window for Claude TUI**: today's default (3s) should be enough — the TUI prints its banner and then sits at the prompt. Tune the per-agent `IdleTimeout` only if real sessions misfire.
