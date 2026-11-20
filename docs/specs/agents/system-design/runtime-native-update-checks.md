---
status: draft
system: agents
requirements:
  - REQ-AGENTS-RUNTIME-NATIVE-CHECK-001
  - REQ-AGENTS-RUNTIME-NATIVE-CHECK-002
created: 2026-11-17
owners:
  - Kandev
---

# Native Agent Update Checks Design

## Boundary and requirement mapping

The agents system owns native check declarations, result adapters, and runtime
status projection. Notification service keeps recipient preferences and
delivery claims. The web client consumes the existing status and preview
projections. The design extends [runtime update notifications](runtime-update-notifications.md)
and [harness self-update](harness-self-update.md). It does not add an update,
selection, or automation path. Rationale is recorded in
[ADR-2026-11-17](../../../decisions/2026-11-17-native-agent-update-checks.md).

| Requirement | Design |
| --- | --- |
| REQ-AGENTS-RUNTIME-NATIVE-CHECK-001 | [Capability contract](#capability-contract), [Execution](#execution), [OMP adapter](#omp-adapter), [Normalization](#normalization) |
| REQ-AGENTS-RUNTIME-NATIVE-CHECK-002 | [Status integration and cache](#status-integration-and-cache), [Preview and job](#preview-and-job), [Notifications](#notifications), [Web presentation](#web-presentation) |

## Capability contract

`internal/agent/agents` adds an optional capability next to
`HarnessUpdateAgent`:

- `NativeUpdateCheckAgent` exposes `NativeUpdateCheck() NativeUpdateCheckSpec`.
- `NativeUpdateCheckSpec` contains `Command` (trusted direct argv), `Parse`,
  a pure adapter from `NativeUpdateCheckOutput` to `NativeUpdateCheckResult`,
  and `Context`, a `NativeUpdateCheckContext` that names the update
  configuration used for cache identity.
- `NativeUpdateCheckContext` contains `EnvKeys`, the environment variable
  names whose values select the agent's configuration, matched
  case-insensitively, and `Sources`, a pure function. `Sources` maps a
  `NativeUpdateCheckResolver` to candidate configuration file paths and to
  directories whose direct regular files are fingerprinted. The resolver is
  built with `NewNativeUpdateCheckResolver(env, workingDir, executable)`.
  It carries the check environment (`Getenv` and case-insensitive
  `GetenvFold`), the working directory, the home directory (`HOME`, or
  `USERPROFILE` on Windows), and the resolved executable path with symlinks
  evaluated. `NativeUpdateCheckContext.Fingerprint(resolver)`, in
  `internal/agent/agents`, computes the fingerprint.
- `NativeUpdateCheckOutput` contains `Stdout`, `Stderr`, and `ExitCode`.
- `NativeUpdateCheckResult` contains `State`, `CurrentVersion`,
  `LatestVersion`, `Channel`, and `Reason`.

`State` uses the existing check-state values `update_available`, `up_to_date`,
and `unknown`. `Reason` is set only for `unknown` and uses this closed set:

| Reason | Source |
| --- | --- |
| `check_failed` | The adapter recognizes a reported check failure, or the command exits non-zero. |
| `unsupported` | The agent reports that another manager owns updates for the installation. |
| `unrecognized_output` | The output is empty, malformed, or conflicting. A recognized line can also contain an invalid version. |
| `timeout` | The 30-second execution limit expires. |
| `output_limit` | Combined stdout and stderr exceed 64 KiB. |
| `not_installed` | The executable cannot be resolved or started. |
| `runtime_mismatch` | The check's current version differs from the runtime-reported version. |
| `cancelled` | Shutdown stopped the check before it finished. Never cached. |
| `unavailable` | The agent is disabled or discovery does not report it available, so no check ran. Never cached. |

The capability is independent of `HarnessUpdateAgent`, automatic update
support, and installation authority. Any built-in agent that implements
`NativeUpdateCheckAgent` uses the native status source. Its install action,
`management`, and manual guidance still come from `RuntimeUpdateCapabilities`
and `HarnessUpdateAgent`, so a checker-only agent keeps its existing manual or
external presentation. `RuntimeUpdateCapabilities` and
`automaticUpdateSupported` do not inspect the native check, so declaring one
never enables `auto_update_supported`. A `ManagedNPMRuntimeAgent` with an
active managed capability cannot also declare a native check. Pinned updates
need the npm version catalogue, so the registry coverage test rejects that
combination.

`OmpACP` declares `Command: NewCommand("omp", "update", "--check")`,
`Parse: ParseOmpUpdateCheck`, and `Context: ompUpdateCheckContext`.
`HarnessUpdate()`, `BuildCommand`, `Runtime()`, `InstallScript`, and
passthrough configuration do not change.

`ompUpdateCheckContext`, including its declared environment keys, source
files, fingerprint, and the inputs bounded by the cache TTL, is specified in
the [OMP native update check context design](runtime-native-update-check-omp-context.md).

## Execution

The controller owns a `nativeUpdateCheckRunner` seam: `Resolve(command)`
resolves the executable, `Environment()` returns the check environment, and
`Run(ctx, command)` executes. The cache key is therefore built without
starting a process. Production uses `execNativeUpdateCheckRunner`. Tests
inject a fake through `SetNativeUpdateCheckRunner`, matching
`SetRuntimeUpdateStatusResolver`. A fixture backend with canned release
lookups and every E2E backend (`KANDEV_E2E_MOCK=true`) calls
`DisableNativeUpdateCheckExecution`, so native checks report `not_installed`
without running a host agent CLI or contacting a real release feed.

`execNativeUpdateCheckRunner.Run(ctx, command)`:

1. Resolves `argv[0]` with `exec.LookPath`. The production environment is
   `filteredInstallEnv()`, the same environment that `runDirectCommand` uses
   for the trusted update command. The working directory is also inherited
   from the backend process, so both commands resolve the same executable and
   project configuration. A resolution failure returns `not_installed` without
   starting a process.
2. Starts the resolved path directly with `exec.CommandContext`, no shell, the
   same environment, and `WaitDelay` of one second. It does not add arguments.
   Upstream check mode can start package-manager probes (`bun pm bin -g`,
   `npm prefix -g`, `brew --prefix`, and `mise bin-paths`) before its check
   returns (`update-cli.ts` lines 249-299 and 2348-2358). The runner therefore
   contains the whole process tree with `internal/agent/proctree`: a process
   group on Unix and a kill-on-close job object on Windows. Its
   `cmd.Cancel` terminates the tree. It also releases the tree after every
   exit, as `copilotplugins/runner.go` does.
3. Captures stdout and stderr in separate buffers that share one 64 KiB budget.
   The first write over the budget records `output_limit` and cancels the run
   context, which terminates the tree immediately. Partial output is never
   parsed.
4. The first terminal cause takes precedence: `output_limit`, then `timeout`
   on the run deadline, then `cancelled` on lifetime cancellation. A
   non-`ExitError` start failure is `not_installed`. A non-zero exit returns the
   output with its exit code so the adapter can distinguish known failures
   from unrecognized text.
5. Returns the resolved executable path for cache identity.

`internal/agent/proctree` is the existing `copilotplugins`
`prepareCommandTree`, `captureCommandTree`, and `commandTree.kill` code moved
into a shared package as `Prepare`, `Capture`, and `Tree.Kill`.
`copilotplugins` is migrated to the shared package in the same change.
`acpdbg` and `codexdbg` keep their own debug-tool copies.

Timeout is `nativeUpdateCheckTimeout = 30 * time.Second`. It is independent of
`runtimeUpdateStatusLookupTimeout`, which continues to bound registry lookups.
The check timeout equals the server's default 30-second write timeout. The
synchronous `GET /api/v1/agent-update/status` and
`GET /api/v1/agent-update/:agentName/preview` handlers therefore extend their
write deadline with `http.ResponseController` to
`controller.UpdateStatusResponseBudget` (90 seconds). That covers discovery,
one slot wait behind another check, the check timeout, and process cleanup,
so a timed-out check still delivers `unknown` / `timeout`.

Cancellation and admission: the lookup context is derived from the caller with
`context.WithoutCancel`, matching shared registry lookups, so one canceled
subscriber does not stop shared work. Native lookups are also bound to a
controller-owned `nativeCheckLifetime`, which holds `ctx`, `cancel`, `mu`,
`closed`, and an in-flight `sync.WaitGroup`.

- The controller creates an open lifetime at construction, so status works
  before the background worker starts.
- Admission happens once for each execution, at the start of the single-flight
  function under `lifetime.mu`. Subscribers that join an existing flight do not
  register again. If `closed` is set, the execution returns
  `unknown` / `cancelled` without starting anything. Otherwise `wg.Add(1)` runs
  while the mutex is still held, before the execution waits for a semaphore
  slot. Slot waiters are therefore counted.
- A lookup waits for its semaphore slot with `select` on `lifetime.ctx`. The
  process context attaches `context.AfterFunc(lifetime.ctx, cancel)`. The
  lifetime is captured before the single-flight call. The flight key, though
  not the cache key, includes the lifetime's unique id. After a stop and
  restart, a request therefore never joins a cancelled flight that has not
  yet published.
- The background stop function sets `closed`, cancels `lifetime.ctx` under
  `lifetime.mu`, releases the mutex, and calls `wg.Wait()`. Because `Add` only
  happens while the lifetime is open and under the same mutex, no lookup can
  register after stop begins waiting. Every admitted lookup returns
  `cancelled` at its slot wait or kills its process tree.
- Process shutdown cancels the background parent context before the HTTP
  drain. A background goroutine then calls `lifetime.shut()`, which sets
  `closed` and cancels without waiting, so a check held by a live request
  returns `cancelled` before the drain. The later stop callback still joins.
- Identity observation (before the run and the post-run revalidation) does
  filesystem reads that cannot be interrupted. While the lifetime is open it
  runs in its own goroutine and the caller selects on `lifetime.ctx`. If
  shutdown wins, the pre-run lookup is `abandoned` (it resolves to `unknown` /
  `cancelled` and commits nothing), and the post-run result is returned
  uncached. The abandoned read finishes in the background and is discarded.
  Once the lifetime is closed, no observation starts: a pre-run lookup is
  `abandoned` and the post-run result is returned uncached. The runner's
  launch-time `LookPath` and `EvalSymlinks` likewise run off the caller and
  return `cancelled` on cancellation, before any process starts.
- `StartRuntimeUpdateBackground` installs a fresh open lifetime when the
  current one is closed. It never reopens a closed lifetime.

A canceled check returns an `unknown` / `cancelled` entry to every waiting
subscriber and is never written to the cache.

## OMP adapter

`ParseOmpUpdateCheck` is a pure function in `internal/agent/agents`. It
removes ANSI escape sequences, splits stdout and stderr into trimmed lines, and
recognizes only these upstream markers from `packages/coding-agent/src/cli/update-cli.ts`
(upstream commit `2b562c5b7e3f`, installed `omp/18.13.3` confirmed the same
strings):

| Line | Meaning |
| --- | --- |
| `Current version: <v>` | Current version |
| `Current channel: <name>` | Channel. Upstream prints it only for `canary`; absence leaves the channel unreported. |
| `New version available: <v>` | `update_available`, latest `<v>` |
| `<icon> Already up to date` (icon optional) | `up_to_date` |
| `... is installed and kept up to date by <manager>; update it from <manager>.` | `unknown` / `unsupported` |
| `Failed to check for updates: ...`, optionally prefixed with `Error: ` | `unknown` / `check_failed` |

Rules:

- Versions must parse with `semver.NewVersion` and contain no build metadata.
  Prerelease versions are accepted. An invalid version is
  `unrecognized_output`.
- Exactly one outcome marker is required. Both available and up-to-date
  markers, a channel-switch line such as `Switching to ...`, a forced
  reinstall line, or no outcome marker produce `unrecognized_output`.
- A non-zero exit without the failure marker is `check_failed`. A non-zero
  exit with an available or up-to-date marker is `check_failed`; exit status
  never upgrades a result.
- `up_to_date` and `update_available` require a `Current version` line.
- Unrecognized informational lines do not affect a recognized result. This
  allows upstream to add text without breaking the check. Version and outcome
  lines must still match exactly.

## Normalization

The controller applies one rule after the adapter: if the result has a current
version and the target's ACP-reported `currentVersion` is known and different,
the result becomes `unknown` / `runtime_mismatch`, and latest and channel are
cleared. Native results never pass through `compareHarnessUpdateStatus`,
`compareRuntimeUpdateStatus`, `validateRuntimeUpdateLatest`, or
`managedruntime.ParseStableVersion`.

## Status integration and cache

`runtimeUpdateStatusTargets` records `nativeCheck *agents.NativeUpdateCheckSpec`
for every agent that implements `NativeUpdateCheckAgent`. A harness agent
keeps update mode `self_update`, and its `Package` and `Source` remain the
harness package for display. A checker-only agent keeps its capability-derived
mode, `management`, `source`, and guidance URL; only its check state comes from
the native result.

`ListAgentUpdateStatuses` groups targets by lookup key instead of package
alone:

- Registry targets keep `runtimeUpdateStatusCacheKey(packageName, mode)`.
- Native targets resolve the executable and the context fingerprint first,
  then use `nativeUpdateCheckCacheKey(agentID, generation, argv, resolvedPath,
  currentVersion, contextFingerprint)`. It joins `native`, the agent ID, the
  agent's current invalidation generation, NUL-separated argv, the resolved
  path, the ACP-reported version, and the fingerprint.
- `contextFingerprint` is a SHA-256 over sorted `EnvKeys` name/value pairs
  from the check environment and, for each `Sources` candidate file and
  enumerated directory entry, existence, size, and modification time from
  `os.Stat`. File contents are never read, and the fingerprint is never logged
  or exposed.

Both kinds share `runtimeUpdateStatusCache`, `runtimeUpdateStatusFlight`, and
the five-slot `runtimeUpdateStatusLookup` semaphore. A native lookup holds one
slot for its full execution. The single-flight key equals the cache key, so a
request after invalidation uses a new generation and cannot join an older
flight. `runtimeUpdateStatusCacheEntry` gains `state`, `current`, `channel`,
`reason`, and `native`. TTL selection is:

- Six hours (`runtimeUpdateStatusSuccessTTL`) for `update_available`,
  `up_to_date`, and `unsupported`.
- Fifteen minutes (`runtimeUpdateStatusFailureTTL`) for every other reason
  except `cancelled` and `unavailable`, which are never cached.

Every native entry point uses one eligibility guard,
`nativeUpdateCheckEligible(agent, available)`. It requires `agent.Enabled()`
and discovery availability (`c.discovery == nil || available[agentID]`, the
same rule as `runtimeUpdateStatusTargets`). The guard runs before executable
resolution, fingerprinting, cache lookup, or execution. Status keeps
ineligible targets in the response without a lookup. For an ineligible native
agent it reports `check_source: "native"`, `check_state: "unknown"`, and
`check_reason: "unavailable"`, matching preview. A resolution failure is cached under a
key with an empty path and the failure TTL.

The status DTO maps a native entry as follows: `CheckState` from the entry,
`LatestVersion` only for `update_available`, `UpdateChannel` when reported,
`CheckReason` for `unknown`, and `CheckSource: "native"`. `CheckedAt` is set
for completed checks. `EffectiveVersion` and `CurrentVersion` are the ACP
version read by the accepted native observation, which is the version that
keyed and normalized the result. Status and preview both project it, and
preview derives its operation from it. A version read earlier in the
request is not used. Status and preview set `CurrentVersion` from the
check's reported current version only when ACP has not reported one. When
ACP reports a version, any disagreement with the check is already handled as
`runtime_mismatch`.

Invalidation:

- Because the key includes the resolved path, the executable file's size,
  modification time, and mode (`nativeExecutableStat`), the runtime version,
  and the context fingerprint, a change to any of them misses the cache. This
  covers a same-path replacement, as OMP's own updater performs.
- Each agent has one active identity, `nativeUpdateCheckActive[agentName]`.
  It holds the identity (generation, argv, launcher, real executable, runtime
  version, and fingerprint) and an epoch. When a status or preview lookup
  computes a different identity, the agent's other native entries are
  evicted, the epoch increases, and the new identity becomes active. The
  cache and single-flight key is the identity plus its epoch. Returning to an
  earlier identity therefore uses a new key and starts a new check. It does
  not reuse a cached result or join a check still in flight for the earlier
  occurrence. The cache holds at most one native entry per agent.
- Observations are ordered. `prepareNativeUpdateCheck` takes a sequence
  number from `nativeUpdateCheckObserved` before it reads the runtime version
  and the other identity inputs. A different identity replaces the active one
  only if its sequence is newer than the newest observation that confirmed
  the active identity. Otherwise the observation is stale. It observes again,
  up to three times, and then uses an isolated key that is neither shared nor
  cached. A paused, out-of-date request can therefore never evict, or
  restart, a newer identity's result.
- After a check finishes, the result is cached only if the executable the
  runner actually launched (`nativeUpdateCheckRun.Executable`) is the
  prepared launcher, its real path evaluated just before launch
  (`RealExecutable`) is the prepared real executable, the identity recomputed
  after the run equals the
  prepared one, the identity and epoch are still active, and the generation
  is unchanged. Otherwise, for example when an executable or configuration
  changed during the slot wait or the run, even if PATH was restored before
  the post-run observation, the result is returned to its waiting
  subscribers but never cached.
- Project directories are opened with `openFingerprintDir`. On Unix it uses
  `O_DIRECTORY|O_NONBLOCK`, so a FIFO or other non-directory, including one
  reached through a symlink, is rejected without blocking. On Windows it
  checks the type before opening.
- `InvalidateNativeUpdateCheck(agentName)` increments
  `nativeUpdateCheckGeneration[agentName]` under `runtimeUpdateStatusMu` and
  removes that agent's native cache entries. A native lookup records its
  generation when admitted. It writes the cache only if that generation is
  still current, under the same mutex. A check that was running during
  invalidation can return its result to the subscribers that were already
  waiting, but it never fills the cache for the new generation.
- `AgentUpdateJobStore.SetStatusInvalidator` changes from `func(string)` to
  `func(RuntimeStatusInvalidation)`, carrying `AgentName`, `Package`, and
  `UpdateMode` from the job. All terminal call sites in `agent_update_job.go`
  migrate:
  - `finishActivatedWithRefreshError` (also reached through
    `finishActivated`) and `finishRefresh` keep their existing conditions for
    pinned jobs. The conditions are a non-empty package, and also `refreshed`
    for `finishRefresh`. They now send the structured value.
  - For `self_update` jobs only, `finishRefresh` sends it on every outcome and
    `finishFailed` sends it on failure, because a failed updater can still have
    changed the installation. `finishFailed` keeps its current
    no-invalidation behavior for pinned jobs.
  - Ordering for `self_update` jobs: the invalidation runs first in
    `finishRefresh` and `finishFailed`. It runs before the job store takes
    `s.mu` to set the terminal status, call `finishLocked`, or release the
    maintenance claim, and before `complete` or the terminal broadcast. When a
    terminal job or a released claim becomes observable, the agent's native
    generation has therefore advanced, and no request can reuse or join a
    pre-job entry. A check admitted between the invalidation and publication
    belongs to the new generation and observes the installation after the
    updater ran. Pinned-job package invalidation keeps its existing
    post-publication position.
  - Lock order: the invalidator is called with no job-store lock held, and it
    takes only `runtimeUpdateStatusMu`. Status code never acquires the job
    store's `s.mu` while holding `runtimeUpdateStatusMu`, so the two locks
    have no inversion.
  - `controller.go` wires `invalidateRuntimeUpdateStatusForJob`, which calls
    `InvalidateRuntimeUpdateStatus(Package)` when `Package` is set, and
    `InvalidateNativeUpdateCheck(AgentName)`.
- `SetRuntimeUpdateStatusResolver` keeps clearing the whole cache.

## Preview and job

`previewHarnessUpdate` checks for a native declaration. When one exists, it
does not call `resolveHarnessLatest`. It first runs discovery for the agent
(`c.detectAgents`, as status does) and applies
`nativeUpdateCheckEligible`. An ineligible agent gets `check_source: "native"`,
`check_state: "unknown"`, and `check_reason: "unavailable"` without a lookup.
An eligible agent obtains the native entry through the same cache and
single-flight lookup as status. A cache hit is reused, and a miss runs one
shared check. Preview returns `check_source`, `check_state`,
`latest_version`, `update_channel`, and `check_reason` on
`AgentUpdatePreviewDTO` and leaves `stable_latest_version` empty.
`target_version`, `available_versions`, approval shape, and
`harnessUpdateOperation` remain unchanged. Preview never bypasses the cache.

Approval and `runHarness` remain unchanged except for terminal invalidation.
The job still runs `HarnessUpdateSpec.UpdateCommand` (`omp update`), streams
output, probes with `omp acp`, and fails an unchanged ACP version.

## Notifications

`publishRuntimeStatus` already notifies from `CheckState == update_available`,
`LatestVersion`, and runtime identity, and does not parse versions. Native
results therefore use the same availability collector, summary, delivery
claims, and occurrence key (`runtimeNoticeKey(agent, runtime, latest,
"available")`) without changes. Tests cover that a canary latest version
reaches the notifier and that `up_to_date` and `unknown` do not.

## Web presentation

API types in `apps/web/lib/api/domains/agent-update-api.ts` add optional
`check_source?: "native"`, `update_channel?: string`, and
`check_reason?: AgentUpdateCheckReason` to `AgentUpdateStatus`. They add
`check_source`, `check_state`, `latest_version`, `update_channel`, and
`check_reason` to `AgentUpdatePreview`. `AgentUpdateCheckReason` mirrors the
closed backend set. Unknown values fall back to the existing unknown copy.

- `RuntimeOverviewRow` in `agent-runtime-update-rows.tsx` keeps its version
  line and grouping. When `update_channel` is present, it adds a muted channel
  label. `rowDescription` maps `check_reason` to a reason-specific locale key
  before falling back to `agents:runtimeCheckUnknown`. When
  `effective_version` is empty, the row displays the check-reported
  `current_version` for native rows. This is display only: approval and
  classification still use `effective_version`.
- `RuntimeVersionSummary` renders the self-update branch for
  `check_source === "native"` without `agent-update-stable-reference-*`. It
  shows the installed version, then one native result line with test ID
  `agent-update-native-check-<agent>`: the latest version, up to date, or the
  reason explanation. It also shows the channel when reported. A native notice
  replaces `agents:harnessUpdateChannelNotice` and states that the status comes
  from the agent's own check of its configured channel. The confirm action,
  log, and result states remain unchanged.
- Existing grouping in `lib/agents/runtime-update-manager.ts` already maps
  self-update states to available, up to date, and unknown. It keeps native
  OMP out of bulk selection because bulk eligibility requires a pinned target.

After an update job finishes, successfully or not, the dialog hides the
native result line and the channel line, because both describe the
installation before the job ran. The dialog shows the job's installed
version and outcome. A fresh native result appears the next time the dialog
is opened.

Desktop and phone share the dialog and drawer composition. Channel and reason
text wrap within the existing columns and add no new controls or overlays.
Phone keeps page-owned scrolling and the existing 44px actions. All new copy is
in `agents` locale catalogs for all seven shipped locales. `canary` is shown
verbatim as an upstream channel identifier.

## Security

Only built-in metadata supplies the check argv and adapter. Request fields
cannot add a command, flag, path, or environment value. Execution uses direct
argv, no shell, and the same filtered host environment as the existing trusted
updater. Check mode returns before upstream acquires an install lock or
changes the installation. It is not side-effect free, however. Like every
`omp` start, including the `omp acp` probe that Kandev already runs on the
host, it initializes OMP settings and local state. That creates
`<agentDir>/agent.db` and logs. A config file that fails to parse is moved
aside to `config.yml.broken-*` and the process exits non-zero, which is
reported as `check_failed`. This was verified with `omp/19.0.0` in a
disposable home directory. The `omp acp` startup produced the same files and
the same move. The native check therefore adds no side effect that Kandev's
existing automatic ACP probe does not already cause. Output is bounded,
check reasons are fixed codes, and raw check output is not added to DTOs,
notifications, logs, or metrics.

## Observability

Debug logs record agent ID, state, reason, and duration for each executed
check. Do not log raw output, environment variables, or the executable path
above debug level. Do not add a metric. The status DTO `checked_at` and
`check_reason` provide operator-visible diagnostics.

## Validation

- Pure adapter table tests cover stable and canary availability, up to date
  with and without a channel, both failure-prefix variants, managed
  installation, ANSI output, conflicting markers, channel-switch and reinstall
  lines, empty output, invalid versions, and non-zero exit with an outcome
  marker.
- Runner tests use the test binary as a helper process. They cover timeout,
  output overflow followed by a long-lived process that is terminated before
  the timeout, a long-lived descendant removed on both timeout and lifetime
  cancellation, a missing executable, exit-code propagation, environment
  parity with `filteredInstallEnv`, and the absence of added arguments.
  `proctree` keeps the migrated `copilotplugins` runner tests passing.
- Controller tests use a fake runner. They cover registry bypass, canary
  latest preservation, runtime mismatch, TTLs, single-flight concurrency,
  rechecks after path, version, and context-fingerprint changes. Fingerprint
  cases edit the default, named-profile, and `PI_CONFIG_FILES` overlay
  configuration, create a project `.omp` file, edit a known project candidate
  in a directory over the entry limit, change the ownership manifest, and edit
  `.npmrc`. Each case keeps the executable path
  and version. They also cover terminal-job invalidation for successful and
  failed harness jobs, and unchanged package invalidation for managed
  activation. A barrier test blocks check A, invalidates the agent, starts a
  post-invalidation request, releases A, and proves that only a fresh check
  fills the cache. Lifetime barrier tests cover admission racing stop, slot
  waiters released as `cancelled` when the semaphore is saturated, a live
  subscriber receiving `cancelled`, and stop followed by restart. Other tests
  cover disabled and unavailable skips in status and in direct preview requests,
  where a discovery-unavailable fixture with a runnable executable must not
  start the runner. They also cover preview fields without a stable lookup,
  notifications, and a checker-only fixture agent with native status but no
  install or automatic-update eligibility. They also cover unchanged results
  for non-native harness and managed agents.
- Web unit tests cover row channel and reason copy and the native dialog
  summary. Desktop and phone Playwright tests cover native availability,
  channel, unknown explanation, and the unchanged approval flow.
