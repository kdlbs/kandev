---
title: "Developer Tools"
description: "Use quick chat, utility agents, saved prompts, voice input, editors, language servers, and task terminals."
---

# Developer Tools

Kandev includes short-lived chat, reusable AI helpers, dictation, file and editor integration, language servers, and terminals. Some tools run in a task environment; others run on the Kandev backend host or in the browser. That boundary determines which files, executables, credentials, and network services they can reach.

## Quick Chat

Quick Chat is an agent conversation outside the board. Use it for repository orientation, experiments, and disposable questions that do not need workflow state, review gates, dependencies, or a delivery record.

### Comment on an agent reply

In Task Chat or Quick Chat, select text from a settled agent prose reply, then select the comment button that appears beside the selection. The editor works like plan comments: enter feedback and choose **Add** to keep it as pending context, or **Run** to send it immediately. Pending selections use the same inline highlight and comment badge as plans; select either one to update or delete the feedback. Pending message comments are kept for the current browser tab, appear as composer context chips, and are included in the next prompt as Markdown. If the agent is busy, **Run** queues the feedback for its next turn.

Inline comments are available only on ordinary settled prose. Streaming replies, tool/thinking/status output, plans, rich-block content, raw views, and user messages do not accept inline comments.

### Reference tasks and work items

In Task Chat or an ordinary structured Quick Chat, type `#` at the start of a line or after whitespace, then enter part of a title or key. Kandev searches the active workspace's tasks and connected Jira, Linear, GitHub, GitLab, Azure DevOps, and Sentry sources. Results stay grouped by provider and type; a disconnected or slow provider does not hide results from another source.

Use the arrow keys and **Tab** or **Enter**, or select a row with pointer or touch. Selection inserts a chip without sending. The chip survives draft reloads and becomes a clickable reference after explicit send; messages queued while an agent is busy keep the same reference metadata. CLI-passthrough chat leaves `#` as literal text and does not search.

Use `@` for files, saved prompts, and the current plan. New task lookup is under `#`; existing saved or sent `@task` references remain readable and sendable.

Select **Quick Chat** beside **New Task** in the expanded sidebar, or select its standalone row in the collapsed sidebar.

### Start a chat

1. Turn on **Configuration chat** when the conversation should inspect or change Kandev configuration. This option is hidden when the workspace already has a configuration conversation.
2. Choose an agent profile. Quick Chat requires one and defaults to the workspace's default agent profile when configured.
3. For an ordinary Quick Chat, optionally add one or more workspace repositories.
4. For each repository, choose a branch. The same repository cannot be added twice.
5. Select **Start chat**.

Each selected repository gets an isolated worktree from the chosen branch. Uncommitted changes in your original checkout are not copied. Without a repository, Kandev creates an ephemeral working directory under `<KANDEV_HOME_DIR>/quick-chat/` (by default `~/.kandev/quick-chat/`).

Quick Chat supports multiple tabs, tab renaming, and **+** to open another ordinary-chat setup. Structured profiles show ACP chat; CLI-passthrough profiles show their PTY interface. The desktop window is resizable, while mobile uses a full-screen view.

Closing a real chat tab permanently deletes its conversation, hidden backing task data, and associated worktree. There is no undo. Kandev also deletes abandoned chats after seven days; cleanup runs when the backend starts and then once per day. Only chats whose session is `RUNNING` or `IDLE` are protected from age-based cleanup. Old `CREATED`, `STARTING`, or `WAITING_FOR_INPUT` chats can expire, so do not use Quick Chat for durable work.

If **Start chat** is disabled, select a profile and finish every repository/branch row. If a repository is missing, confirm that it belongs to the current workspace and refresh the repository configuration. Use a normal task when the result must remain visible on a board or become a reviewed PR.

## Utility agents

Open **Settings > Utility Agents** (`/settings/utility-agents`). Utility agents are one-shot ACP calls used to generate small pieces of text; they do not create a durable task conversation.

The built-in actions are:

- `commit-message`
- `commit-description`
- `branch-name`
- `pr-title`
- `pr-description`
- `enhance-prompt`
- `summarize-session`

Set a global **Default utility agent model** by choosing an inference-capable agent and one of its models. Each built-in action can inherit that pair or override it, and its prompt template is editable. Kandev probes the agent for its live model list; refresh or retry when model discovery fails.

You can also create a custom utility. Name, prompt, agent, and model are required; description is optional. Prompt fields offer autocomplete for supported `{{...}}` template variables. A per-action override must specify a usable agent/model pair; otherwise resolution falls back to the global pair. Buttons that depend on a utility are disabled or return an error when no valid model is available.

Utility calls run as ephemeral processes on the Kandev backend host. Kandev records the resolved prompt, response, selected model, token counts when provided, duration, status, and error. The prompt can include repository, diff, task, or conversation context, so its content goes to the selected model provider. Apply the same credential, retention, and data-classification rules as a normal agent session.

### Configuration Chat

The same settings page configures the **Configuration Chat Agent** for each workspace. Choose a profile, choose **No default**, or rely on the workspace default profile. Kandev remembers the first explicit selection as that workspace's configuration-chat default.

Open Configuration Chat from the floating chat button on Settings pages, turn on **Configuration chat** while creating a Quick Chat, or run **Configuration Chat** from the `Cmd/Ctrl+K` command menu. A workspace currently has one configuration conversation. The Settings panel shows that conversation without tabs; **Open in Quick Chat** moves the same setup or session into the larger tabbed dialog without copying it.

Configuration Chat uses a repository-less ephemeral task. Its configuration-mode MCP can inspect and change workflows, agent profiles, and MCP configuration. The selected profile's model, credentials, permissions, and external MCP settings apply. Review requested configuration mutations before approving them.

Closing the floating Settings panel preserves the conversation. To delete it, open it in Quick Chat, close its tab, and confirm deletion. Configuration tasks are excluded from the seven-day Quick Chat sweeper and remain available until explicitly deleted or their workspace is deleted.

## Saved prompts

Open **Settings > Prompts** (`/settings/prompts`) to add, edit, or delete reusable prompts. A saved prompt needs a unique name and non-empty content.

Type `@` in the task chat composer and select a prompt. The visible message keeps the `@name`; Kandev expands the prompt content into hidden system context for the agent. References are recognized only at the start of the text or after whitespace and must match the stored name. Prompt content can reference other saved prompts. Expansion stops at a depth of eight, skips cycles, and includes each prompt only once.

Kandev seeds these built-ins:

- `code-review`
- `open-pr`
- `merge-base`
- `ci-auto-fix`
- `changes-walkthrough`

Built-ins are marked in the UI but remain editable. Editing `ci-auto-fix` or `changes-walkthrough` changes the corresponding PR repair or walkthrough action. Seed insertion does not overwrite edits. If you delete a built-in, it stays absent for the current backend run and is seeded again on the next service start. There is no reset-to-default button.

A saved prompt is an instruction, not an authorization or policy boundary. Executor permissions, human gates, tests, and provider protections still control what can happen.

## Voice Mode

Open **Settings > Voice Mode** (`/settings/voice-mode`). Voice Mode inserts a transcript at the cursor in the active chat composer.

Defaults are:

| Setting | Default |
|---|---|
| Enabled | On |
| Engine | Automatic |
| Language | Auto-detect |
| Activation | Click to toggle |
| Auto-send | Off |
| Whisper Web model | Base, approximately 75 MB |
| Shortcut | `Cmd/Ctrl+Shift+M` |

The shortcut is also configurable under **Settings > General > Keyboard Shortcuts**. Hold-to-talk applies on a fine-pointer device. On touch/coarse-pointer devices, Kandev uses toggle behavior while preserving the stored preference. With auto-send enabled, a successful transcript is sent as soon as it is inserted.

### Choose an engine

| Engine | Where recognition happens | Requirements and data flow |
|---|---|---|
| **Automatic** | First available engine | Selects the first currently available capability in this order: Web Speech, Whisper Web, then Whisper Server. A pinned engine that is unavailable is resolved through the same capability order. |
| **Web Speech** | Browser-provided implementation | No audio is sent to the Kandev backend. Browser/vendor behavior and privacy policy still apply, and some implementations require a network service. |
| **Whisper Web** | In the browser | Downloads and caches an ONNX model from Hugging Face, then runs local inference in a worker. Tiny is about 40 MB, Base about 75 MB, and Small about 240 MB. |
| **Whisper Server** | Kandev backend and OpenAI | The browser uploads audio to Kandev; Kandev sends it to OpenAI's `whisper-1` transcription API. Configure `KANDEV_VOICE_OPENAI_API_KEY` on the backend. |

Whisper Server accepts at most 10 MiB per request and has a 60-second backend timeout. It returns an unavailable error when the key is not configured, a payload-too-large error above the limit, and an upstream error when transcription fails. Automatic selection can choose this unconfigured server when both browser capabilities are unavailable.

Engine choice is capability selection, not runtime failover. Once recognition or transcription starts, an error does not retry the request through the next engine. The backend `/api/v1/transcribe` route has no Kandev authentication and spends the configured OpenAI key; protect the whole backend origin and do not publish that endpoint directly.

Microphone capture requires browser permission and normally HTTPS or localhost. Whisper Web also needs `getUserMedia`, `MediaRecorder`, workers, enough browser storage, and a first-use network download. The UI mentions common Chrome, Edge, and Safari versions, but Kandev does not enforce a browser/version allow-list; runtime availability is determined from the required APIs. If recording fails, check site permission, input device, secure context, model download/cache, browser support, and network access. The composer must remain enabled; switching tasks or disabling the input cancels recording.

## Files and editor integrations

The task **Files** panel browses, searches, opens, and edits task-worktree files. Kandev rejects file paths that escape the resolved worktree. A session with one worktree opens that worktree directly in a host editor. When a session has several worktrees, the editor button asks which repository or worktree to open, and each configured editor in the adjacent menu expands to the same repository-and-branch picker. Check that selection before launching an editor from a multi-repository task. Older API clients that omit `worktree_id` retain the first-worktree fallback.

**VS Code (Embedded)** runs code-server inside the task environment and displays it in a workbench panel. Opening it starts code-server independently of the agent process. It launches with `--auth none` and binds `0.0.0.0` on a random port inside that runtime; network/firewall isolation is therefore important, especially for Local and SSH environments. If the binary is absent, agentctl attempts to download it into `~/.kandev/tools/code-server`; first use needs a supported host platform and access to the GitHub release. Use the panel error and task-environment logs when installation, startup, or proxying fails.

Use the workbench top bar's split-editor action to open the selected session worktree in the default editor. Its menu lets you choose another configured editor. A file's **Open with** menu can also open a specific editor, copy the path, or ask the operating system to show the folder.

Open **Settings > General > Editors** (`/settings/general/editors`) to set a default and configure integrations. Kandev discovers these built-in desktop editors when installed:

- Visual Studio Code
- Zed
- Cursor
- Windsurf
- IntelliJ IDEA
- GoLand
- PhpStorm

You can add:

- a custom command with `{cwd}`, `{file}`, `{rel}`, `{line}`, and `{column}` placeholders;
- VS Code Remote SSH with a required host and optional user and URI scheme;
- a hosted editor base URL, to which Kandev adds file or folder query parameters.

Desktop editor discovery and custom commands run on the Kandev backend host. The executable must be installed there and visible in its `PATH`; in a remote browser deployment, a local-editor command may therefore launch on the server rather than your laptop. Remote SSH needs a reachable SSH host and a registered VS Code URI handler. Hosted URLs need an accessible service and receive the absolute backend-host path in their `file` or folder query parameter. Configure only trusted custom commands because invoking one executes it on the host.

## Language servers

Language-server settings are part of **Settings > General > Editors**. Kandev currently registers servers for:

- TypeScript and JavaScript;
- Go;
- Rust;
- Python.

Auto-start and auto-install are off for every language by default. Enable only the languages used by the workspace, then save settings. A file toolbar can start or stop a server manually; browser-local storage remembers manual enablement for that session and language. Kandev disconnects an unused server connection after two minutes.

Server lookup checks installed executables and `<KANDEV_HOME_DIR>/lsp-servers` (by default `~/.kandev/lsp-servers`). Auto-install uses different toolchains:

- TypeScript/JavaScript and Python install npm packages into Kandev's language-server storage;
- Go runs `go install ...@latest` and therefore needs a working Go toolchain;
- Rust downloads a release for supported macOS or Linux, x86-64 or ARM64 hosts. Windows is not registered for automatic Rust installation.

The default Go server configuration enables semantic tokens. A custom language-server configuration must be a JSON object; Kandev sends it through the language-server workspace configuration request. Installing a language server does not install project dependencies or make an unsupported language available. If startup fails, check the backend log/status, executable `PATH`, supported host platform, toolchain/network access, project dependencies, and that the file belongs to the active task worktree.

Language-server subprocesses run on the Kandev backend host with the task workspace path as their working directory; they are not launched inside a Docker container, SSH host, or Sprites sandbox. Remote-only paths, binaries, and project dependencies are therefore normally unavailable to this LSP path even when the agent itself can use them.

## Integrated terminal

The task terminal is a PTY in the task environment. Local tasks run through the local environment; Docker, SSH, and other remote executors route the terminal to their configured runtime.

On desktop, select **+ > Terminals > New Terminal**. Parked terminal sessions can appear in the same menu for reopening. The tab context menu offers **Rename** and **Terminate**, not a separate Close action. Selecting the tab's **X** deletes the terminal and asks for confirmation when it is busy. Removing the panel through layout management can instead park it and keep its PTY alive; terminating a live terminal or deleting a parked entry destroys it. On mobile, select **Terminal** in task navigation and use the terminal picker to create another.

Do not confuse a user terminal with a CLI-passthrough agent tab: both use PTYs, but only the agent tab is the agent's native interface. `Cmd/Ctrl+J` toggles the bottom terminal area.

Open **Settings > General > Terminal** (`/settings/general/terminal`) to configure:

- preferred shell, which defaults to the system shell; the built-in choices are zsh, bash, and sh on macOS/Linux, and PowerShell (`pwsh`), Windows PowerShell, and cmd on Windows, plus a custom executable;
- terminal font, with a default Menlo/Monaco-style stack;
- font size, default 13 px and allowed range 8–24 px;
- URL handling, which defaults to a new browser tab and can instead use Kandev's built-in browser panel.

Shell changes apply whenever a new or restarted terminal is created, including inside an existing task. Only an already-live PTY keeps its current shell. A custom shell must exist in the task environment. Fonts render only when available to the browser. Commands can behave differently from your login shell because the executor may use another user, `PATH`, credentials, working directory, or startup files.

If no terminal can be created, wait for the task environment to become ready and confirm its executor is reachable. If a reopened terminal is dead, create a new one; the original PTY or remote connection may have exited.

## Choose the right surface

| Need | Use |
|---|---|
| Disposable question or experiment | Quick Chat |
| Board history, branch, review, dependency, or PR | A normal task and workflow |
| Small generated title, summary, or description | Utility agent |
| Reusable task-chat instructions | Saved prompt |
| Dictate into chat | Voice Mode |
| Browse or edit task files | Files and editor integration |
| Diagnostics for supported languages | Language server |
| Run a command in the task runtime | Integrated terminal |

Related: [Use Kandev](use-kandev.md), [Sessions and review](sessions-and-review.md), [Agents and profiles](agents-and-profiles.md), and [Integrations](integrations.md).
