---
title: "Get Started"
description: "Install Kandev, add a repository, configure an agent, run a first task, and diagnose startup failures."
---

# Get Started with Kandev

Use this guide to run one agent on one local Git repository. Start with the built-in Worktree profile so the agent works in a separate checkout.

## Quick path

1. Install and start Kandev.
2. Add a local repository and review its scripts and trust boundary.
3. Configure an agent profile and keep Worktree selected for the first task.
4. Start, inspect, test, and review the task before archiving or opening a pull request.

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

# Scoop on Windows
scoop bucket add kandev https://github.com/kdlbs/scoop-kandev
scoop install kandev
kandev

# One-off npm launch
npx kandev@latest

# Global npm installation
npm install -g kandev@latest
kandev
```

Scoop installs the native runtime bundle, so Node.js is not required to install or start Kandev. It
is still needed for the agent CLIs Kandev installs through its own interface.

Stable is the default and is selected by npm's `latest` tag. To test the current prerelease from
`main` without changing a global installation, launch the package once from the npm-only `nightly`
tag:

```bash
npx -y kandev@nightly
```

To replace a global Stable installation with Nightly, use:

```bash
npm install -g kandev@nightly
kandev
```

The global command replaces the installed `kandev` channel. Return to Stable with
`npm install -g kandev@latest`.

Nightly builds may be unstable. They are best-effort daily snapshots, and a new version appears
only when `main` has commits after the latest Stable release. Homebrew, Scoop, Desktop, and
container installs have no Nightly channel.

The launcher selects the platform runtime, starts the Go backend and agent runtime, serves the web app, and opens its local URL. The preferred backend port is `38429`; if it is unavailable, the launcher chooses another free port and prints the actual URL. Use `kandev --headless` (or `KANDEV_NO_BROWSER=1`) when a browser must not open.

By default, persistent state is under `~/.kandev`, including the SQLite database, repository materializations, sessions, logs, and backups. `KANDEV_HOME_DIR` relocates that root. `KANDEV_DATABASE_PATH` overrides the database file and moves System snapshots to `backups/` beside that file. Kandev creates its data directory with owner-only permissions and rejects symlinked components in that path, but file permissions do not replace host access controls. Kandev does not move snapshots from another directory automatically.

See the [CLI reference](cli.md) for commands, port and logging flags, data paths, environment variables, and update behavior. Other supported entry points are the [desktop app](desktop-app.md), [service](run-as-a-service.md), and [Docker deployment](docker.md). Follow [Mobile Remote Access](mobile-remote-access.md) for a private phone connection. Read [Security and trust](security.md) before making any installation remotely reachable.

<details>
<summary>First-run records and onboarding details</summary>

## Know what a fresh database contains

Kandev creates these records when no prior workspace or executor configuration exists:

- **Default Workspace**.
- **Development**, materialized from the built-in Kanban (`simple`) workflow template.
- System **Local** and **Worktree** executors, plus matching Local and Worktree executor profiles.
- A Local Docker executor configured with the platform's default Docker host. The record is created even when no usable Docker daemon is available; task launch still requires one.
- A disabled Sprites executor entry.

The first-run dialog scans supported agent CLIs, lets you inspect detected profiles, and introduces executors, workflows, and the command panel. **Skip** stores only a browser-local onboarding marker. Advancing/completing also saves any dirty agent-profile edits made in the dialog. Neither path creates another workspace. The dialog warns that default agent profiles can have **Auto Approve** enabled. Inspect every profile before assigning trusted code or credentials.

</details>

## Find a setting

Open **Settings** and use **Search settings** at the top of its navigation tree. Search results are
grouped by area; selecting a specific control opens its owning page, scrolls it into view, focuses
it, and briefly highlights it without changing the saved value. The same search is available in
the Settings menu on phones.

From anywhere, press `Cmd/Ctrl+K` and begin typing a setting name or familiar alias. Individual
settings appear only after typing, while **Go to Settings** remains in the command menu at rest.
Discovery searches setting names and curated aliases, never saved values, secrets, paths, or other
configuration content.

## Switch workspace

The workspace picker sits in the sidebar header, next to the Kandev brand. It lists every
workspace with a **Kanban** or **Office** badge and ends with the workspace-creation actions.

From anywhere, press `Cmd/Ctrl+E` to open it with the keyboard: focus lands inside the menu,
so arrow keys move between workspaces and **Enter** switches to the highlighted one. If the sidebar
is collapsed, the shortcut expands it first. Rebind it under
**Settings > General > Keyboard Shortcuts** (**Open Workspace Picker**) if it clashes with a
browser or window-manager binding.

The shortcut does nothing on narrow windows and phones, where the sidebar is hidden. Switch
workspace there from the menu sheet instead.

## Add a local repository

1. Open **Settings > Workspaces > Default Workspace > Repositories**. If you created or renamed the workspace, choose that workspace instead.
2. Select **Add Local Repository**.
3. Choose a discovered repository, or enter an absolute path and select **Validate**. The backend accepts any existing Git repository the Kandev process can access. Configured discovery roots bound automatic scans; they do not restrict an explicit path.
4. Select **Use Repository**. This opens an unsaved repository card.
5. Review the repository name, worktree branch template, pull behavior, setup/cleanup/dev scripts, copied files, and custom commands. Then select **Save changes**.

New local repository records default to:

- source type `local`;
- legacy worktree branch prefix `feature/`;
- worktree branch template `feature/{title}-{suffix}`;
- pull-before-worktree enabled;
- empty setup, cleanup, dev, copied-file, and custom-script configuration.

Scripts execute in agent workspaces and therefore belong to the trust boundary. Do not add an unreviewed command or copy pattern. Repository deletion is irreversible in the UI and is blocked while an active task session still references the repository.

Repositories saved by an older Kandev version may still contain a path spelling with symbolic-link components. If branch operations report that such a saved path resolves to a different location after upgrading, edit and save the repository path again to record its current canonical location. Kandev does not silently accept the new resolution because that would also accept a saved path whose symbolic-link target was changed after registration.

Remote repository and issue/PR URLs are not added from this settings page. Use the **Remote** tab in **New Task** to search configured GitHub, GitLab, and Azure DevOps repositories, or paste a supported provider URL. See [Integrations](integrations.md) and [Tasks and workflows](tasks-and-workflows.md).

## Configure an agent profile

1. Open **Settings > Agents**.
2. Select **Rescan** if an installed CLI is missing. An available agent appears with its detected installation and authentication state.
3. If necessary, select **Install**. Kandev runs that agent's advertised install script on the Kandev host and shows live output. You can instead install it yourself and rescan.
4. Complete the provider's authentication flow. Some agents expose a dedicated login terminal; others use their own CLI outside Kandev.
5. Open the agent and its profile. Verify the selected model and mode, permission switches, CLI flags, environment variables, MCP configuration, and CLI-passthrough setting. Save changes.

Detected capabilities come from the installed CLI and can change after an agent upgrade or login. A model or mode shown in documentation is not guaranteed for every account. If no built-in adapter fits, **Add TUI Agent** creates a passthrough integration; passthrough has a different resume, usage, and MCP contract from an ACP-capable agent.

For the first task on an existing repository, keep the seeded **Worktree** executor profile. It creates a separate Git checkout so concurrent Kandev tasks do not edit the same working tree. A worktree is Git isolation, not operating-system isolation. Choose **Local** only when direct edits in the selected checkout are intentional. A repository initialized from **New Task** already has one empty initial commit and no project files, and Kandev selects a direct Local profile for that flow. See [Agents and profiles](agents-and-profiles.md) and [Executors](executors.md) before using Docker, SSH, Sprites, custom scripts, or shared infrastructure.

## Create and start the first task

1. Select **New Task** from the sidebar or task board.
2. Enter a specific title and a description with the expected outcome, constraints, and validation. A non-empty description enables the normal start and create-only actions.
3. Under **Repo**, select the workspace repository and base branch. To start a new project, open the repository selector, choose **Create new repository**, enter one folder name, and select its parent folder. Kandev creates an empty `main` repository with one empty initial commit and no project files. Select **None** only for work that genuinely needs a plain workspace directory.
4. Choose the Development workflow and an agent profile. For an existing repository, choose the Worktree executor profile. For a newly initialized repository, keep the Local profile that Kandev selects automatically. Kandev remembers compatible recent choices, so re-check them after changing repository, agent, or trust boundary.
5. Select **Start task**. Its menu also offers **Start task in plan mode** and **Create without starting agent**. On an empty description, the primary action is **Start Plan Mode**.

Starting creates the task and its initial session. Workflow step configuration determines whether entry actions or later transitions start another session, inject a prompt, or stop at a human gate. With a structured ACP profile, create-only prepares the session/workspace without starting an agent turn. A passthrough/TUI profile is the exception: the backend upgrades prepare-only to a full launch because the native PTY must exist.

### Start from an empty remote

When the selected repository's remote has no refs, Kandev creates an empty local baseline during task preparation so the agent can use the normal Worktree executor. It adds no project files and does not publish the baseline during launch, resume, or worktree recovery.

After the agent creates and commits work, open **Changes** and choose **Push** or **Create pull request**. Kandev publishes the selected base branch before the task branch and uses the task runtime's Git credentials. Read or clone access alone does not authorize this first publication. Provider change-request creation starts only after both branches are available on the remote.

If another actor initializes the remote first, Kandev stops without replacing that history. Reconcile the remote with the task branch before retrying. If the base was published but the task branch was not, retry **Push**; Kandev keeps the local task branch. The same flow is available from the touch-sized **Changes** menu on phones.

The dialog also supports multiple repositories, remote GitHub rows, and a single-repository **Fork a new branch** option when the Local executor is selected. Important boundaries:

- Multi-repository tasks require the Worktree executor in the current task-create path.
- Creating an empty repository is available only for a single repository row and switches the task to a direct Local profile.
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
- Managed GitHub task access (the default) uses task/repository-bound broker leases from the workspace automation connection. GitHub App tokens are minted for the redeemed repository; PAT and named-CLI bearer tokens retain all provider-granted scopes once delivered to the trusted agent subprocess. Personal tokens and the App private key never enter executors. Choose **Inherit executor Git credentials** in the workspace GitHub settings only when task Git and `gh` should use host-visible credentials for Local/Worktree or intentionally configured credentials for remote executors.
- A profile-supplied `GITHUB_TOKEN` or `GH_TOKEN` is an explicit unmanaged override and bypasses workspace broker selection. Ambient backend tokens and the host-active `gh` account are used only by migration-only **Legacy shared** workspace connections.
- Repository, executor, and task action scripts are executable configuration. Review them like code.
- Authentication is opt-in. When it is disabled, treat anyone who can reach the backend as an operator. Keep the whole origin on loopback or a trusted network, or put it behind an authenticated TLS proxy.

Use the [Security and trust](security.md) guide to choose a local, remote, shared-team, or unattended-automation boundary and to review the deployment checklist.

<details>
<summary>Troubleshoot a task that will not start</summary>

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
- If repository validation fails, use an absolute path to an existing, accessible Git working tree. Discovery roots control automatic scans only; they do not restrict an explicit path.
- If a worktree cannot be prepared, resolve dirty/conflicting branch state, remote authentication, pull errors, setup-script failures, and disk exhaustion before retrying. If another actor initialized an empty remote, reconcile that history with the local task branch first.
- If the browser did not open, use the URL printed by the launcher or start with `--headless`; do not assume port `38429` when the launcher selected a fallback.

For backup, update, log, database, and recovery details, see [Operations](operations.md). For release-specific disagreement, record the version from **Settings > System > About** and compare it with the matching GitHub tag.

</details>
