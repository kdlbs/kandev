---
title: "Plugin Manifest Reference"
description: "Complete field-by-field reference for a kandev plugin's manifest.yaml, including the full event-subscription vocabulary."
status: experimental
---

# Plugin Manifest Reference

`manifest.yaml` is the authoritative description of a plugin — identity,
runtime executables, capabilities, webhooks, config schema, and
optional UI bundle. Kandev parses and validates it **before any plugin code
runs**. See [Authoring a plugin](plugins-authoring.md) for the build
workflow and [Plugins](plugins.md) for install/operate.

## Annotated example

```yaml
id: "kandev-plugin-slack"                    # ^[a-z0-9][a-z0-9._-]*$
api_version: 1                               # must be 1 (the only supported value)
version: "1.0.0"
display_name: "Slack Notifications"
description: "Post to Slack on task events, relay messages to agents"
author: "kandev"
categories: ["connector"]                    # connector | automation | tools | analytics

runtime:
  type: binary                               # only supported value today
  executables:
    linux-amd64: server/plugin-linux-amd64
    linux-arm64: server/plugin-linux-arm64
    darwin-amd64: server/plugin-darwin-amd64
    darwin-arm64: server/plugin-darwin-arm64
    windows-amd64: server/plugin-windows-amd64.exe
                                              # any subset; the host's <goos>-<goarch> key
                                              # is required at install time
min_kandev_version: "0.78.0"                 # optional

capabilities:
  events: ["task.created", "task.state_changed", "agent.completed"]
  api_read: ["tasks", "agent_profiles"]      # gates the Host data-reader RPCs
  api_write: ["tasks"]                       # reserved, no Host RPC enforces this yet
  state: true
  secrets: true

webhooks:
  - key: "slack-events"
    description: "Slack Events API webhook"
    method: "POST"                           # informational only, not enforced

config_schema:
  type: object
  properties:
    bot_token:  { type: string, secret: true, title: "Bot Token", description: "Slack bot OAuth token" }
    default_channel:  { type: string, description: "Default channel for notifications" }
    notify_on_task_created: { type: boolean, default: true }
  required: ["bot_token", "default_channel"]

ui:                                           # optional native frontend plugin
  bundle: "/ui/bundle.js"                    # root-relative
  styles: ["/ui/plugin.css"]                 # optional, root-relative
```

## Field reference

| Field | Required | Type | Notes |
|---|---|---|---|
| `id` | yes | string | Must match `^[a-z0-9][a-z0-9._-]*$` (lowercase alphanumeric, dots, underscores, hyphens; must start with a lowercase alphanumeric). Directory name under `~/.kandev/plugins/`. |
| `api_version` | yes | int | Must be exactly `1`. Any other value is rejected. |
| `version` | yes | string | Free-form; used as the version directory name (`~/.kandev/plugins/<id>/<version>/`). |
| `display_name` | no | string | Shown in Settings > Plugins. |
| `description` | no | string | Shown in Settings > Plugins. |
| `author` | no | string | Free-form. |
| `categories` | no | string[] | Each entry must be one of `connector`, `automation`, `tools`, `analytics`. Unknown values are rejected. |
| `icon` | no | string | Package-relative path (e.g. `icon.svg`) to an image the package ships, rendered on the [marketplace](plugins-marketplace.md) card and in plugin lists. The registry index-build resolves it to an absolute `icon_url`; for an installed plugin it is served from the extracted package. Omit it and the card falls back to a letter tile. |
| `runtime.type` | conditionally | string | `"binary"` is the only supported value. Setting it (vs. leaving it empty) makes the manifest **runtime-managed** — see "Managed vs. legacy" below. |
| `runtime.executables` | required when `runtime.type: binary` | map\<string,string\> | Key is `<goos>-<goarch>` (e.g. `linux-amd64`, `darwin-arm64`, `windows-amd64`); value is a clean, package-relative path under `server/` (no leading `/`, no `..` segments). At least one entry required; the running host's key must be present at install time. Windows values end in `.exe`. |
| `min_kandev_version` | no | string | Optional advisory; not currently enforced by the installer. |
| `capabilities.events` | no | string[] | Bus subjects (or wildcard patterns) this plugin subscribes to. See "Event subscription vocabulary" below. |
| `capabilities.api_read` | no | string[] | Gates the Host data API's read-only accessors. Each entry is a resource name: `tasks`, `sessions`, `workspaces`, `workflows`, `agent_profiles`, `repositories`. Calling the matching `Host` accessor (e.g. `Tasks()`) without its resource declared returns gRPC `PermissionDenied`. See "Host data API resource vocabulary" below. |
| `capabilities.api_write` | no | string[] | **Reserved for future Host RPCs.** Declared but not enforced by anything today — no Host RPC currently writes kandev's own data. |
| `capabilities.state` | no | bool | Gates `Host.GetState`/`SetState`/`DeleteState`/`ListState`. Calling any of them without this set to `true` returns gRPC `PermissionDenied`. |
| `capabilities.secrets` | no | bool | Gates `Host.RevealSecret`/`GetSecret`/`SetSecret`/`DeleteSecret`. Calling any of them without this set to `true` returns gRPC `PermissionDenied`. |
| `webhooks[].key` | yes | string | Must be unique within the manifest. Used in the relay path `POST /api/plugins/{id}/webhooks/{key}`. |
| `webhooks[].description` | no | string | Free-form. |
| `webhooks[].method` | no | string | **Informational only** — kandev does not validate or enforce the inbound HTTP method against this value. |
| `config_schema` | no | object | JSON-Schema-like object driving the settings form at **Settings > Plugins > `<plugin>`** (`GET /api/plugins/{id}/config` and `PATCH /api/plugins/{id}`). See "Config schema validation and secret fields" below. |
| `ui.bundle` | no | string | Root-relative path (must start with `/`, e.g. `/ui/bundle.js`) to the plugin's native UI ES module, served at `GET /api/plugins/{id}/bundle`. |
| `ui.styles` | no | string[] | Root-relative CSS paths (each must start with `/`), served at `GET /api/plugins/{id}/ui/*` and injected as `<link>` tags on load. |
| `ui.pages` | no | object[] | Optional declarative page metadata. Secondary to `ui.bundle` — a native bundle registers its own routes/nav at runtime, so most plugins omit `ui.pages`. |
| `ui.pages[].key` | yes* | string | Stable identifier for the page (*required when a page entry is present). |
| `ui.pages[].title` | yes* | string | Display title. |
| `ui.pages[].path` | yes* | string | Route path for the page. |
| `ui.pages[].surface` | yes* | string | Where the page mounts. Enum, one of: `settings` · `task-panel` · `main-nav`. Any other value is a validation error. |

`ui.pages` is declarative manifest metadata only. A native bundle's runtime
nav items, icons, and per-route title-bar chrome (`registerNavItem`,
`registerRoute`'s `options.topbar`) are a separate JS SDK surface with no
`manifest.yaml` field — see [Authoring a plugin](plugins-authoring.md).

## Managed vs. legacy manifests

Setting `runtime.type: binary` makes a manifest **runtime-managed**: kandev
spawns and supervises the declared executable itself. A managed manifest
must **not** set `base_url` or an `endpoints` block (`health`/`events`/
`webhooks` paths on a remote service) — those describe the old
remote/operator-hosted tier, and validation rejects a managed manifest that
sets them.

A manifest with an empty `runtime.type` still *parses* as a legacy remote
manifest (with `base_url`/`endpoints` instead) and passes
`manifest.Validate()` on its own — but the **installer** rejects it: `pkgtar
.Install` requires `manifest.IsManaged()` to be true (`runtime.type:
binary`), so a legacy manifest can never actually be installed via `POST
/api/plugins/install` or a filesystem sideload. The remote tier is
effectively removed in practice, even though the manifest schema still
recognizes its shape.

## Host data API resource vocabulary

`capabilities.api_read` gates the read-only Host data accessors (ADR 0043):
each entry must be one of `tasks`, `sessions`, `workspaces`, `workflows`,
`agent_profiles`, `repositories`. Declaring a resource grants the matching
`Host` accessor (`Tasks()`, `Sessions()`, `Workspaces()`, `Workflows()`,
`AgentProfiles()`, `Repositories()` — see [Authoring a
plugin](plugins-authoring.md)); calling an accessor for an undeclared
resource returns gRPC `PermissionDenied`. `capabilities.api_write` reserves
the same resource names for a future write path — no write RPC exists yet,
so declaring it currently has no effect.

## Config schema validation and secret fields

`config_schema` is not an arbitrary, purely descriptive JSON Schema — kandev
validates submitted config against a specific subset of it before
persisting:

- `required` (an array of property names) is enforced — a `PATCH` missing a
  required property is rejected.
- `type` (`string`, `boolean`, `number`, or `integer`) is checked against
  the submitted value.
- `enum` membership is checked when present.
- A property with `secret: true`, or `format: "password"`, is treated as a
  **secret field** and must be `type: string` (or untyped) — a non-string
  secret is rejected. Secret values are moved into kandev's encrypted vault;
  `GET /api/plugins/{id}/config` returns the literal mask `"********"` in
  their place, and resubmitting that mask unchanged is treated as "keep the
  stored value" rather than overwriting it with the literal string.
- `title` is read by the settings-page renderer as a display label
  override for the property (falling back to the property name); it has no
  backend validation effect.

The plugin process itself always sees real, unmasked values (secrets
included) via the `GetConfig` Host RPC — masking only applies to the
operator-facing API/UI.

## Event subscription vocabulary

`capabilities.events` entries are bus subjects (e.g. `task.created`) or
wildcard patterns using `*` as a **single dot-segment** wildcard (e.g.
`task.*`, `agent.*`, `github.*`). A pattern segment of `*` matches exactly
one subject segment; every other segment must match literally; and the
**pattern and subject must have the same number of dot-separated segments**
to match at all. `task.*` matches `task.created` (2 segments each) but does
**not** match a three-segment subject such as `shell.output.<sessionId>`
(`shell.*` would not match it either — 2 vs. 3 segments).

Any subject kandev publishes on its internal event bus is a valid
subscription target — this is not a closed list scoped to any one feature
area. The table below groups every subject defined in
`internal/events/types.go` by domain (some are further suffixed per-session,
e.g. `shell.output.<sessionId>`, `git.event.<sessionId>` — subscribe to the
literal wildcard segment count that matches, e.g. `shell.output.*`).

| Domain | Events |
|---|---|
| Tasks | `task.created`, `task.updated`, `task.state_changed`, `task.deleted`, `task.moved`, `task.tree_hold_created`, `task.tree_hold_released` |
| Workspaces | `workspace.created`, `workspace.updated`, `workspace.deleted` |
| Workflows | `workflow.created`, `workflow.updated`, `workflow.deleted` |
| Workflow steps | `workflow_step.created`, `workflow_step.updated`, `workflow_step.deleted`, `workflow.step_completion_signaled` |
| Comments / messages | `message.added`, `message.updated`, `message.deleted`, `message.queue.status_changed` |
| Task sessions | `task_session.state_changed` |
| Task plans | `task_plan.created`, `task_plan.updated`, `task_plan.deleted`, `task_plan.revision.created`, `task_plan.reverted` |
| Task walkthroughs | `task_walkthrough.created`, `task_walkthrough.updated`, `task_walkthrough.deleted` |
| Session turns | `turn.started`, `turn.completed` |
| Repositories | `repository.created`, `repository.updated`, `repository.deleted`, `repository.script.created`, `repository.script.updated`, `repository.script.deleted` |
| Executors | `executor.created`, `executor.updated`, `executor.deleted`, `executor.profile.created`, `executor.profile.updated`, `executor.profile.deleted`, `executor.prepare.progress`, `executor.prepare.completed` |
| Users | `user.settings.updated` |
| System | `system.job.update` |
| Environments | `environment.created`, `environment.updated`, `environment.deleted` |
| Agent profiles | `agent_profile.created`, `agent_profile.updated`, `agent_profile.deleted` |
| Agents | `agent.started`, `agent.running`, `agent.boot_ready`, `agent.ready`, `agent.completed`, `agent.failed`, `agent.stopped`, `agent.context_reset`, `agent.acp_session_created`, `agentctl.starting`, `agentctl.ready`, `agentctl.error` |
| Agent stream | `agent.stream` (per-session: `agent.stream.<sessionId>`), `agent.turn.message_saved` |
| Agent prompts | `permission_request.received` (per-session: `permission_request.received.<sessionId>`) |
| Clarification | `clarification.answered`, `clarification.primary_answered`, `clarification.cancelled`, `clarification.stale_dismissed` |
| Git / workspace status | `git.event` (per-session), `git.ws` (per-session), `file.change.notified` (per-session) |
| Shell I/O | `shell.output` (per-session), `shell.exit` (per-session) |
| Dev server I/O | `process.output` (per-session), `process.status` (per-session) |
| Session context | `context_window.updated` (per-session), `available_commands.updated` (per-session), `session_mode.changed` (per-session), `agent_capabilities.updated` (per-session), `session_models.updated` (per-session), `session_info.updated` (per-session), `session_todos.updated` (per-session), `session_prompt_usage.updated` (per-session) |
| Automations | `automation.triggered`, `automation.run.created` |
| GitHub | `github.pr_feedback`, `github.pr_state_changed`, `github.new_pr_to_review`, `github.new_issue`, `github.task_pr.updated`, `github.task_ci_options.updated`, `github.watch.event`, `github.rate_limit.updated` |
| GitLab | `gitlab.mr_feedback`, `gitlab.mr_state_changed`, `gitlab.new_mr_to_review`, `gitlab.new_issue`, `gitlab.task_mr.updated`, `gitlab.watch.event` |
| Jira | `jira.new_issue` |
| Linear | `linear.new_issue` |
| Sentry | `sentry.new_issue` |
| Office (autonomous agents) | `office.agent.created`, `office.agent.updated`, `office.agent.status_changed`, `office.skill.created`, `office.skill.updated`, `office.project.created`, `office.project.updated`, `office.approval.created`, `office.approval.resolved`, `office.comment.created`, `office.cost.recorded`, `office.run.queued`, `office.run.processed`, `office.run.event_appended` (per-run), `office.routine.triggered`, `office.inbox.item`, `office.task.status_changed`, `office.task.updated`, `office.task.decision_recorded`, `office.task.review_requested`, `office.provider.health_changed`, `office.route_attempt.appended`, `office.routing.settings_updated` |
| Cross-plugin | `plugin.<plugin_id>.<name>` — published by `Host.EmitEvent`; subscribe with `plugin.<other-plugin-id>.*` to react to another plugin's events. |

> Event list current as of 2026-07-16; regenerate from
> `apps/backend/internal/events/types.go` if this page drifts from the code.

## Runtime-managed fields (do not author these)

Once installed, kandev writes several fields onto the stored record
alongside the parsed manifest. These are **kandev-owned runtime state, not
author-supplied manifest fields** — do not include them in `manifest.yaml`;
they have no effect there and are overwritten on install:

| Field | Meaning |
|---|---|
| `status` | Current lifecycle state: `registered`, `active`, `error`, `disabled`, or `uninstalled`. |
| `install_path` | Absolute path the package was extracted to (`~/.kandev/plugins/<id>/<version>/`). |
| `signed` | Whether the package's `checksums.txt.sig` was cryptographically verified. Signature verification is not currently wired, so this is always false today (every package is reported unsigned). |
| `installed_at` | Install timestamp. |
| `restart_count` | Best-effort restart bookkeeping used by the supervision loop. |

Related: [Plugins](plugins.md), [Authoring a plugin](plugins-authoring.md).
