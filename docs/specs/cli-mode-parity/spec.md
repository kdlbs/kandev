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

For the Claude case: `claude_acp.go` sets `AutoInjectPrompt: true`, `SubmitSequence: "\r"`. Other passthrough-capable agents stay default-off and gain auto-inject later.

### Follow-up prompts via PTY stdin (backend route landed; UI surface deferred)

The orchestrator's `PromptTask` handler branches on `IsPassthroughSession(sessionID)` and writes the prompt text + `SubmitSequence` to the agent's PTY stdin instead of sending over ACP. This is the same path used by auto-injection and is reachable today from any caller that hits `agent.prompt` for a passthrough session.

**UI gap (deferred):** kandev's chat compose box is rendered by `ChatInputArea`, which is replaced entirely by `PassthroughTerminal` when `session.is_passthrough` is true. There is no compose surface in passthrough mode today. Users send follow-ups by typing directly into the terminal (xterm forwards each keystroke to the PTY) — which works perfectly for the Claude CLI use case. A dedicated kandev compose box that drives the PTY (without replacing the terminal) is a follow-up.

### Stop sends Ctrl-C (backend route landed; UI surface deferred)

The orchestrator's `CancelAgent` handler branches on `IsPassthroughSession(sessionID)` and writes `\x03` to the PTY's stdin instead of sending an ACP cancel. DB reconciliation still runs so the UI unsticks regardless of the write outcome.

**UI gap (deferred):** Same root cause as the compose-box gap — `ChatInputArea` (which hosts the Stop button) is not rendered in passthrough mode. Users today press Ctrl-C directly in the xterm terminal, which travels through the same PTY stdin path. A dedicated Stop button overlay in `PassthroughTerminal` is a follow-up.

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

### Stop / cancel route is in place (UI surface follow-up)
- GIVEN a CLI-mode task running
- WHEN any caller invokes `CancelAgent` for the session
- THEN `\x03` is written to the PTY
- AND DB reconciliation still completes so the session unsticks

### Agent without AutoInjectPrompt
- GIVEN a passthrough-capable agent with `AutoInjectPrompt: false`
- WHEN a task starts
- THEN no stdin write happens (today's behavior preserved); the user pastes their prompt manually

## Out of scope

- Office / autonomous-agent CLI mode — explicitly deferred. Office launches stay on ACP for now.
- Parsing PTY output into `task_messages` (transcriber). The terminal panel shows live output; the chat transcript only shows user-sent text in CLI mode.
- Billing-type / subscription-quota badge or DTO surface. That belongs to `subscription-usage.md`.
- Adding passthrough support to agents that don't have it (`Supported: false`).
- Migrating away from the current ACP bridge.
- Headless `-p` mode for Claude (drains API credit; we deliberately do not use it).

## Follow-ups

- **Compose box in passthrough mode.** Surface a dedicated kandev compose input (alongside the PTY terminal) that writes to PTY stdin via the now-wired backend route. Today users type into xterm directly, which works but is inconsistent with kandev's ACP UX.
- **Stop button overlay in `PassthroughTerminal`.** Small affordance that calls the existing cancel route. Today users press Ctrl-C inside xterm directly.
- **AutoInjectPrompt on other passthrough agents** (Codex CLI, OpenCode TUI, etc.) — only Claude is opted in for v1. Adding others requires verifying their submit sequence and idle behavior.

## Open questions

- **Submit sequence for Claude TUI**: `"\r"` is the expected default. If the TUI swallows the first `\r` as a focus-acquisition event we may need a tiny pre-write delay or `"\r\n"`. Resolve empirically during dogfooding; the field is configurable per agent.
- **Idle window for Claude TUI**: today's default (3s) should be enough — the TUI prints its banner and then sits at the prompt. Tune the per-agent `IdleTimeout` only if real sessions misfire.
