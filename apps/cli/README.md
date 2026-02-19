# Kandev

Manage tasks. Orchestrate agents. Review changes. Ship value.

## Quick Start

```bash
npx kandev
```

Downloads the latest release, starts the backend + web app, and opens your browser. Data (worktrees, SQLite DB) is stored in `~/.kandev` by default.

## What You Get

- **Multi-agent support** - Claude Code, Codex, GitHub Copilot, Gemini CLI, Amp, Auggie, OpenCode
- **Integrated workspace** - Terminal, code editor with LSP, git changes, browser preview, and chat
- **Kanban & pipeline views** - Organize tasks with opinionated workflows and gates
- **CLI passthrough** - Drop into raw agent TUI mode for full terminal access
- **Workspace isolation** - Git worktrees prevent concurrent agents from conflicting
- **Session management** - Resume and review agent conversations

## Supported Agents

| Agent              | Default Model  | Protocol    |
| ------------------ | -------------- | ----------- |
| **Claude Code**    | Sonnet 4.5     | stream-json |
| **Codex**          | GPT-5.2 Codex  | Codex       |
| **GitHub Copilot** | GPT-4.1        | Copilot SDK |
| **Gemini CLI**     | Gemini 3 Flash | ACP         |
| **Amp**            | Smart Mode     | stream-json |
| **Auggie**         | Sonnet 4.5     | ACP         |
| **OpenCode**       | GPT-5 Nano     | REST/SSE    |

> **Beta** - Under active development. Expect rough edges and breaking changes.

## Requirements

- Node.js (for `npx`)
- Git
- Docker (optional - needed for container runtimes)

## Platforms

- macOS (Intel + Apple Silicon)
- Linux (x64)
- Windows (x64, WSL)

## Learn More

Open source, multi-provider, no telemetry, not tied to any cloud.

See the [GitHub repository](https://github.com/kdlbs/kandev) for architecture, vision, development setup, and contributing guidelines.

## License

[AGPL-3.0](https://github.com/kdlbs/kandev/blob/main/LICENSE)
