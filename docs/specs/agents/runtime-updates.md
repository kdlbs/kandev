---
status: approved
created: 2026-07-26
updated: 2026-08-22
owner: Kandev
---

# Managed Agent Runtime Versions and Updates

## Why

Operators need newly released agent models without waiting for a Kandev
release. They also need a UI recovery path when the newest npm release is
partly published, incompatible with ACP, or otherwise cannot start. Rebuilding
an npm cache is not sufficient when an unversioned command selects the same
broken release again.

Operators also need to know that a newer managed agent bridge exists before
they open its version dialog. Kandev must provide a stable reviewed default for
unmodified installations without preventing an operator from selecting a newer
validated version between Kandev releases.

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
- Every managed npm runtime has an exact Kandev default version. A successful
  activation persists an operator-selected exact version for this Kandev
  install. The effective version is the selected version when present and the
  Kandev default otherwise.
- Kandev does not persist the default as a selection. An installation without
  an operator selection follows default-pin changes delivered by later Kandev
  releases.
- Every Kandev-built ACP command for the managed package uses the effective
  exact version, including probes, utility calls, standalone sessions,
  containers, and SSH executors. Active sessions continue unchanged.
- Settings lets the operator clear the selected version and return to the
  Kandev default after that default passes the normal candidate validation.
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
- When the Agents settings page loads, Kandev checks each available managed
  package against npm's stable `latest` version through one batch status
  request. A newer version marks the existing update control with a blue dot
  and accessible update information.
- An update-status check is read-only. It never prepares, probes, selects, or
  starts a runtime. Registry failure leaves the status unknown, keeps the update
  control usable, and does not show a false up-to-date result.
- Kandev caches successful registry status checks for six hours per package and
  failed checks for fifteen minutes. Opening the version dialog still performs
  an authoritative live preview.
- The authoritative preview normalizes valid stable package metadata returned
  by supported npm CLI versions, including npm versions that wrap a
  multi-field `npm view` response in a one-element array. A valid response
  always produces the same stable version catalogue for the operator.
- The version dialog presents a compact status summary and quick choices for
  the latest and Kandev default versions. The complete stable catalogue stays
  behind an explicit browse action and can be searched by version.
- The initial preview fits within the desktop dialog and phone drawer without
  body overflow. The shared body becomes scrollable only when the version
  browser or streamed job output needs more space.
- Opening the version browser uses an anchored popover. The popover must not
  increase the dialog or drawer height, must remain viewport-contained, and
  must close after a version is selected.

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

Kandev distinguishes six version values:

- `current_version` is the version reported by the last successful host ACP
  probe. It can be absent after a failed probe.
- `default_version` is the exact reviewed version shipped with Kandev.
- `active_version` is the exact operator-selected version persisted for future
  managed-runtime commands. It is absent when the operator follows the Kandev
  default.
- `effective_version` is `active_version` when present and `default_version`
  otherwise. It is never empty for a supported managed runtime.
- `latest_version` is npm's stable `latest` version from the most recent update
  status check or authoritative preview. It is absent when that check fails.
- `target_version` is the stable version selected by the operator.

The backend derives the operation from `effective_version` and
`current_version`. When an active operator selection exists and the operator
chooses `default_version`, `use_default` takes precedence over version
comparison. Without that reset condition, the backend classifies the request as
`update`, `rollback`, `repair`, or `up_to_date`:

| Condition | Operation | Approval |
| --- | --- | --- |
| Target is newer than the effective version. | `update` | **Update runtime** |
| Target is older than the effective version. | `rollback` | **Roll back runtime** |
| Current is unknown or versions cannot be compared. | `repair` | **Repair runtime** |
| Effective, current, and target versions match. | `up_to_date` | Disabled as **Up to date** |
| An active operator selection exists and the operator chooses the shipped default. | `use_default` | **Use Kandev default** |

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
    "default_version": "1.18.5",
    "effective_version": "1.18.5"
  }
}
```

`current_version` and `active_version` are independently omitted when unknown
or not selected. `default_version` and `effective_version` are required when
`supported` is true. Unmanaged agents omit `runtime_update`.

### Update status

- `GET /api/v1/agent-update/status` returns one status for every available
  built-in managed runtime.
- The backend queries stale package entries concurrently with a fixed bound and
  reuses fresh per-package cache entries.
- One package lookup failure does not fail the full response.

```json
{
  "statuses": [
    {
      "agent_name": "opencode-acp",
      "package": "opencode-ai",
      "default_version": "1.18.5",
      "effective_version": "1.18.5",
      "latest_version": "1.18.16",
      "check_state": "update_available",
      "checked_at": "timestamp"
    }
  ]
}
```

`check_state` is `update_available`, `up_to_date`, or `unknown`. The backend
sets `update_available` only when strict stable SemVer ordering proves that
`latest_version` is newer than `effective_version`. It does not flag an older
upstream version as an update. An unknown entry omits `latest_version` and
`checked_at` when no successful cached value exists.

### Preview and jobs

- `GET /api/v1/agent-update/:agentName/preview` returns the version catalogue
  and previews the upstream latest stable target.
- `GET /api/v1/agent-update/:agentName/preview?target_version=1.18.5`
  validates and previews one selected version without mutation.
- `GET /api/v1/agent-update/:agentName/preview?use_default=true` previews
  returning to the trusted shipped default.
- `POST /api/v1/agent-update/:agentName` accepts JSON
  `{ "target_version": "1.18.5" }` or `{ "use_default": true }` and starts or
  returns the active job. `use_default` resolves the target from trusted agent
  metadata; callers do not supply both fields.
- `GET /api/v1/agent-update/jobs` and
  `GET /api/v1/agent-update/jobs/:id` retain their current polling behavior.
- State-changing requests use the Settings mutation interlock.

A preview contains:

```json
{
  "agent_name": "opencode-acp",
  "package": "opencode-ai",
  "current_version": "1.18.5",
  "default_version": "1.18.5",
  "effective_version": "1.18.5",
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
  "default_version": "1.18.5",
  "effective_version": "1.18.16",
  "target_version": "1.18.5",
  "output": "",
  "error": "",
  "refresh_error": "",
  "started_at": "timestamp",
  "finished_at": "timestamp"
}
```

`active_version` reflects the persisted operator selection at that job
snapshot. `effective_version` reflects the version future managed commands use.
For a selected target, both become the target only after successful validation
and persistence. For `use_default`, `active_version` is removed and
`effective_version` becomes `default_version` only after successful validation. The
existing `agent.update.started`, `agent.update.output`, and
`agent.update.finished` WebSocket notifications carry the same job fields.

## Activation lifecycle

| State | Backend behavior | Observable behavior |
| --- | --- | --- |
| `queued` | Accept the selected version and maintenance claim. | The version picker and action are disabled. |
| `resolving` | Re-read npm versions, resolve the requested exact target or trusted default, and classify the operation. | The UI shows the selected target and resolving progress. |
| `updating` | Prepare `package@target` in its version-specific npm execution tree. On first failure, invalidate only that exact tree and retry once. | Bounded stdout and stderr stream into the dialog. |
| `refreshing` | Probe the candidate command without replacing cached capabilities. On success, save the selected target or delete the selection for `use_default`, then publish the candidate capabilities. | The UI explains that Kandev is validating before activation. |
| `succeeded` | The exact target is active and its capabilities are published. | The catalogue and models refresh without a page reload. |
| `failed` | Resolution, preparation, probe, or persistence failed. The previous effective version and capabilities remain unchanged. | The UI shows the captured error and permits a new selection or retry. |

Jobs are terminal after `succeeded` or `failed`. Selecting the effective,
healthy version produces `up_to_date` in preview and starts no job. Returning
to the default still validates that default before deleting an existing
selection. Selecting the effective version while its current probe is unknown
produces a repair job.

## Command routing

- Every managed-agent command resolves the effective version immediately before
  building its command and emits the trusted `package@effective_version` spec.
- Boot probes, manual capability refreshes, model-configuration resolution,
  sessionless utility prompts, standalone sessions, containers, and SSH
  executors use the same effective-version resolver.
- Native-binary preference continues to win when an agent deliberately selects
  its supported native binary path.
- Passthrough command construction does not receive or apply the active managed
  ACP version. It continues to use the agent's declared interactive CLI.
- A saved selection read error fails the new managed command with an actionable
  error. It does not fall back to the default or an unversioned package.
- Candidate validation bypasses the active selection only for the trusted exact
  candidate command created by the version job.
- No supported managed-runtime command emits an unversioned package spec.

## Launch-time stale metadata recovery

- Normal managed runtime commands continue to use `--prefer-offline`.
- Kandev inspects bounded process stderr when ACP initialization ends before a
  response. Automatic recovery requires both npm `ETARGET` and a matching
  `No matching version found for package@version` message.
- Kandev classifies the npm error from stderr. It does not build a command from
  package names, versions, paths, or registry values found in stderr.
- Recovery removes only the deterministic `_npx` execution tree for the
  trusted built-in package specification. It never clears npm's full cache.
- Kandev starts the same managed runtime once with `--prefer-online`. The
  command keeps the same trusted package, exact effective version,
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
- A valid package metadata response in a supported npm CLI output shape is not
  treated as a registry failure. Malformed, empty, or ambiguous metadata still
  fails closed and keeps approval disabled.
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
- Registry failure during the batch update-status check returns `unknown` for
  only the affected package. It does not disable the update control, show an
  error badge, or claim that the package is up to date.
- A stale update-available dot can remain until the successful six-hour cache
  entry expires. Opening the dialog always replaces that hint with a live
  authoritative preview. A successful update or reset invalidates the affected
  status cache and refreshes the page state.

## Persistence guarantees

- The trusted package identity and operator-selected version are stored
  install-wide per built-in agent in the system settings store and survive backend and browser
  restarts. A record whose package no longer matches the agent's built-in
  metadata is treated as having no active selection; the replacement package's
  reviewed default becomes effective.
- The Kandev default is compiled into the managed runtime catalogue and is not
  copied into the settings database. Clearing a selection deletes its settings
  record only after the default candidate passes validation.
- An operator-selected version is written only after successful candidate
  validation, so it is also the last known good selection.
- Jobs, process output, and capability cache remain process-local and do not
  survive a backend restart.
- Update-status results and timestamps are process-local cache entries. They do
  not survive a backend restart and do not require a database migration.
- npm's execution cache is best-effort and is not Kandev-owned inventory. If an
  exact effective cache entry disappears, npm may prepare that same exact
  version again; it must not advance to another version.
- Dialog selection, output, and result remain page-local after a browser page
  restart.
- A terminal launch-time npm resolution error stores its stable failure code
  and bounded sanitized details in `last_agent_error`. No database migration is
  required because the record is JSON metadata with optional fields.

## Desktop and mobile behavior

- The existing agent-card update icon remains the entry point on desktop and
  mobile and keeps a minimum 44 px touch target.
- When `check_state` is `update_available`, the update icon shows a blue dot.
  The button's accessible name and desktop tooltip state the current effective
  and latest versions, so color is not the only signal. The dot is absent for
  `up_to_date` and `unknown`.
- Desktop uses the existing dialog. Phone layouts use the existing bottom
  drawer; no nested drawer is introduced.
- The version selector is inside the shared body, is keyboard and touch
  accessible, and shows latest and active markers without encoding state in
  color alone.
- The compact dialog does not render the complete catalogue by default. The
  browse surface exposes the complete list only after the operator requests it,
  provides a search field, and keeps the selected, latest, active, and default
  markers available.
- The body is the single internal scroll owner. The safe-area-aware footer
  keeps the operation action reachable while long version lists and process
  output remain viewport-contained.
- Selection state, preview loading, operation labels, request payloads, and
  terminal results are shared across desktop and mobile presentations.
- Tapping the dotted control on a phone opens the same version drawer and shows
  the authoritative version summary. The dot itself is not a separate touch
  target and does not depend on hover.
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
  boot probes and later managed commands use the same exact version.
- **GIVEN** an agent has no operator selection, **WHEN** Kandev builds any of
  its managed npm ACP commands, **THEN** the command uses the exact reviewed
  Kandev default and never an unversioned package spec.
- **GIVEN** an agent has a validated operator selection, **WHEN** Kandev builds
  a local, container, or SSH managed npm ACP command, **THEN** the command uses
  that exact selection instead of the Kandev default.
- **GIVEN** an operator selection exists, **WHEN** the operator chooses **Use
  Kandev default**, **THEN** Kandev validates the exact default, deletes the
  selection only after success, and future commands follow shipped defaults.
- **GIVEN** a candidate fails ACP initialization, **WHEN** the job ends,
  **THEN** the previous active version and capabilities remain authoritative.
- **GIVEN** the current version is unknown and the effective version is known,
  **WHEN** the operator selects a published target, **THEN** the UI offers
  **Repair runtime** and validation establishes the selected exact version.
- **GIVEN** effective, current, and target versions match, **WHEN** the dialog
  opens, **THEN** it shows **Up to date** and starts no job.
- **GIVEN** a different target is submitted while a job is active for the same
  agent, **WHEN** the backend receives it, **THEN** it returns the existing job
  and does not run a second candidate concurrently.
- **GIVEN** an agent whose interactive passthrough CLI is separate from its
  managed ACP adapter, **WHEN** the effective ACP version changes, **THEN**
  later passthrough sessions still launch the declared interactive CLI and do
  not launch the ACP package under a PTY.
- **GIVEN** a phone viewport and a long version catalogue or process log,
  **WHEN** the operator selects and activates a version, **THEN** the drawer
  remains contained and the primary action remains touch-reachable.
- **GIVEN** npm reports a newer stable version than the effective version,
  **WHEN** the Agents settings page loads, **THEN** the existing update control
  shows a blue dot and exposes both versions in accessible update information.
- **GIVEN** the update-status lookup fails for one managed package, **WHEN** the
  Agents settings page loads, **THEN** that package shows no blue dot, its
  update control remains usable, and other package statuses still render.
- **GIVEN** npm returns valid stable package metadata in either its object form
  or a supported one-element collection form, **WHEN** the operator opens a
  managed runtime update dialog, **THEN** the dialog lists the stable versions,
  selects npm's stable latest version, and does not show a resolution error.
- **GIVEN** a managed runtime has a long stable version catalogue, **WHEN** the
  operator opens its update dialog, **THEN** the dialog shows the status summary
  and quick choices without rendering the full version history.
- **GIVEN** the operator opens the full version browser and enters a version
  fragment, **WHEN** matching versions exist, **THEN** only matching stable
  versions remain selectable and selecting one previews that exact target.
- **GIVEN** the operator opens the full version browser on a phone, **WHEN** the
  operator searches or selects a version, **THEN** the existing update drawer
  keeps one contained scroll owner, exposes 44px touch rows, and preserves the
  same target selection behavior as desktop.
- **GIVEN** the operator opens a long version catalogue, **WHEN** the operator
  opens the version selector, **THEN** the catalogue appears in an anchored
  popover without increasing the dialog or drawer height, and selecting a
  version closes the popover.
- **GIVEN** a dotted update control on a phone, **WHEN** the operator taps it,
  **THEN** the existing update drawer opens and shows a live authoritative
  preview without requiring hover.
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

- Automatic runtime installation, automatic operator-selection changes, and
  automatic rollback after launch failure.
- Global npm cache cleanup, registry replacement, dependency substitution, or
  automatic selection of another package version.
- Prerelease, tag, arbitrary package-spec, registry, or shell-command input.
- Kandev-owned npm artifact retention or a package lockfile.
- Removing npm or network access from the launch path, or locking transitive
  dependency ranges inside upstream packages.
- Restarting or hot-swapping active sessions.
- Native-only update channels and separately distributed passthrough or
  authentication packages.
- Persisting job output or reopening the dialog after a browser restart.
