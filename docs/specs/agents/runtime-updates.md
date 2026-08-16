---
status: approved
created: 2026-07-26
updated: 2026-08-16
owner: Kandev
---

# Managed Agent Runtime Version Recovery

## Why

Operators need newly released agent models without waiting for a Kandev
release. They also need a UI recovery path when the newest npm release is
partly published, incompatible with ACP, or otherwise cannot start. Rebuilding
an npm cache is not sufficient when an unversioned command selects the same
broken release again.

Managed runtimes can also fail when npm has stale package metadata. In that
case, npm can report `ETARGET` and `No matching version found` for a dependency
that the configured registry now contains. Kandev currently reports the later
ACP disconnect instead of the npm cause. The user then has no useful recovery
action unless they inspect backend logs.

## What

- Settings exposes version management for the built-in managed npm runtimes
  used by Claude, Codex, OpenCode, Copilot, and Gemini.
- The update dialog lists stable versions published for the trusted package.
  The list contains the newest 50 stable versions plus the active and last
  observed versions when either falls outside that window. The upstream
  `latest` stable version is selected initially.
- The backend classifies the selected action as `update`, `rollback`, `repair`,
  or `up_to_date`. The UI uses this structural state for copy and approval; it
  never compares translated labels or version strings itself.
- Kandev stages the exact trusted `package@version`, ACP-probes that candidate,
  and activates it only after a successful probe. Candidate failure preserves
  the prior active version and capability catalogue.
- A successful activation persists the exact version for this Kandev install.
  Later standalone host probes, utility calls, and local sessions use that
  exact package spec. Active sessions continue unchanged.
- When no active selection exists, legacy host launches remain unversioned.
  The first successful update, rollback, or repair establishes the durable
  active version.
- SSH, containers, and other remote executors do not use the host selection.
- Package name, ACP arguments, and command shape remain trusted built-in
  metadata. A caller can submit only a version returned by the trusted
  package's npm metadata. Tags, prereleases, package specs, registry URLs, and
  shell text are rejected.
- Managed runtime resolution changes only the structured ACP command surfaces.
  When an agent's interactive passthrough CLI is distributed separately from
  its ACP adapter, passthrough keeps its own package, executable, and install
  recipe.
- Jobs for one agent are idempotent while queued or running. Installation and
  version management for the same agent cannot run concurrently.
- A host-local managed runtime launch detects a strict npm `ETARGET` version
  resolution failure before ACP initialization. Kandev refreshes npm metadata
  and retries the same trusted package and version once.
- If the retry also fails, Kandev reports an npm runtime preparation error and
  offers one runtime retry action. The user does not need to understand ACP.

The managed package set is:

| Agent | Managed runtime package |
| --- | --- |
| Claude | `@agentclientprotocol/claude-agent-acp` |
| Codex | `@agentclientprotocol/codex-acp` |
| OpenCode | `opencode-ai` |
| Copilot | `@github/copilot` |
| Gemini | `@google/gemini-cli` |

Passthrough commands, native authentication helpers, and native-only agents
remain outside this version action when they use another installer or package.

Decision:
[ADR-2026-08-12-validated-managed-runtime-version-selection](../../decisions/2026-08-12-validated-managed-runtime-version-selection.md).

## Version and operation semantics

Kandev distinguishes three version values:

- `current_version` is the version reported by the last successful host ACP
  probe. It can be absent after a failed probe.
- `active_version` is the exact version persisted for future host-local
  managed-runtime commands. It can be absent before the first activation.
- `target_version` is the stable version selected by the operator.

The backend derives the operation from `active_version` when present and
otherwise from `current_version`:

| Condition | Operation | Approval |
| --- | --- | --- |
| Target is newer than the effective version. | `update` | **Update runtime** |
| Target is older than the effective version. | `rollback` | **Roll back runtime** |
| Current is unknown, versions cannot be compared, or the observed version matches but has not been activated exactly. | `repair` | **Repair runtime** |
| Active, current, and target versions match. | `up_to_date` | Disabled as **Up to date** |

Only strict stable SemVer values from npm's published version list are
selectable. Kandev does not offer prereleases. Version ordering and operation
classification are backend responsibilities.

## API surface

### Agent catalogue

Installed-agent catalogue entries expose optional runtime-management metadata:

```json
{
  "runtime_update": {
    "supported": true,
    "package": "opencode-ai",
    "current_version": "1.18.5",
    "active_version": "1.18.5"
  }
}
```

`current_version` and `active_version` are independently omitted when unknown
or not yet established. Unmanaged agents omit `runtime_update`.

### Preview and jobs

- `GET /api/v1/agent-update/:agentName/preview` returns the version catalogue
  and previews the upstream latest stable target.
- `GET /api/v1/agent-update/:agentName/preview?target_version=1.18.5`
  validates and previews one selected version without mutation.
- `POST /api/v1/agent-update/:agentName` accepts JSON
  `{ "target_version": "1.18.5" }` and starts or returns the active job.
- `GET /api/v1/agent-update/jobs` and
  `GET /api/v1/agent-update/jobs/:id` retain their current polling behavior.
- State-changing requests use the Settings mutation interlock.

A preview contains:

```json
{
  "agent_name": "opencode-acp",
  "package": "opencode-ai",
  "current_version": "1.18.5",
  "active_version": "1.18.5",
  "target_version": "1.18.16",
  "operation": "update",
  "available_versions": [
    { "version": "1.18.16", "latest": true },
    { "version": "1.18.5", "latest": false }
  ],
  "command": [
    "npm",
    "exec",
    "--yes",
    "--prefer-online",
    "--package=opencode-ai@1.18.16",
    "--",
    "node",
    "-e",
    ""
  ],
  "command_string": "npm exec --yes --prefer-online --package=opencode-ai@1.18.16 -- node -e \"\""
}
```

The POST endpoint resolves npm metadata again and rejects a target that is no
longer a published stable version. The request never controls the package,
registry, command, or ACP arguments.

A job retains the existing timestamps, output, and errors and adds the
authoritative operation and active version:

```json
{
  "job_id": "uuid",
  "agent_name": "opencode-acp",
  "status": "refreshing",
  "operation": "rollback",
  "current_version": "1.18.16",
  "active_version": "1.18.16",
  "target_version": "1.18.5",
  "output": "",
  "error": "",
  "refresh_error": "",
  "started_at": "timestamp",
  "finished_at": "timestamp"
}
```

`active_version` reflects the persisted selection at that job snapshot. It
changes to the target only after successful validation and persistence. The
existing `agent.update.started`, `agent.update.output`, and
`agent.update.finished` WebSocket notifications carry the same job fields.

## Activation lifecycle

| State | Backend behavior | Observable behavior |
| --- | --- | --- |
| `queued` | Accept the selected version and maintenance claim. | The version picker and action are disabled. |
| `resolving` | Re-read npm versions, validate the exact target, and classify the operation. | The UI shows the selected target and resolving progress. |
| `updating` | Prepare `package@target` in its version-specific npm execution tree. On first failure, invalidate only that exact tree and retry once. | Bounded stdout and stderr stream into the dialog. |
| `refreshing` | Probe the candidate command without replacing cached capabilities. On success, persist the target and then publish the candidate capabilities. | The UI explains that Kandev is validating before activation. |
| `succeeded` | The exact target is active and its capabilities are published. | The catalogue and models refresh without a page reload. |
| `failed` | Resolution, preparation, probe, or persistence failed. The previous active version and capabilities remain unchanged. | The UI shows the captured error and permits a new selection or retry. |

Jobs are terminal after `succeeded` or `failed`. Selecting the active, healthy
version produces `up_to_date` in preview and starts no job. Selecting the active
version while its current probe is unknown produces a repair job.

## Host command routing

- A standalone managed-agent launch reads the active selection immediately
  before building its command and passes the exact version through trusted
  command options.
- Boot probes, manual capability refreshes, model-configuration resolution, and
  sessionless utility prompts use the same active-version resolver.
- Native-binary preference continues to win when an agent deliberately selects
  its supported native binary path.
- Passthrough command construction does not receive or apply the active managed
  ACP version. It continues to use the agent's declared interactive CLI.
- SSH and container command builders receive no host version override.
- A saved selection read error fails the new host command with an actionable
  error. It does not fall back to an unversioned package.
- Candidate validation bypasses the active selection only for the trusted exact
  candidate command created by the version job.

## Launch-time stale metadata recovery

- Normal host-local managed runtime commands continue to use
  `--prefer-offline`.
- Kandev inspects bounded process stderr when ACP initialization ends before a
  response. Automatic recovery requires both npm `ETARGET` and a matching
  `No matching version found for package@version` message.
- Kandev classifies the npm error from stderr. It does not build a command from
  package names, versions, paths, or registry values found in stderr.
- Recovery removes only the deterministic `_npx` execution tree for the
  trusted built-in package specification. It never clears npm's full cache.
- Kandev starts the same managed runtime once with `--prefer-online`. The
  command keeps the same trusted package, exact active version when one exists,
  ACP arguments, configured npm registry, command prefix, permissions, model,
  and session identity.
- Recovery is limited to one retry for each launch attempt. A delayed event
  from the first child process cannot fail or complete the replacement process.
- User cancellation and backend shutdown stop recovery. Kandev does not retry
  a remote executor, a native runtime, a passthrough command, an unrelated npm
  error, or a second failed online attempt.
- A successful retry continues the original session without a failure card.
  Kandev records structured recovery telemetry without exposing host paths or
  raw process logs.
- A failed retry emits a stable npm runtime failure code and bounded sanitized
  details. This evidence is stored with the last agent error so the focused UI
  survives a page reload.

## Failure and recovery behavior

- Registry failure during preview keeps approval disabled and runs no command.
- Registry failure or target disappearance after approval fails before staging.
- Preparation failure invalidates only the deterministic `_npx` tree for the
  exact `package@version` and retries once. Kandev never runs a global npm cache
  clean.
- ACP initialization failure, unsupported protocol behavior, authentication
  required, or an unsuccessful capability probe does not activate the
  candidate. The staged npm cache may remain for a later retry.
- Persistence failure after a successful candidate probe does not publish the
  candidate capabilities and leaves the previous active selection unchanged.
- Browser disconnect does not cancel a running job. The jobs endpoint can
  recover process-local progress while the backend remains running.
- Active sessions are never restarted, replaced, or hot-swapped.
- A launch-time stale metadata retry is not a version rollback. It prepares the
  same package selection again and does not change the active version.

## Persistence guarantees

- The trusted package identity and active version are stored install-wide per
  built-in agent in the system settings store and survive backend and browser
  restarts. A record whose package no longer matches the agent's built-in
  metadata is treated as having no active selection and is not applied to the
  replacement package.
- The active version is written only after successful candidate validation, so
  it is also the last known good selection.
- Jobs, process output, and capability cache remain process-local and do not
  survive a backend restart.
- npm's execution cache is best-effort and is not Kandev-owned inventory. If an
  exact selected cache entry disappears, npm may prepare that same exact
  version again; it must not advance to another version.
- Dialog selection, output, and result remain page-local after a browser page
  restart.
- A terminal launch-time npm resolution error stores its stable failure code
  and bounded sanitized details in `last_agent_error`. No database migration is
  required because the record is JSON metadata with optional fields.

## Desktop and mobile behavior

- The existing agent-card update icon remains the entry point on desktop and
  mobile and keeps a minimum 44 px touch target.
- Desktop uses the existing dialog. Phone layouts use the existing bottom
  drawer; no nested drawer is introduced.
- The version selector is inside the shared body, is keyboard and touch
  accessible, and shows latest and active markers without encoding state in
  color alone.
- The body is the single internal scroll owner. The safe-area-aware footer
  keeps the operation action reachable while long version lists and process
  output remain viewport-contained.
- Selection state, preview loading, operation labels, request payloads, and
  terminal results are shared across desktop and mobile presentations.
- A failed automatic launch retry uses the existing inline recovery card in
  Kanban chat and Office chat. It does not open a dialog or drawer.
- The card states that npm could not prepare the agent runtime. It states that
  Kandev refreshed package data and retried once. Technical details are
  collapsed initially.
- The card offers one **Retry runtime** action. When a resume token exists, the
  action resumes the session. Otherwise, it starts a replacement run. The card
  does not present session history loss as a fix for an npm problem.
- Phone actions stack when needed, remain at least 44 px high, and do not add a
  second scroll container.

## Scenarios

- **GIVEN** OpenCode latest is partly published and its ACP probe fails,
  **WHEN** an operator selects an older published stable version and approves
  **Roll back runtime**, **THEN** Kandev prepares and probes that exact version,
  persists it only after success, and restores its model list without restart.
- **GIVEN** a healthy exact active version, **WHEN** Kandev restarts, **THEN**
  boot probes and later standalone sessions use the same exact version.
- **GIVEN** a candidate fails ACP initialization, **WHEN** the job ends,
  **THEN** the previous active version and capabilities remain authoritative.
- **GIVEN** the current version is unknown and there is no active selection,
  **WHEN** the operator selects a published target, **THEN** the UI offers
  **Repair runtime** and validation establishes the first exact active version.
- **GIVEN** active, current, and target versions match, **WHEN** the dialog
  opens, **THEN** it shows **Up to date** and starts no job.
- **GIVEN** a different target is submitted while a job is active for the same
  agent, **WHEN** the backend receives it, **THEN** it returns the existing job
  and does not run a second candidate concurrently.
- **GIVEN** an SSH or container session, **WHEN** the host active version
  changes, **THEN** that executor's command remains unchanged.
- **GIVEN** an agent whose interactive passthrough CLI is separate from its
  managed ACP adapter, **WHEN** the host active ACP version changes, **THEN**
  later passthrough sessions still launch the declared interactive CLI and do
  not launch the ACP package under a PTY.
- **GIVEN** a phone viewport and a long version catalogue or process log,
  **WHEN** the operator selects and activates a version, **THEN** the drawer
  remains contained and the primary action remains touch-reachable.
- **GIVEN** a host-local managed runtime exits before ACP initialization with
  npm `ETARGET` and a matching missing dependency version, **WHEN** Kandev
  reads the captured stderr, **THEN** it removes only the trusted package's
  deterministic `_npx` tree and retries the same runtime once with current npm
  metadata.
- **GIVEN** that online retry starts successfully, **WHEN** ACP initialization
  completes, **THEN** the original session continues and no recovery card is
  shown.
- **GIVEN** that online retry fails with the same npm resolution error,
  **WHEN** the failure reaches Kanban or Office chat, **THEN** the UI explains
  the npm cause, keeps sanitized technical details collapsed, and offers only
  **Retry runtime**.
- **GIVEN** a native, SSH, container, passthrough, or unrelated failed launch,
  **WHEN** Kandev classifies the error, **THEN** this automatic npm recovery
  does not run.

## Out of scope

- Scheduled or automatic updates and automatic rollback after launch failure.
- Global npm cache cleanup, registry replacement, dependency substitution, or
  automatic selection of another package version.
- Prerelease, tag, arbitrary package-spec, registry, or shell-command input.
- Kandev-owned npm artifact retention or a package lockfile.
- Applying host selections to SSH, container, or other remote runtimes.
- Restarting or hot-swapping active sessions.
- Native-only update channels and separately distributed passthrough or
  authentication packages.
- Persisting job output or reopening the dialog after a browser restart.
