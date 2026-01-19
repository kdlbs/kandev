# Kandev

Kandev is a local-first AI execution platform for software development work. It pairs a kanban-style task surface with live agent orchestration, file/workspace access, and real-time streaming of changes.

## Start locally

```bash
npx kandev
```

The launcher downloads the latest release bundle, starts the backend + web app, and opens your browser.

## What you get

- **Task-centric execution**: turn tasks into runnable sessions with streaming updates.
- **Workspace control**: shell, file, and git access scoped to each task.
- **Agent orchestration**: run different agent types and resume sessions.
- **Change visibility**: review diffs and track state per task/session.
- **Live UI**: WebSocket-driven updates across tasks, messages, and outputs.

## Requirements

- Node.js (for `npx`)
- Git (for repository access)
- Docker (optional; only needed for containerized agents)

## Platforms

- macOS (Intel + Apple Silicon)
- Linux (x64)
- Windows (x64) (not tested yet)

