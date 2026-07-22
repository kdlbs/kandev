---
title: "Feature Status"
description: "Check which Kandev capabilities are supported, dependency-bound, limited, experimental, or still in progress."
---

# Feature Status

This inventory describes the current `main` branch. A capability counts as shipped only when current backend behavior, a user-facing path, and tests agree. A database field, hidden route, ADR, specification, or mock-only E2E path is not enough.

Released builds can lag this page. Check **Settings > System > About** or `kandev --version` and compare the matching GitHub tag when behavior differs.

## Status meanings

| Status | Meaning |
|---|---|
| Supported | Available through the normal product path and backed by maintained implementation and tests. |
| Dependency-bound | Shipped, but useful behavior depends on a provider, agent CLI, executor, platform, credential, or install channel. |
| Limited | Implemented and reachable, but the current UI or operating contract has a material restriction called out here. |
| Experimental | Intentionally unstable or based on a fragile/nonstandard integration contract. |
| In progress | Source, schema, tests, a flag, or a stub exists, but there is no supported production workflow yet. |
| Internal | Development, test, diagnostic, or implementation support rather than an end-user feature. |

## Tasks, workflows, and coordination

| Capability | Status | Current boundary |
|---|---|---|
| Workspaces, local repositories, Kanban board, task list, workflow steps, and task archive | Supported | A fresh database contains Default Workspace and a Development workflow. Repository settings add existing local Git paths; remote GitHub sources enter through New Task. Archive state is immediate, while runtime/resource cleanup is asynchronous and best-effort. |
| Workflow templates, custom steps, prompts, per-step agent profiles, WIP/pull controls, and auto-archive | Supported | Configure under **Settings > Workspaces > _workspace_ > Workflows**. Imported/synced configuration can still require local profile mapping. |
| Regular workflow events and actions | Supported | Current UI covers step entry, turn start/completion, step exit, and direct-child completion, including auto-start, plan mode, reset context, moves, explicit completion gating, and human wait steps. The generic Office triggers and quorum/participant actions are not part of this contract. |
| Human-gated operation | Supported | A step can do nothing on turn completion and wait for a user; an optional `step_complete_kandev` signal can gate auto-advance. Kandev does not bypass repository checks, provider permissions, or branch protection. |
| One structured plan per regular task | Dependency-bound | Plan mode, the Plan panel, revisions, compare/revert, and Implement are shipped. Creating/updating the plan from an agent requires task MCP support; passthrough-only sessions do not have the same contract. |
| Multiple named task documents | In progress | Storage, revisions, Office UI, and Office MCP tools exist. Regular Kanban has no document panel and its task MCP does not register the document tools. |
| Task labels and label filters | In progress | The shared task row stores a JSON label field, but label catalogs, pickers, filters, and maintained E2E coverage are on the feature-flagged Office surface, not regular Kanban. |
| Named parallel sessions | Supported | Users can add, name, make primary, stop, resume, delete, and hand off sessions. Blank, copied, or summarized context is available when starting another agent; resume still depends on retained agent/executor state. |
| Agent-spawned sessions, targeted task messages, and direct-child stop | Dependency-bound | `spawn_session_kandev`, `message_task_kandev`, and `stop_task_kandev` are task-MCP tools. Queue limits, session IDs, direct-parent authority, and agent MCP capability apply. Stop is halt-only, covers all observed live sessions on the child, and schedules runtime teardown asynchronously. |
| One-level subtasks | Supported | Regular Kanban supports a parent plus direct children through UI and MCP. A child may inherit the parent's materialized workspace or request a new one; inherited/shared files can conflict. Arbitrary-depth Office trees remain in progress. |
| Blocker/dependency editing | In progress | Same-workspace storage and cycle checks exist, and related-task MCP can report stored relations. The picker and blocker properties are currently Office UI; regular Kanban has no complete create/edit/filter path. |
| Multi-repository tasks | Limited | New Task supports multiple local or remote GitHub rows, and Changes/Review/PR surfaces group by repository. Two or more repositories currently require the Worktree executor. |
| Additional task branches | Limited | `add_branch_to_task_kandev` can attach another branch from the same or another repository only with a Worktree task. There is no equivalent regular after-start UI. Changing Kandev's comparison base does not retarget commits or an existing pull request. |
| Workflow YAML import/export | Supported | Available for regular Kanban workflows. Office workflows are deliberately excluded. |
| GitHub-backed workflow sync | Dependency-bound | Requires GitHub access. Synced workflows are read-only in Kandev and guarded updates can need local agent/executor mapping. |
| Coordinator-led teams, participants/quorum, arbitrary task trees, routines, budgets, and heartbeats | In progress | These belong to Office. Office is disabled in the production profile and remains an evolving, feature-flagged autonomy layer. |

## Sessions, review, and developer tools

| Capability | Status | Current boundary |
|---|---|---|
| Structured chat, tool activity, files, integrated PTYs, and session lifecycle | Dependency-bound | Panel availability follows the task environment, agent protocol, executor, and viewport. A terminal needs a materialized task environment. |
| Changes and cumulative Review | Supported | Supports staged, unstaged, commit, and multi-repository views; persisted reviewed hashes, stale detection, and anchored comments are implemented. Discard is destructive and permanent. |
| Agent-authored code walkthrough | Dependency-bound | One saved walkthrough per task is supported through task MCP. A new walkthrough replaces the old one; anchors can drift, and the backend does not prove that referenced file content is still current. |
| GitHub pull-request review, checks, merge, and CI-fix automation | Dependency-bound | Requires GitHub credentials and repository permissions. CI automation is opt-in. Azure has a create-PR path only; other Git hosts do not get the full task PR panel. |
| Share a session snapshot as a secret GitHub Gist | Dependency-bound | Requires GitHub auth and a structured session outside `CREATED`/`STARTING`. The viewing URL is rendered through `gist.githack.com`; unlisted URLs and heuristic redaction are not access control or exhaustive secret detection. Payloads are capped and shares can be revoked. |
| Quick Chat | Dependency-bound | Requires an agent profile. Repository-backed chats use isolated Kandev worktrees; closing deletes the ephemeral task permanently. Chats older than seven days are swept at startup and every 24 hours unless their session is `RUNNING` or `IDLE`. |
| Utility agents and custom one-shot helpers | Dependency-bound | Require an ACP inference-capable agent and usable model. Passthrough-only agents do not satisfy this path. |
| Configuration Chat | Limited | Each workspace currently presents one repository-less configuration conversation. Settings shows it in a tabless floating panel and can move the same session into Quick Chat. Confirmed tab closure deletes its backing task; config-mode tasks are excluded from Quick Chat expiry. |
| Voice input | Dependency-bound | Browser Web Speech, a browser-downloaded Whisper model, and server Whisper paths exist. Selection does not retry another engine after a recognition error. Server Whisper requires `KANDEV_VOICE_OPENAI_API_KEY` (empty by default); without it the transcription route returns 503. |
| Embedded VS Code | Limited | code-server runs inside the task runtime with `--auth none` on `0.0.0.0:<random-port>`. It relies on task-runtime and network isolation rather than its own login. |
| Built-in editors and host editor launch | Dependency-bound | Built-in editing is shipped. External editors depend on host discovery/custom commands. A multi-worktree session presents a repository/worktree picker before host launch; clients that omit the worktree ID retain a first-worktree fallback. A remote executor path may not work without remote-editor configuration. |
| Language servers | Limited | Current built-in mapping covers TypeScript/JavaScript, Go, Rust, and Python. Auto-start/installation default off, and subprocesses run on the backend host rather than inside Docker, SSH, or Sprites runtimes. |
| Integrated shell terminals | Dependency-bound | Create, reopen, rename, and terminate are shipped. Shell settings affect new terminals, and executor/task-environment availability controls where the PTY runs. |
| Prompts, keyboard shortcuts, notifications, appearance, and task actions | Supported | These are normal settings surfaces. Browser notification permission, custom command validity, and local shell/editor availability still apply to their effects. |

## Agents, executors, integrations, and automation

| Capability | Status | Current boundary |
|---|---|---|
| Built-in ACP agent registry and profiles | Dependency-bound | The registry supports many CLIs, but installation, authentication, models, modes, configuration options, resume, and usage reporting come from the installed CLI and account. |
| Bring-your-own TUI/passthrough agent | Limited | Native PTY interaction is supported. Passthrough does not provide full structured chat, task MCP, resumable ACP state, usage, plans, utility-agent execution, or workspace automation compatibility. |
| Profile permissions, CLI flags, literal environment values, and encrypted secret references | Supported | Secret references resolve at launch; a missing/deleted reference omits that variable. Auto-approve and permission-skipping settings enlarge the process trust boundary. |
| Local and Worktree executors | Supported | Both are seeded. They execute on the Kandev host; a worktree isolates Git checkouts, not processes, network, credentials, or the rest of the filesystem. |
| Local Docker and Sprites executors | Dependency-bound | Require the relevant daemon or provider, compatible host/image, token, platform, and credential-copy configuration. |
| SSH executor | Limited | Remote sessions support trusted Linux or macOS hosts on amd64 or arm64, with public-key authentication, SFTP, and TCP forwarding. Kandev does not currently clone/materialize attached repositories there, ignores profile prepare/cleanup scripts, and retains the remote task directory for manual cleanup. |
| Remote Docker executor | In progress | The type and legacy/direct settings fields are registered, but runtime create/stop return `not yet implemented`, and the current **Settings > Executors** hub does not offer it. |
| GitHub integration | Dependency-bound | Each workspace selects one PAT, named `gh` account, or App registration plus installation for automation and may add a personal App identity. Registrations can be reused deliberately or separated for independent root credentials and bot identities. Managed task access uses repository-bound broker leases; App tokens are minted for that repository, while redeemed PAT/CLI bearer tokens retain their provider scopes. Explicit profile `GITHUB_TOKEN`/`GH_TOKEN` values remain unmanaged overrides. |
| Azure DevOps integration | Limited | Azure DevOps Services supports workspace PAT configuration, read-only work-item and pull-request browsing, feedback, task launch, and durable task PR associations. Azure DevOps Server/TFS, Entra OAuth, writes, watches, and webhooks are not supported; neither `gh` nor `az` is required. |
| GitLab integration | Limited | Each workspace can connect one `gitlab.com` or self-managed origin, browse and launch from issues/MRs, link tasks to MRs, run review/issue watches, review and act on linked MRs, assign reviewers, toggle MR notifications, and create/auto-link MRs. Provider permissions and exact host/project identity apply. Browse project narrowing is current-page-only, watches inspect at most their first 50 matches, GitLab has no team review-request equivalent, and issue notification subscription has no UI control. |
| Jira integration | Limited | Search/browse, supported transitions, launch presets, task launch, and watches require provider credentials. Launch copies data rather than creating a durable association; each watch poll reads only the first 50 JQL matches. |
| Linear integration | Limited | Search/browse, state changes, task launch, and watches require provider credentials. Launch has no preset/durable association; a watch reads 50 matches in provider order or at most 250 with explicit local sorting. |
| Sentry integration | Limited | Multi-instance configuration, issue watches, and current-task issue actions are shipped; there is no top-level browser. Watches read only the newest first page, and the status picker allows multiple selections while the backend accepts at most one. |
| Slack integration | Experimental | Uses a nonstandard browser-session token/cookie and polling rather than OAuth or a bot installation. A first scan can process up to 30 existing matches, and reactions/replies are best-effort. |
| Integration **Enabled** switches | Limited | Jira/Linear/Sentry/Slack switches are browser-local presentation state; they do not stop backend pollers. Pause/delete watches or remove the Slack/provider configuration when polling must stop. |
| Manual, preset/`@every` schedule, authenticated webhook, and GitHub PR-open automations | Limited | Task and Run modes are shipped, but Run cannot answer permission prompts. PR triggers need explicit repositories and ignore the saved Opened checkbox. Webhooks silently retain only the first 1 MiB. **Play** remains clickable while disabled but produces no history. |
| Fixed cron timezone behavior | Limited | The UI accepts fixed cron and timezone values, but the current interval scheduler does not honor the configured timezone and some fixed cron expressions may never fire. Prefer tested presets or `@every` intervals. |
| GitHub push and CI automation triggers | In progress | Trigger types and UI labels exist as coming-soon/stub paths; they are not working production triggers. GitHub's separate opt-in PR CI-fix automation is shipped. |
| Automation Run mode | Limited | Creates a hidden ephemeral task and starts immediately. It cannot require user input. Automatic worktree/branch reaping applies only when a Worktree runtime ID is present; other executors follow their own lifecycle. |

## MCP, clients, and operation

| Capability | Status | Current boundary |
|---|---|---|
| Task-scoped Kandev MCP | Dependency-bound | Injected into compatible task sessions with scoped task/session, plan, walkthrough, branch, workflow-step, and coordination tools. Available tools vary by MCP mode and task/session context. |
| Profile-defined external MCP servers | Limited | Stdio, HTTP, SSE, and streamable HTTP are implemented, but current launch resolution starts from an allow-all baseline and overlays only explicit executor policy. Blank remote policies therefore allow transports instead of applying the separate deny-all remote default. A stdio process is per-session and cannot be shared like a network server. |
| External Kandev MCP endpoint | Supported | **Settings > External MCP** exposes streamable HTTP at `/mcp` and SSE at `/mcp/sse` for configuration and task management. It has no Kandev user authentication; keep it on loopback/VPN or place it behind an authenticated proxy. It intentionally omits `stop_task_kandev` along with live-session question, plan, walkthrough, spawn, and message tools. |
| External MCP tool preview | Limited | The endpoint registration is authoritative. The current settings preview omits several registered tools and should not be used as a complete allow-list. |
| Office MCP and autonomous coordination | In progress | Office-only document, team, routine, and coordinator behavior is feature-gated and is not part of the supported external or regular task MCP contract. |
| CLI/browser runtime, Linux/macOS service, Docker deployment, and remote hosting | Dependency-bound | Platform, filesystem, browser, systemd/launchd, container, reverse-proxy, and network configuration determine availability. The web app, API, WebSocket, and external MCP add no Kandev user-login boundary; protect the whole remotely reachable origin. |
| Native desktop app | Dependency-bound | macOS, Windows, and Linux artifacts and a Tauri shell are implemented. Signing trust and update behavior differ by platform and release artifact. |
| Kubernetes deployment guide | Experimental | The guide describes one deployment approach; Kandev does not ship a Kubernetes executor or operator. |
| Statistics and host resource metrics | Supported | Workspace stats and on-demand CPU/memory/disk display are shipped. Execution-environment metrics depend on executor/runtime support. |
| SQLite backups, database maintenance, status, and logs | Supported | Backups use SQLite `VACUUM INTO` snapshots under the data root. Retention, free space, file permissions, and off-host copies remain operator responsibilities. |
| Update checking and applying updates | Dependency-bound | Checking is shipped. Package-manager installs update through their package manager; backend self-update is service-install-specific; desktop updater support varies by artifact/platform. |
| Feature Toggle settings | Supported | The page exposes Office, Plugins, and Debug overrides, environment locks, and restart-required state. Debug is high risk because logs/endpoints can expose prompt, file, and tool data. A toggle being present does not promote its target feature to supported. |
| Office mode | In progress | Production defaults `KANDEV_FEATURES_OFFICE=false`; development and E2E enable it for implementation/testing. Its routes, agents, labels, documents, dependencies, routines, skills, routing, costs, and approvals may change between releases. |
| Plugin system | In progress | Production defaults `KANDEV_FEATURES_PLUGINS=false`; development and E2E enable it for implementation/testing. Loaded plugin code runs with backend privileges; manifest schema, capabilities, and host API may change between releases. |
| Mock providers and E2E-only routes | Internal | They are selected only by development/test runtime profiles and must not be exposed as product integrations. |

## Publication boundary

Published user documentation lives in `docs/public/**`. Files elsewhere under `docs/**` can be proposed ADRs, draft specifications, implementation plans, test notes, or historical decisions. Their presence does not change a row above.

Before relying on a dependency-bound, limited, or experimental capability:

1. record the running Kandev version and platform;
2. inspect its current settings screen and health state;
3. test the smallest representative workflow with non-production credentials;
4. verify failure, retry, permission, review, and cleanup behavior;
5. keep a human gate until the workflow has proved safe for that repository and executor.

When reporting disagreement, include the Kandev version, platform, executor and profile, provider, reproduction steps, expected behavior, logs, and screenshots without secrets.

Related guides: [Tasks and workflows](tasks-and-workflows.md), [Coordination](coordination.md), [Sessions and review](sessions-and-review.md), [Developer tools](developer-tools.md), [Agents and profiles](agents-and-profiles.md), [Executors](executors.md), [Integrations](integrations.md), [Automation and MCP](automation-and-mcp.md), and [Operations](operations.md).
