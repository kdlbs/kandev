---
title: "Feature Status"
description: "Understand which Kandev capabilities are supported, dependency-bound, experimental, or still in progress."
---

# Feature Status

These docs describe the current `main` branch and are not versioned yet. A released binary can be older than this page. Check **Settings > System > About** or `kandev --version`, then compare the release notes when a screen or option differs.

## Status meanings

| Status | Meaning |
|---|---|
| Supported | Shipped in the normal product path and covered by maintained source/tests. |
| Dependency-bound | Shipped, but availability depends on an agent, provider, platform, credential, or executor. |
| Experimental | Available behind a toggle or with a deliberately limited operating contract. |
| In progress | Present in source or planning, but not documented as a supported production workflow. |
| Internal | Engineering material, test support, or implementation detail rather than a user feature. |

## User-facing capability matrix

| Area | Status | Notes |
|---|---|---|
| Workspaces, repositories, kanban, workflows, tasks, labels, and documents | Supported | Core coordination model. See [Tasks and workflows](tasks-and-workflows.md). |
| Parallel sessions and integrated files, terminal, changes, preview, plans, and PR review | Supported | Exact panels vary by viewport and task context. See [Sessions and review](sessions-and-review.md). |
| Code walkthroughs with anchored review feedback | Supported | Requires a compatible agent session with task MCP and matching files. |
| Subtasks, dependencies, multi-repository tasks, and additional task branches | Supported | Shared workspaces can still conflict; see [Coordination](coordination.md). |
| ACP agents and bring-your-own TUI agents | Dependency-bound | Installation, login, models, modes, resume, usage, and MCP capabilities vary by CLI. |
| Local, worktree, Docker, remote Docker, SSH, and Sprites executors | Dependency-bound | Each runtime has platform, host, image, credential, and lifecycle prerequisites. |
| GitHub, GitLab, Jira, Linear, Sentry, and Slack integrations | Dependency-bound | Provider API, account scope, and workspace configuration determine behavior. |
| Workflow import/export and GitHub-backed workflow sync | Supported | Imported profiles may require local mapping; sync applies guarded updates. |
| Task MCP and external backend MCP | Supported | External MCP has no Kandev user-auth boundary and must be network-protected. |
| Scheduled, webhook, GitHub PR/push/CI automations | Supported | Begin with narrow filters, credentials, and concurrency. |
| Desktop app, CLI, service, Docker, and remote-host deployment | Supported | Platform-specific installation and signing details vary by release channel. |
| Kubernetes deployment guide | Experimental | The guide describes deploying Kandev; a first-class Kandev Kubernetes executor/operator is not shipped. |
| Resource metrics, statistics, backups, logs, updates, and task Gist shares | Supported | Some provider metrics and update actions depend on install channel. |
| Voice engines and language-server/editor integration | Dependency-bound | Browser APIs, models, binaries, credentials, and platform support vary. |
| Feature toggles | Experimental | Intended for unfinished or diagnostic behavior and may require restart. |
| Office mode | In progress | Feature-flagged autonomy work; persistent agent teams, routines, budgets, and related UX are not yet a supported public contract. |
| Kubernetes executor/operator | In progress | Listed as a future remote runtime direction, not a current executor choice. |

## Source and publication boundary

User-ready pages live in `docs/public/**` and are published at `kandev.ai/docs`. Other files under `docs/**` can be architecture notes, ADRs, plans, specifications, test procedures, release internals, or historical context. Their presence does not change a capability's public status.

When behavior changes, update the implementation, tests, relevant public page, and this matrix in the same pull request. See [Contributing to the docs](README.md) and [Contributing to Kandev](contributing.md).

## Verify behavior in your installation

For a feature you intend to rely on:

1. confirm the running Kandev version;
2. inspect the relevant settings screen and health status;
3. verify the selected agent/executor/provider dependency;
4. test the smallest representative task without production credentials;
5. confirm failure, retry, review, and cleanup behavior;
6. keep a human gate until the workflow has proven safe for its repository.

Open a bug with the version, platform, executor, agent profile, reproduction steps, expected behavior, logs, and screenshots when the current release disagrees with these docs.
