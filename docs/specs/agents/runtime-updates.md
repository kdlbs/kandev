---
status: approved
created: 2026-07-26
owner: Kandev
---

# Managed Agent Runtime Updates

## Why

Operators need newly released agent capabilities and models without waiting for
a Kandev release. They also need normal agent launches to avoid paying an
upstream package-resolution cost on every session when a usable npm execution
cache is already present.

## What

- Kandev launches the managed npm runtime for Claude, Codex, OpenCode, Copilot,
  and Gemini by package name without an exact version or an explicit `latest`
  tag.
- Normal launches may reuse npm's execution cache. Cache reuse is best-effort;
  Kandev does not present it as a durable installed-version guarantee.
- Each supported installed agent has a compact, accessible update icon action
  on its Settings > Agents card on desktop and mobile. Update versions,
  command details, progress, output, and results are not rendered in the card.
- Opening the update action is read-only. It opens an update dialog that
  resolves and shows the current runtime version, upstream target version,
  exact built-in command, host-only scope, capability refresh, and active
  session behavior before an update can start.
- The update starts only after the operator approves it in the dialog. The
  dialog then shows live stdout/stderr, progress, and the terminal success or
  failure state.
- A successful package update automatically starts a fresh host capability
  probe. The returned runtime version, models, modes, configuration options,
  commands, and capability status replace the previous cached values.
- An update affects new host probes, utility calls, and sessions. It does not
  restart or mutate an already-running agent session.
- Kandev uses the ACP initialization protocol version and advertised
  capabilities as the compatibility boundary. Kandev does not gate managed
  npm runtimes by a repository-maintained package-version allowlist.
- Package update commands are defined by Kandev's built-in agent metadata.
  Callers cannot submit package names, versions, registry URLs, or shell text.
- Update jobs for the same agent are idempotent while queued or running. An
  install and update for the same agent cannot run concurrently.

The initial managed package set is:

| Agent | Managed runtime package |
| --- | --- |
| Claude | `@agentclientprotocol/claude-agent-acp` |
| Codex | `@agentclientprotocol/codex-acp` |
| OpenCode | `opencode-ai` |
| Copilot | `@github/copilot` |
| Gemini | `@google/gemini-cli` |

The managed package is the runtime Kandev uses for ACP capability discovery.
Separately configured passthrough commands and native authentication helpers
remain outside this update action when they use another package or installer.

Decision: [ADR-2026-07-26-user-managed-agent-runtime-updates](../../decisions/2026-07-26-user-managed-agent-runtime-updates.md).

## API surface

### Agent catalogue

Installed-agent catalogue entries expose optional runtime-management metadata:

```json
{
  "runtime_update": {
    "supported": true,
    "package": "@agentclientprotocol/claude-agent-acp",
    "current_version": "0.62.0"
  }
}
```

`current_version` is omitted when no successful capability probe has reported a
runtime version. Unmanaged agents omit `runtime_update`.

### Update jobs

- `GET /api/v1/agent-update/:agentName/preview` resolves the current and
  upstream target versions and returns the exact trusted built-in update
  command without starting an update.
- `POST /api/v1/agent-update/:agentName` starts or returns the active update
  job for a built-in managed agent.
- `GET /api/v1/agent-update/jobs` returns active and recently completed update
  jobs.
- `GET /api/v1/agent-update/jobs/:id` returns one retained update job.
- State-changing update requests use the same Settings interlock as agent
  installation and profile mutation.

An update job contains:

```json
{
  "job_id": "uuid",
  "agent_name": "claude-acp",
  "status": "resolving",
  "current_version": "0.62.0",
  "target_version": "0.63.0",
  "output": "",
  "error": "",
  "refresh_error": "",
  "started_at": "timestamp",
  "finished_at": "timestamp"
}
```

`current_version`, `target_version`, `finished_at`, `error`, and
`refresh_error` are optional until known. The backend emits
`agent.update.started`, `agent.update.output`, and `agent.update.finished`
notifications with the same job identity and state. Output notifications carry
only the appended output chunk.

An update preview contains:

```json
{
  "agent_name": "claude-acp",
  "package": "@agentclientprotocol/claude-agent-acp",
  "current_version": "0.62.0",
  "target_version": "0.63.0",
  "command": [
    "npm",
    "exec",
    "--yes",
    "--prefer-online",
    "--package=@agentclientprotocol/claude-agent-acp",
    "--",
    "node",
    "-e",
    ""
  ],
  "command_string": "npm exec --yes --prefer-online --package=@agentclientprotocol/claude-agent-acp -- node -e \"\""
}
```

The preview endpoint accepts only the built-in agent name. Package names,
versions, registry URLs, and command arguments are not caller-controlled.

## State machine

| State | Trigger | Observable behavior |
| --- | --- | --- |
| `queued` | The backend accepts an update request. | The action is disabled and shows that the update is queued. |
| `resolving` | A worker starts the job. | Kandev discovers the current runtime version and upstream npm target. |
| `updating` | Version resolution succeeds. | Kandev streams package-update output and shows current → target. |
| `refreshing` | The package update succeeds. | Kandev re-probes the host runtime and keeps the action disabled. |
| `succeeded` | The capability probe succeeds, or the package updated but capability refresh returned a recoverable error. | The UI shows the installed target. When refresh succeeded, it replaces model and mode data. A refresh-only error is shown without claiming the package update was rolled back. |
| `failed` | Registry lookup or package update fails, the command times out, or ACP initialization is incompatible. | The UI retains the previous model list and shows the captured error and output. |

Jobs are terminal after `succeeded` or `failed`. Retrying creates a new job.

## Failure modes

- If npm registry metadata cannot be resolved, the job fails before changing
  the runtime and retains the prior capability data.
- If the first package update command fails, Kandev may remove only the
  deterministic npm execution-cache directory for that built-in package and
  retry the update once. It never runs a global npm cache clean. If the repair
  or retry fails or times out, the job fails and retains the prior capability
  data.
- If preview version resolution fails, the dialog shows the error and keeps
  approval disabled. No update job or package command starts.
- If the package update succeeds but the capability probe fails because
  authentication is required or another recoverable probe error occurs, the
  job reports package-update success plus `refresh_error`; the previous model
  list remains visible and the operator can authenticate or retry the refresh.
- If the updated runtime negotiates an unsupported ACP protocol version or
  cannot initialize, the job fails visibly. Kandev does not silently fall back
  to a repository-pinned runtime.
- Raw process output is bounded using the existing in-memory job output limit.
  Package-manager credentials and configured registry authentication are never
  returned as structured fields.
- Loss of the browser connection does not cancel a running job. Reopening the
  page recovers retained job progress through the jobs endpoint.

## Persistence guarantees

- Update jobs and capability data are process-local and do not survive a Kandev
  backend restart.
- Completed jobs remain queryable for the existing short job-retention window.
- Settings does not rehydrate retained update jobs after a browser page
  restart. Update dialog state, output, and results are intentionally
  page-local and disappear when the page is restarted.
- npm's host cache may survive Kandev restarts, but Kandev does not own or
  guarantee that cache.
- After a backend restart, normal host capability probing reports whichever
  runtime npm resolves in that environment.

## Scenarios

- **GIVEN** a managed agent with a cached runtime, **WHEN** a new session starts
  without an explicit update, **THEN** Kandev invokes the unversioned package
  spec and does not require a repository-maintained version pin.
- **GIVEN** Claude reports runtime version `0.62.0` and npm reports `0.63.0`,
  **WHEN** the operator opens its update action, **THEN** the dialog shows
  `0.62.0 → 0.63.0`, the exact built-in command, and how the host update
  affects capabilities and active sessions without starting the command.
- **GIVEN** the update dialog has a resolved preview, **WHEN** the operator
  approves the update, **THEN** Kandev starts exactly one update and the dialog
  streams stdout/stderr until the job is terminal.
- **GIVEN** an update succeeds and the new runtime advertises an additional
  model, **WHEN** the automatic capability probe completes, **THEN** the new
  model appears without a page reload or manual Rescan.
- **GIVEN** an update is already queued, resolving, updating, or refreshing,
  **WHEN** the operator selects the action again, **THEN** Kandev returns the
  existing job and does not run a second update.
- **GIVEN** an install is active for an agent, **WHEN** an update is requested
  for that agent, **THEN** Kandev returns the active maintenance job rather
  than running install and update concurrently.
- **GIVEN** npm registry lookup fails while preparing the dialog, **WHEN** the
  update action is opened, **THEN** the dialog shows a retryable preview failure,
  keeps approval disabled, and starts no update job.
- **GIVEN** npm registry lookup succeeds for the preview but fails after
  approval because the registry changed, **WHEN** the update job resolves its
  target again, **THEN** the dialog shows a retryable update failure and retains
  the previous models.
- **GIVEN** a managed package's extracted `_npx` execution tree is corrupt,
  **WHEN** the first update preparation fails, **THEN** Kandev invalidates only
  that built-in package's deterministic execution-cache directory, retries
  once, and probes the rebuilt runtime before reporting success.
- **GIVEN** the package update succeeds but the fresh probe requires
  authentication, **WHEN** the job finishes, **THEN** the dialog reports the new
  package version and refresh error, the previous models remain available, and
  the card keeps its existing authentication recovery action.
- **GIVEN** an agent is unmanaged or native-only, **WHEN** Settings renders its
  installed card, **THEN** no update action is shown.
- **GIVEN** an update dialog has shown progress or a terminal result, **WHEN**
  the operator restarts the page, **THEN** the dialog is closed and no prior
  update details, output, or result appear on the agent card or in a newly
  opened dialog.
- **GIVEN** an update is running on a phone viewport, **WHEN** the operator
  views the update surface, **THEN** current and target versions, the command,
  progress, output, and retry state are reachable by touch without horizontal
  page scrolling.
- **GIVEN** an agent session is already running, **WHEN** its host runtime is
  updated, **THEN** the existing session continues unchanged and only later
  probes or launches use the updated runtime.

## Out of scope

- Scheduled or automatic package updates.
- Automatically deleting or rebuilding npm execution caches during every
  ordinary agent launch.
- Exact package-version pins, version allowlists, rollback, or user-selected
  historical versions.
- Updating configured remote executors or every running container from the
  host Settings action.
- Restarting or hot-swapping active agent sessions.
- Managing native-only update channels such as Cursor.
- Updating separately distributed passthrough or authentication helper
  packages when they are not the managed ACP runtime.
- Resuming or recovering update dialog state after a browser page restart.
