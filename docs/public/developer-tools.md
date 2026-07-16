---
title: "Developer Tools"
description: "Use quick chat, utility agents, voice input, editors, terminals, prompts, notifications, and workbench preferences."
---

# Developer Tools

Kandev combines task coordination with the developer surfaces needed to inspect and finish work. Most tools operate in the current task environment, while quick chat and utility agents handle shorter work that does not need a full workflow.

## Quick chat

Quick chat opens a focused agent conversation without first creating a kanban task. Use it for short questions, repository orientation, command help, or other disposable interactions.

Use a normal task when the work needs durable workflow state, a dedicated branch or worktree, structured documents, review gates, dependencies, or a pull request. Quick chat is not a substitute for that delivery record.

## Utility agents

**Settings > Utility Agents** configures small agent-backed jobs used by Kandev itself. Built-in uses include generating or refining:

- task prompts;
- branch names;
- commit messages;
- pull-request descriptions;
- session summaries.

Each utility can select an agent/profile and prompt template. These jobs can send repository or task context to the configured model provider, so choose credentials and profiles with the same care as a normal session. Keep the fallback behavior usable when a utility agent or provider is unavailable.

## Prompts

**Settings > Prompts** manages reusable prompt templates. Workflows, utility agents, and task actions can reference them so a team can standardize planning, implementation, review, and handoff instructions.

Prompts are process policy, not enforcement. Pair them with workflow gates, scoped executor permissions, tests, and branch protection when those controls matter.

## Voice mode

**Settings > Voice Mode** configures dictation in the chat composer. Available engines depend on browser and deployment support:

- browser Web Speech;
- local in-browser Whisper Web;
- server-side Whisper transcription.

Settings include language, click-to-toggle or hold-to-talk activation, and optional auto-send. Server-side transcription requires its configured provider credential and sends audio outside the browser. Review the privacy and retention policy of the selected engine before using customer or proprietary content.

## Files, editor, and language servers

The task workbench can browse and edit files, search the workspace, preview images, and show diagnostics or language features when a matching language server is installed. **Settings > General > Editors** controls editor integrations and discovery.

The file tree reflects the selected task repository or attachment. In a multi-repository task, check the repository label before editing or reviewing. Agent changes can arrive while a file is open, so refresh the diff before commenting on line numbers.

## Terminal and shell

The integrated terminal runs in the task environment. For local/worktree executors that is the host; for Docker, SSH, or Sprites it is the selected runtime through agentctl.

**Settings > General > Terminal** controls shell behavior. A command that works in your login shell may fail under a service account, container, or remote environment because PATH, credentials, working directory, or startup files differ.

## Changes, review, and walkthroughs

The Changes panel groups repository state, staged/unstaged files, commits, branches, and pull requests. Review comments can be attached to exact lines and sent back to the agent. A generated code walkthrough presents an ordered tour of relevant file ranges.

See [Sessions and review](sessions-and-review.md) for the full review loop and [Git operations](git-operations.md) before rebasing or resolving conflicts.

## Personal settings

General settings include:

- appearance, panel layout, and resource-metric visibility;
- keyboard shortcuts;
- desktop/browser notifications;
- task-action confirmations;
- terminal and editor preferences;
- named secrets used by profiles.

Keyboard shortcuts are editable in **Settings > General > Keyboard Shortcuts**. Avoid assigning the same chord to global browser, terminal, editor, and Kandev actions.

Notifications depend on browser/desktop permission and operating-system policy. Treat them as convenience signals rather than the only alert path for an unattended workflow.

## Resource and usage visibility

Kandev can show host or environment CPU, memory, disk, load, and other supported metrics. It can also surface task/session statistics and provider usage hints where an integration exposes them.

These values help diagnose capacity and activity; they are not quotas, billing guarantees, or security controls. Configure hard limits in the executor, provider, or host platform.

## Choose the right surface

| Need | Use |
|---|---|
| Short disposable question | Quick chat |
| Branch, review, or delivery state | Normal task and workflow |
| Repeatable model-assisted metadata | Utility agent |
| Shared process instructions | Prompt template and workflow |
| Inspect or edit task files | Workbench editor |
| Run project commands | Task terminal |
| Explain a change to a reviewer | Changes panel and code walkthrough |

Related: [Get started](use-kandev.md), [Tasks and workflows](tasks-and-workflows.md), and [Agents and profiles](agents-and-profiles.md).
