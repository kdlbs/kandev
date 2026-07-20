---
title: "Get Started"
description: "Install Kandev, add a repository, configure an agent, run a first task, and diagnose startup failures."
---

# Get Started with Kandev

This guide establishes the smallest useful Kandev setup: one local Git repository, one coding-agent profile, the built-in Worktree executor profile, and the default Development workflow.

## Prerequisites

- Git and filesystem access to the repository.
- Credentials for at least one supported coding-agent CLI. Kandev can run an advertised installer on the Kandev host, but it does not create the provider account or subscription.
- npm 7 or newer only for the npm/`npx` distribution.
- Docker only for Docker executor profiles. SSH and Sprites profiles have their own host or token requirements.

Kandev runs the selected agent with the access exposed by its executor, profile, environment, MCP policy, and credentials. Use a test repository and scoped credentials while learning the product.

## Install and start

Choose one release channel:

```bash
# Homebrew on macOS or Linux
brew install kdlbs/kandev/kandev
kandev

# One-off npm launch
npx kandev@latest

# Global npm installation
npm install -g kandev@latest
kandev
```

The launcher selects the platform runtime, starts the Go backend and agent runtime, serves the web app, and opens its local URL. The preferred backend port is `38429`; if it is unavailable, the launcher chooses another free port and prints the actual URL. Use `kandev --headless` (or `KANDEV_NO_BROWSER=1`) when a browser must not open.

By default, persistent state is under `~/.kandev`, including the SQLite database, repository materializations, sessions, logs, and backups. `KANDEV_HOME_DIR` relocates that root; `KANDEV_DATABASE_PATH` overrides only the database. Kandev creates its data directory with owner-only permissions and rejects symlinked components in that path, but file permissions do not replace host access controls.

See the [CLI reference](cli.md) for commands, port and logging flags, data paths, environment variables, and update behavior. Other supported entry points are the [desktop app](desktop-app.md), [service](run-as-a-service.md), and [Docker deployment](docker.md). Read [Security and trust](security.md) before making any installation remotely reachable.

## Know what a fresh database contains

Kandev creates these records when no prior workspace or executor configuration exists:

- **Default Workspace**.
- **Development**, materialized from the built-in Kanban (`simple`) workflow template.
- System **Local** and **Worktree** executors, plus matching Local and Worktree executor profiles.
- A Local Docker executor configured with the platform's default Docker host. The record is created even when no usable Docker daemon is available; task launch still requires one.
- A disabled Sprites executor entry.

The first-run dialog scans supported agent CLIs, lets you inspect detected profiles, and introduces executors, workflows, and the command panel. **Skip** stores only a browser-local onboarding marker. Advancing/completing also saves any dirty agent-profile edits made in the dialog. Neither path creates another workspace. The dialog warns that default agent profiles can have **Auto Approve** enabled. Inspect every profile before assigning trusted code or credentials.

## Add a local repository

1. Open **Settings > Workspaces > Default Workspace > Repositories**. If you created or renamed the workspace, choose that workspace instead.
2. Select **Add Local Repository**.
3. Choose a discovered repository, or enter an absolute path and select **Validate**. The backend accepts only an existing Git repository within its configured discovery roots (the current user's home directory by default) or Kandev's configured clone base.
4. Select **Use Repository**. This opens an unsaved repository card.
5. Review the repository name, worktree branch template, pull behavior, setup/cleanup/dev scripts, copied files, and custom commands. Then select **Save**.

New local repository records default to:

- source type `local`;
- legacy worktree branch prefix `feature/`;
- worktree branch template `feature/{title}-{suffix}`;
- pull-before-worktree enabled;
- empty setup, cleanup, dev, copied-file, and custom-script configuration.

Scripts execute in agent workspaces and therefore belong to the trust boundary. Do not add an unreviewed command or copy pattern. Repository deletion is irreversible in the UI and is blocked while an active task session still references the repository.

Remote repository and issue/PR URLs are not added from this settings page. Use the **Remote** tab in **New Task** to search configured GitHub, GitLab, and Azure DevOps repositories, or paste a supported provider URL. See [Integrations](integrations.md) and [Tasks and workflows](tasks-and-workflows.md).

## Configure an agent profile

1. Open **Settings > Agents**.
2. Select **Rescan** if an installed CLI is missing. An available agent appears with its detected installation and authentication state.
3. If necessary, select **Install**. Kandev runs that agent's advertised install script on the Kandev host and shows live output. You can instead install it yourself and rescan.
4. Complete the provider's authentication flow. Some agents expose a dedicated login terminal; others use their own CLI outside Kandev.
5. Open the agent and its profile. Verify the selected model and mode, permission switches, CLI flags, environment variables, MCP configuration, and CLI-passthrough setting. Save changes.

Detected capabilities come from the installed CLI and can change after an agent upgrade or login. A model or mode shown in documentation is not guaranteed for every account. If no built-in adapter fits, **Add TUI Agent** creates a passthrough integration; passthrough has a different resume, usage, and MCP contract from an ACP-capable agent.

For the first task, keep the seeded **Worktree** executor profile. It creates a separate Git checkout so concurrent Kandev tasks do not edit the same working tree. A worktree is Git isolation, not operating-system isolation. Choose **Local** only when direct edits in the selected checkout are intentional. See [Agents and profiles](agents-and-profiles.md) and [Executors](executors.md) before using Docker, SSH, Sprites, custom scripts, or shared infrastructure.

## Create and start the first task

1. Select **New Task** from the sidebar or task board.
2. Enter a specific title and a description with the expected outcome, constraints, and validation. A non-empty description enables the normal start and create-only actions.
3. Under **Repo**, select the workspace repository and base branch. Select **None** only for work that genuinely needs a plain workspace directory.
4. Choose the Development workflow, an agent profile, and the Worktree executor profile. Kandev remembers compatible recent choices, so re-check them after changing repository, agent, or trust boundary.
5. Select **Start task**. Its menu also offers **Start task in plan mode** and **Create without starting agent**. On an empty description, the primary action is **Start Plan Mode**.

Starting creates the task and its initial session. Workflow step configuration determines whether entry actions or later transitions start another session, inject a prompt, or stop at a human gate. With a structured ACP profile, create-only prepares the session/workspace without starting an agent turn. A passthrough/TUI profile is the exception: the backend upgrades prepare-only to a full launch because the native PTY must exist.

The dialog also supports multiple repositories, remote GitHub rows, and a single-repository **Fork a new branch** option when the Local executor is selected. Important boundaries:

- Multi-repository tasks require the Worktree executor in the current task-create path.
- No-repository tasks cannot use the Worktree executor.
- The local fork option is off by default and requires explicit consent before Kandev discards dirty source-checkout changes.
- Agent and executor choices can be disabled when their remote credential requirements are incompatible.

## Supervise and finish the task

1. Watch chat and tool activity. Answer clarification or permission requests instead of assuming a stalled process is finished.
2. Inspect terminal output, files, preview, and **Changes** while the session runs.
3. Review the cumulative diff and any walkthrough. Add anchored review comments or another prompt when corrections are needed.
4. Run the repository's required checks. Agent completion does not prove that tests passed.
5. Inspect commits and branch state. Create or associate a pull request only after provider credentials and the target base branch are correct.
6. Move the task through the workflow's human review gate. Archive it only after deciding what should happen to its branch, worktree, and external issue or pull request.

See [Sessions and review](sessions-and-review.md) for the workbench and [Tasks and workflows](tasks-and-workflows.md) for transitions, plans, workflow automation, and the current document and label limitations.

## Credentials and security boundaries

- Prefer Kandev secret references and provider-native login over plaintext environment values. Scope tokens to the required repositories and API operations.
- **Auto Approve**, permission-skipping flags, and unrestricted passthrough remove human gates inside the agent CLI. They are security decisions.
- Local and worktree sessions run on the Kandev host. Worktree separation does not limit process, filesystem, network, or credential access.
- Containers and remote executors change the boundary, but their mounted files, copied credentials, environment, daemon/socket access, and prepare scripts can reintroduce host or provider access.
- Local Docker, Sprites, and SSH launches resolve remote credentials. Unless the profile already supplies `GITHUB_TOKEN` or `GH_TOKEN`, Kandev can inject the globally stored GitHub token as both variables, with local `gh` authentication as a fallback. The registered Remote Docker path does the same credential resolution, although its runtime is not implemented. Workspace repository scope does not limit that token's provider privileges.
- Repository, executor, and task action scripts are executable configuration. Review them like code.
- The web app, HTTP API, WebSocket, and external MCP routes currently have no Kandev user-login boundary. Treat anyone who can reach the backend as an operator; keep the whole origin on loopback or a trusted network, or put it behind an authenticated TLS proxy.

Use the [Security and trust](security.md) guide to choose a local, remote, shared-team, or unattended-automation boundary and to review the deployment checklist.

## Troubleshoot a task that will not start

Check in this order:

1. **Settings > System > Status** for reported health issues, disk usage, and the running version.
2. **Settings > Agents** for CLI availability, probe errors, login state, profile model/mode, and permission configuration.
3. The repository card and branch. Run `git status` and `git fetch` as the same operating-system user that runs Kandev.
4. **Settings > Executors** for a disabled or incompatible profile and for Docker, SSH, Sprites, image, host, or prepare-script failures.
5. The task's session error and terminal output, then **Settings > System > Logs** for backend/runtime details.
6. Available disk space. Repositories, worktrees, session state, logs, and backups all consume the Kandev data root.

Common corrections:

- If an agent is absent after installation, run **Rescan** and inspect the host terminal's `PATH`.
- If a profile has no usable model or mode, authenticate the CLI, rescan, and reopen the profile. Capability probing can heal selections that an upgraded CLI removed.
- If repository validation fails, use an absolute path inside an allowed discovery root and confirm it is a Git working tree.
- If a worktree cannot be prepared, resolve dirty/conflicting branch state, remote authentication, pull errors, setup-script failures, and disk exhaustion before retrying.
- If the browser did not open, use the URL printed by the launcher or start with `--headless`; do not assume port `38429` when the launcher selected a fallback.

For backup, update, log, database, and recovery details, see [Operations](operations.md). For release-specific disagreement, record the version from **Settings > System > About** and compare it with the matching GitHub tag.
