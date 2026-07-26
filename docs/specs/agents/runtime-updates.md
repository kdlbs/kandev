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
- Each supported installed agent has a visible **Update agent** action on the
  Settings > Agents page on desktop and mobile.
- Starting an update shows the current runtime version, the upstream target
  version, live progress, and the terminal success or failure state.
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
- If the package update command fails or times out, the job fails and retains
  the prior capability data.
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
- npm's host cache may survive Kandev restarts, but Kandev does not own or
  guarantee that cache.
- After a backend restart, normal host capability probing reports whichever
  runtime npm resolves in that environment.

## Scenarios

- **GIVEN** a managed agent with a cached runtime, **WHEN** a new session starts
  without an explicit update, **THEN** Kandev invokes the unversioned package
  spec and does not require a repository-maintained version pin.
- **GIVEN** Claude reports runtime version `0.62.0` and npm reports `0.63.0`,
  **WHEN** the operator selects **Update agent**, **THEN** the card shows
  `0.62.0 → 0.63.0` and streams progress until the job is terminal.
- **GIVEN** an update succeeds and the new runtime advertises an additional
  model, **WHEN** the automatic capability probe completes, **THEN** the new
  model appears without a page reload or manual Rescan.
- **GIVEN** an update is already queued, resolving, updating, or refreshing,
  **WHEN** the operator selects the action again, **THEN** Kandev returns the
  existing job and does not run a second update.
- **GIVEN** an install is active for an agent, **WHEN** an update is requested
  for that agent, **THEN** Kandev returns the active maintenance job rather
  than running install and update concurrently.
- **GIVEN** npm registry lookup fails, **WHEN** an update is requested,
  **THEN** the card shows a retryable failure and retains the previous models.
- **GIVEN** the package update succeeds but the fresh probe requires
  authentication, **WHEN** the job finishes, **THEN** the card reports the new
  package version, shows the refresh error, retains the previous models, and
  keeps the existing authentication recovery action available.
- **GIVEN** an agent is unmanaged or native-only, **WHEN** Settings renders its
  installed card, **THEN** no update action is shown.
- **GIVEN** an update is running on a phone viewport, **WHEN** the operator
  views the installed-agent card, **THEN** current and target versions,
  progress, output, and retry state are reachable by touch without horizontal
  page scrolling.
- **GIVEN** an agent session is already running, **WHEN** its host runtime is
  updated, **THEN** the existing session continues unchanged and only later
  probes or launches use the updated runtime.

## Out of scope

- Scheduled or automatic package updates.
- Exact package-version pins, version allowlists, rollback, or user-selected
  historical versions.
- Updating configured remote executors or every running container from the
  host Settings action.
- Restarting or hot-swapping active agent sessions.
- Managing native-only update channels such as Cursor.
- Updating separately distributed passthrough or authentication helper
  packages when they are not the managed ACP runtime.
