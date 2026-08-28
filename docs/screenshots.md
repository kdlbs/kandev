# Screenshots

## Table of contents

- [Core workflow](#core-workflow)
- [Automations](#automations)
- [Plugins](#plugins)
- [Workspace & agent settings](#workspace--agent-settings)
- [Mobile](#mobile)
- [Preferences](#preferences)
- [System settings](#system-settings)

## Core workflow

<table>
  <tr>
    <td align="center"><strong>Kanban</strong><br><img src="screenshots/kanban.png" alt="Kanban"></td>
    <td align="center"><strong>Add Task</strong><br><img src="screenshots/add task.png" alt="Add Task"></td>
  </tr>
  <tr>
    <td align="center"><strong>Task Session</strong><br><img src="screenshots/task session.png" alt="Task Session"></td>
    <td align="center"><strong>Plan Mode</strong><br><img src="screenshots/plan mode.png" alt="Plan Mode"></td>
  </tr>
  <tr>
    <td align="center"><strong>Workflows</strong><br><img src="screenshots/workflows.png" alt="Workflows"></td>
    <td align="center"><strong>Pipeline</strong><br><img src="screenshots/pipeline.png" alt="Pipeline"></td>
  </tr>
  <tr>
    <td align="center"><strong>Workflow Details</strong><br><img src="screenshots/workflow details.png" alt="Workflow Details"></td>
    <td align="center"><strong>File Editor</strong><br><img src="screenshots/file editor.png" alt="File Editor"></td>
  </tr>
  <tr>
    <td align="center"><strong>Embedded VS Code</strong><br><img src="screenshots/embedded vscode.png" alt="Embedded VS Code"></td>
    <td align="center"><strong>Git Operations</strong><br><img src="screenshots/git operations.png" alt="Git Operations"></td>
  </tr>
  <tr>
    <td align="center"><strong>Review PRs</strong><br><img src="screenshots/review PRs.png" alt="Review PRs"></td>
    <td align="center"><strong>Review Dialog</strong><br><img src="screenshots/review dialog.png" alt="Review Dialog"></td>
  </tr>
  <tr>
    <td align="center" valign="top">
      <strong>PR auto-fix</strong><br>
      Automate CI fixes, review-comment follow-up, and optional auto-merge or requeue when ready.<br><br>
      <img src="screenshots/pr-auto-fix.png" alt="PR automation panel with auto-fix and auto-merge enabled" width="520">
    </td>
    <td align="center" valign="top">
      <strong>Agent updates and version notifications</strong><br>
      Get a visible prompt when a managed agent runtime has a newer version.<br><br>
      <img src="screenshots/agent-updates.png" alt="Agent update notification showing a newer Codey version" width="520">
    </td>
  </tr>
  <tr>
    <td align="center"><strong>Plan Comments</strong><br><img src="screenshots/plan comments.png" alt="Plan Comments"></td>
    <td align="center"><strong>CLI Agent</strong><br><img src="screenshots/cli agent.png" alt="CLI Agent"></td>
  </tr>
  <tr>
    <td align="center"><strong>Quick Chats</strong><br><img src="screenshots/quick chats.png" alt="Quick Chats"></td>
    <td align="center" valign="top">
      <strong>Stats</strong><br>
      Review task, session, Git activity, provider, GitHub, workload, and telemetry metrics.<br><br>
      <img src="screenshots/stats-overview.png" alt="Statistics overview showing tasks, time spent, Git activity, signal, telemetry, and top models" width="520"><br><br>
      <img src="screenshots/stats-github-workload.png" alt="Statistics dashboard showing GitHub activity and workload metrics" width="520">
    </td>
  </tr>
</table>

## Automations

<table>
  <tr>
    <td align="center" valign="top" width="50%">
      <strong>Workspace automations</strong><br>
      Review scheduled and webhook automations, their enabled state, and recent activity.<br><br>
      <img src="screenshots/automation-list.png" alt="Workspace automations showing scheduled and webhook triggers" width="520">
    </td>
    <td align="center" valign="top" width="50%">
      <strong>Automation editor</strong><br>
      Configure repository access, run destination, context, concurrency, and save changes.<br><br>
      <img src="screenshots/automation-editor.png" alt="Automation editor showing repository access, run destination, context, and save controls" width="520">
    </td>
  </tr>
  <tr>
    <td colspan="2" align="center">
      <strong>Automation sidebar</strong><br>
      Open workspace automations from the main navigation.<br><br>
      <img src="screenshots/automation-sidebar.png" alt="Kandev sidebar showing workspace automations" width="360">
    </td>
  </tr>
</table>

## Plugins

Kandev plugins and companion tools add focused views for exploring MCP servers,
monitoring service health, and tracking usage.

<table>
  <tr>
    <td align="center" valign="top" width="50%">
      <strong>Plugin marketplace</strong><br>
      Browse, filter, and install plugins from the in-app catalog.<br><br>
      <img src="screenshots/plugin-browse.png" alt="Plugin marketplace showing available plugins and filters" width="520">
    </td>
    <td align="center" valign="top" width="50%">
      <strong>Plugin settings</strong><br>
      Manage installed plugins, updates, activation, and configuration.<br><br>
      <img src="screenshots/plugin-settings.png" alt="Plugin settings showing installed plugins and update controls" width="520">
    </td>
  </tr>
  <tr>
    <td align="center" valign="top" width="50%">
      <strong>MCP Explorer</strong><br>
      Inspect connected servers, available tools, and estimated token costs.<br><br>
      <img src="screenshots/plugin-mcp-explorer.png" alt="MCP Explorer showing an active server and available tools" width="520">
    </td>
    <td align="center" valign="top" width="50%">
      <strong>GitHub Status</strong> (<a href="https://github.com/kdlbs/kandev-plugin-github-status">plugin repository</a>)<br>
      See GitHub health at a glance, then open detailed component and incident status.<br><br>
      <img src="screenshots/plugin-github-status.png" alt="GitHub Status showing service health and an active incident" width="520"><br><br>
      <img src="screenshots/plugin-github-status-bar.png" alt="GitHub Status indicator in the Kandev status bar" width="520">
    </td>
  </tr>
  <tr>
    <td align="center" valign="top">
      <strong>Provider Usage</strong> (<a href="https://github.com/kdlbs/kandev-plugin-provider-usage">plugin repository</a>)<br>
      Monitor provider quota usage and reset windows from the status bar.<br><br>
      <img src="screenshots/plugin-provider-usage.png" alt="Provider Usage panel showing Codex usage and reset windows" width="360">
    </td>
    <td align="center" valign="top">
      <strong>Session Cost</strong> (<a href="https://github.com/kdlbs/kandev-plugin-session-cost">plugin repository</a>)<br>
      Track session spend, turns, token totals, cache reads, and per-model costs.<br><br>
      <img src="screenshots/plugin-session-cost.png" alt="Session Cost panel showing spend, turns, token totals, and per-model costs" width="520">
    </td>
  </tr>
  <tr>
    <td colspan="2" align="center">
      <strong>Kandy</strong> (<a href="https://github.com/kdlbs/kandev-plugin-kandy">plugin repository</a>)<br>
      Add playful progress feedback with levels, experience, status, and celebrations.<br><br>
      <img src="screenshots/plugin-kandy.png" alt="Kandy progress card showing level, experience, and status" width="360">
    </td>
  </tr>
</table>

## Workspace & agent settings

<table>
  <tr>
    <td align="center" valign="top" width="50%">
      <strong>Integrations</strong><br>
      Connect a workspace to GitHub, GitLab, Jira, Linear, Sentry, and Azure DevOps.<br><br>
      <img src="screenshots/settings-integrations.png" alt="Integrations settings showing provider connection cards" width="520">
    </td>
    <td align="center" valign="top" width="50%">
      <strong>Agents</strong><br>
      Review detected agent CLIs, profiles, availability, and runtime updates.<br><br>
      <img src="screenshots/settings-agents.png" alt="Agents settings showing detected agent CLIs, profiles, and update indicators" width="520">
    </td>
  </tr>
  <tr>
    <td colspan="2" align="center">
      <strong>Executors</strong><br>
      Configure local, worktree, Docker, Sprites.dev, and SSH execution profiles.<br><br>
      <img src="screenshots/settings-executors.png" alt="Executors settings showing existing and available executor profiles" width="760">
    </td>
  </tr>
</table>

## Mobile

<table>
  <tr>
    <td align="center"><strong>Mobile Chat</strong><br><img src="screenshots/mobile-chat.webp" alt="Mobile Chat" width="240"></td>
    <td align="center"><strong>Mobile Changes</strong><br><img src="screenshots/mobile-changes.webp" alt="Mobile Changes" width="240"></td>
    <td align="center"><strong>Mobile Diff Review</strong><br><img src="screenshots/mobile-diff.webp" alt="Mobile Diff Review" width="240"></td>
  </tr>
</table>

## Preferences

<table>
  <tr>
    <td colspan="2" align="center">
      <strong>Layouts</strong><br>
      Configure reusable desktop workbench profiles and panel arrangements.<br><br>
      <img src="screenshots/settings-layouts.png" alt="Layouts settings showing built-in desktop workbench profiles" width="760">
    </td>
  </tr>
  <tr>
    <td align="center" valign="top" width="50%">
      <strong>Language servers</strong><br>
      Configure language-server startup, installation, status location, and server settings.<br><br>
      <img src="screenshots/settings-terminal-editors.png" alt="Terminal and Editors settings showing language server controls" width="520">
    </td>
    <td align="center" valign="top" width="50%">
      <strong>Task Behavior</strong><br>
      Configure task titles, archive confirmation, unread messages, and host sleep prevention.<br><br>
      <img src="screenshots/settings-task-behavior.png" alt="Task Behavior settings showing task title, archive, unread message, and host sleep controls" width="520">
    </td>
  </tr>
  <tr>
    <td align="center" valign="top" width="50%">
      <strong>Keyboard Shortcuts</strong><br>
      Customize chat input and command-panel keybindings.<br><br>
      <img src="screenshots/settings-keyboard-shortcuts.png" alt="Keyboard Shortcuts settings showing chat input and command panel bindings" width="520">
    </td>
    <td align="center" valign="top" width="50%">
      <strong>Appearance</strong><br>
      Configure status-bar visibility and resource metrics for the host and execution environments.<br><br>
      <img src="screenshots/settings-appearance.png" alt="Appearance settings showing status bar and resource metrics controls" width="520">
    </td>
  </tr>
  <tr>
    <td colspan="2" align="center">
      <strong>Shell and terminal</strong><br>
      Choose the default shell, terminal font, font size, and link behavior.<br><br>
      <img src="screenshots/settings-terminal.png" alt="Terminal and Editors settings showing shell, terminal font, font size, and link controls" width="760">
    </td>
  </tr>
</table>

## System settings

<table>
  <tr>
    <td align="center" valign="top" width="50%">
      <strong>Status</strong><br>
      Check system health, the running version, and the Kandev data directory.<br><br>
      <img src="screenshots/system-status.png" alt="Status settings showing health, version, and disk usage" width="520">
    </td>
    <td align="center" valign="top" width="50%">
      <strong>Storage</strong><br>
      Review disk capacity, Kandev-managed usage, and reclaimable space.<br><br>
      <img src="screenshots/system-storage.png" alt="Storage settings showing disk capacity and storage analysis" width="520">
    </td>
  </tr>
  <tr>
    <td align="center" valign="top" width="50%">
      <strong>Maintenance policy</strong><br>
      Choose the schedule, idle window, workspace cleanup, and folder allowlist.<br><br>
      <img src="screenshots/system-maintenance-policy.png" alt="Maintenance policy settings showing schedule and workspace cleanup controls" width="520">
    </td>
    <td align="center" valign="top" width="50%">
      <strong>Docker cleanup</strong><br>
      Configure dedicated-daemon protection, build-cache retention, and unused image cleanup.<br><br>
      <img src="screenshots/system-docker-cleanup.png" alt="Docker cleanup settings showing cache and unused image cleanup controls" width="520">
    </td>
  </tr>
  <tr>
    <td colspan="2" align="center">
      <strong>Quarantine</strong><br>
      Restore or permanently remove recoverable resources after cleanup.<br><br>
      <img src="screenshots/system-quarantine.png" alt="Quarantine settings showing a recoverable resource with restore and delete controls" width="760">
    </td>
  </tr>
</table>
