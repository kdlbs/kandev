---
status: shipped
created: 2026-08-08
owner: kandev
---

# Storage maintenance scan, capacity, and dependency cleanup

This is a focused extension to [Storage maintenance](storage-maintenance.md). It improves cold
analysis time, gives the operator an immediate view of the filesystem capacity that contains
Kandev's storage, and adds an opt-in way to reclaim dependency directories from archived or deleted
task workspaces without deleting their source trees.

## Why

The Storage maintenance page currently waits for a cold overview scan before it can show the
analysis card. The page already requests policy, run history, quarantine, and overview data
independently, so those lightweight sections are not the source of the long wait. The backend
overview itself measures workspaces, the Go build cache, quarantine, and Docker usage one after
another.

The page also needs to answer two operator questions without requiring filesystem expertise: how
close is the host volume containing Kandev's data to full, and can dependency-heavy archived task
workspaces be trimmed while their source remains recoverable? The capacity signal must not wait for
the slow overview scan, and dependency cleanup must be explicitly enabled because restoring an
archived workspace after pruning requires dependencies to be installed again.

A live cold request took about 103 seconds on a host with approximately 102 GB of workspace data
and 38 GB of Go-cache data. The same route was effectively instantaneous on the next request
because the successful 15-minute overview cache was warm. The independent measurements should be
able to overlap so the cold path is governed primarily by the slowest source rather than the sum
of all four source times.

## What

### Parallel overview measurements

After the settings snapshot needed to construct the providers is loaded, the overview scan starts
these independent measurements concurrently:

- workspace usage and candidate classification;
- Go build-cache usage and candidate classification;
- quarantine totals;
- Docker usage, when Docker is configured and available.

The scan waits for all four results before returning the existing `Summary`. Each measurement keeps
its current value/error behavior: one failed source does not discard successful results from the
other sources, and the response shape remains unchanged.

The existing successful-snapshot cache, 15-minute freshness window, forced refresh used by manual
Analyze, concurrent-refresh coalescing, and stale-snapshot protection remain in force. No new
durable state is introduced by this scan optimization.

### Filesystem capacity indicator

The page adds a lightweight, independently loaded disk-capacity card. It reads the filesystem that
contains Kandev's storage root (the task/workspace storage volume), rather than adding Kandev-owned
byte counts to the host disk total. The card shows:

- a localized exact percentage such as **80% full**;
- a progress bar with the same value exposed through `aria-valuenow`;
- used, available, and total capacity in the existing GB formatting; and
- the measured storage path or a localized explanation when the capacity read is unavailable.

The capacity request is not held behind the cold overview scan and is not served from the 15-minute
overview snapshot. It can therefore show current host-volume pressure while the detailed analysis
card is still loading. The percentage is calculated from the filesystem's service-available bytes,
clamped to 0–100, and the API returns the raw byte values so the UI never has to infer them.

The indicator uses accessible warning styling when the volume reaches 80% full and critical styling
at 90% full. Those thresholds are presentation constants, not cleanup triggers; the card does not
automatically delete anything when a threshold is crossed.

### Opt-in dependency-directory cleanup

Maintenance settings add an off-by-default **Remove dependencies from archived or deleted task
workspaces** option under the Workspaces section. When enabled, scheduled maintenance and a full
manual run may recursively remove a fixed, safety-reviewed allowlist of dependency directories
inside Kandev-owned task workspace roots:

- `node_modules`, `bower_components`, and `.pnpm-store`;
- `.yarn/cache` and `.yarn/unplugged`;
- `.venv`, `venv`, `.tox`, `.nox`, and `__pypackages__`;
- `Pods` and `.gradle` when they are nested beneath an eligible task workspace.

The initial allowlist deliberately excludes ambiguous names such as `vendor`, `target`, `build`,
`dist`, `.cache`, and `.git`. Those may contain committed source, recovery metadata, or unrelated
artifacts and require a separate classification decision. The option does not accept arbitrary
paths or shell commands.

The Workspaces policy card always shows the exact allowlist in a localized, code-styled **Folders
Kandev will check** disclosure so the user can see what the option covers before enabling it. An
adjacent information icon provides hover, keyboard-focus, and tap behavior and explains that these
are directory names Kandev searches for recursively, that absent directories are simply skipped,
that excluded names are never targeted, and that restoring a pruned task may require reinstalling
dependencies. The list and help content remain usable on mobile; hover is an enhancement, not the
only way to access the explanation.

Dependency pruning uses the same fail-closed workspace ownership and activity inventory as orphan
workspace cleanup. A target must be beneath a real Kandev-owned task workspace, belong to a task that
is archived or deleted, and have no active execution, session worktree, environment borrower, or
other protected reference. The provider revalidates the target before each destructive operation.
Deleted tasks are identified from the ownership marker and the absence of an authoritative task
row; an incomplete inventory causes the provider to make no changes.

This option does not change the existing full-workspace lifecycle. The current defaults are seven
days of orphan grace followed by seven days of quarantine retention, not a single two-week age
threshold. Dependency pruning can reclaim the large dependency directories during that lifecycle
while preserving source files and workspace metadata. A later restore succeeds with the remaining
workspace, but the UI and run result explain that dependencies were removed and may need to be
reinstalled.

The existing manual resource-selection contract gains the provider name
`workspace_dependencies`, allowing an explicit **Clean workspace dependencies** action in the
Workspaces settings card. It uses the normal maintenance activity gate, busy-resource feedback,
Run anyway behavior, cancellation, retry, and run-history reporting. It never runs as part of
read-only Analyze.

### Page behavior

The Storage maintenance page continues to render policy, history, quarantine, and the capacity card
as soon as their independent requests complete. Only the analysis card remains in its existing
loading state while the cold overview scan is running. A cached overview continues to load without a
new filesystem scan, and manual Analyze continues to request a fresh read-only snapshot. Enabling
dependency cleanup adds one policy control and one workspace-specific action without changing the
mobile one-column composition.

### Idle behavior invariant

This change does not alter cleanup scheduling or task admission. The existing resource-idle gate
continues to block scheduled cleanup when instrumented execution, shell/test/setup/cleanup work,
or Docker image builds are active. The quiet period is measured from the last released task
activity, and a newly admitted task cancels and drains a maintenance lease before proceeding.

Manual Analyze remains read-only and is not idle-gated. The existing `skipped_busy` history entry
that reports `execution_preparing` and `execution_running` is therefore a correct scheduled-run
outcome, not an overview-scan failure.

## Success criteria

- A deterministic backend test proves all four independent measurements have started before any
  blocking measurement is released; the test must not rely on a timing race.
- The same test proves the returned summary and per-source errors match the serial implementation.
- When a compatible post-change backend is available, a cold live overview measurement is captured
  after implementation using the existing request timing logs. The result should be bounded by the
  slowest provider plus join overhead, rather than the sum of provider durations, on the same host
  and dataset. If the only running backend predates the capacity route, the baseline timing plus the
  deterministic fan-out barrier and held-overview managed E2E are the accepted non-mutating evidence
  until a compatible runtime is available.
- Existing cache tests still prove that warm reads, forced Analyze refreshes, coalesced misses, and
  failed refreshes behave exactly as before.
- Existing desktop and mobile Storage maintenance flows continue to show fast sections while an
  overview is pending, and the capacity card renders independently with no horizontal overflow.
- Capacity tests verify exact byte fields, percentage clamping, 80%/90% presentation thresholds,
  unavailable filesystems, and accessible progress-bar semantics.
- Dependency-cleanup tests verify the default-off setting, allowlist-only deletion, byte/count
  reporting, symlink and path-containment safety, active-workspace protection, archived/deleted
  eligibility, cancellation, retry, and restore-after-prune behavior.
- Desktop and mobile UI tests verify the visible allowlist, the info control's hover/focus/tap
  explanation, excluded-folder copy, and no horizontal overflow.
- Existing activity-coordinator and scheduled-runner tests continue to prove the idle behavior
  described above.

## Scenarios

### Cold overview starts independent sources together

Given no cached successful overview, when `GET /api/v1/system/storage` or manual Analyze begins,
then settings are loaded once, all independent measurements start without waiting for one another,
and the response contains the same summary fields as before.

### Capacity is visible while analysis is pending

Given a slow or cold overview scan, when the Storage page opens, then the independent capacity
request can render the filesystem percentage and progress bar before the detailed analysis finishes.
If the capacity read fails, the card shows an unavailable state without blocking the other sections.

### Dependency cleanup is opt-in and scoped

Given the dependency option is disabled, when scheduled or manual maintenance runs, then no
dependency directory is removed. Given it is enabled and an archived or deleted workspace is
unprotected, when the workspace-dependency provider runs, then only allowlisted dependency
directories beneath that workspace are removed and the run records their count and reclaimed bytes.

### The UI explains the cleanup scope

Given the Workspaces policy card is visible, then it shows the exact allowlisted folder names before
the user enables the option. Its information control explains the recursive search, skipped absent
folders, excluded folder names, safety checks, and reinstall consequence through hover, keyboard
focus, and touch activation.

### Active or restored workspaces are protected

Given a workspace is active, borrowed by an active task, has an active execution/session handle, or
changes state while pruning, then the provider skips it without deleting any dependency directory.
Given a user restores a workspace after dependencies were pruned, then the source tree and recovery
metadata remain available and the result explains that dependencies must be reinstalled.

### One source fails

Given one measurement returns an error, when the other measurements complete, then successful
source results remain available, the failed source reports its existing unavailable/error state,
and the request does not deadlock or leak measurement goroutines.

### Cached overview is reused

Given a successful snapshot younger than 15 minutes, when the page is opened, refreshed, or policy
settings are saved, then the cached snapshot is returned without starting another filesystem or
Docker scan.

### Manual Analyze remains read-only

Given active task execution, when the user selects Analyze, then the request may run because it
does not acquire the maintenance idle lease and it does not delete or mutate storage.

### Scheduled cleanup remains idle-gated

Given `execution_preparing` or `execution_running` activity is active, when the scheduled cleanup
interval fires, then the run is persisted as `skipped_busy` with those stable resource labels and
no cleanup provider runs. When a task is admitted while maintenance holds the lease, maintenance
is cancelled and drained before the task proceeds.

## Failure modes

- A provider returns an error: preserve its current per-source error representation while joining
  the other provider results.
- The request context is cancelled: stop waiting and return according to the existing handler/job
  contract without leaving background work or a shared cache flight stranded.
- Multiple callers miss or force-refresh together: preserve the existing single-flight cache
  behavior and ensure a stale completion cannot overwrite a newer snapshot.
- Docker is unavailable: preserve the current unavailable Docker summary and do not make other
  measurements wait on a Docker-specific retry.
- Filesystem capacity is unavailable or times out: return an unavailable capacity card and continue
  loading policy, history, quarantine, and overview independently.
- A dependency target is a symlink, escapes the workspace root, or has ambiguous ownership: skip it
  and record a warning; never follow it or fall back to arbitrary recursive deletion.
- A task becomes active or is restored during dependency pruning: cancel/skip the affected target,
  leave it intact, and allow the maintenance job to retry safely.

## API surface

The existing `GET /api/v1/system/storage` and `POST /api/v1/system/storage/analyze` contracts remain
the public entry points for cached and forced overview reads. Add a lightweight
`GET /api/v1/system/storage/disk` response with `path`, `total_bytes`, `used_bytes`,
`available_bytes`, `used_percent`, `available`, and an optional `warning`. Extend persisted storage
settings with the workspace dependency-cleanup boolean, and expose `workspace_dependencies` as a
valid resource name for the existing `POST /api/v1/system/storage/run` selection field. No other
route or API contract changes are required.

## Persistence guarantees

The disk-capacity response is read-only and is not persisted or included in the 15-minute overview
cache. The dependency-cleanup preference is persisted with the existing install-wide storage
settings object and defaults to false. Maintenance history continues to persist cleanup results,
including dependency counts/bytes and scheduled `skipped_busy` outcomes. Overview snapshots retain
their existing TTL and invalidation rules.

## Out of scope

- Changing the 15-minute cache TTL, cache ownership, or refresh semantics.
- Parallelizing directory traversal inside an individual workspace or Go-cache tree; that can be a
  separate bounded-worker optimization if the post-change measurements show it is still dominant.
- Changing storage ownership, full-workspace grace/retention rules, quiet-period semantics, resource
  labels, or task preemption behavior.
- Allowing arbitrary dependency paths, shell commands, generic `vendor`/`target`/build-directory
  deletion, or host-wide package-manager cache cleanup.
- Automatically pruning on the disk-percentage threshold; the progress bar is informational only.
- Replacing the existing progressive-loading page composition; the new capacity card is an
  independently loaded addition and uses the existing mobile one-column pattern.
