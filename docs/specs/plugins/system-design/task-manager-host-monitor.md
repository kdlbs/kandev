---
status: draft
system: plugins
requirements:
  - REQ-PLUGINS-TASK-MANAGER-MONITOR-001
  - REQ-PLUGINS-TASK-MANAGER-MONITOR-002
  - REQ-PLUGINS-TASK-MANAGER-MONITOR-003
created: 2026-09-05
owners:
  - kandev
---

# Task Manager Host Monitor System Design

## Purpose and boundaries

The official `kdlbs/kandev-plugin-task-manager` repository owns this feature.
It already ships a Go process sampler, an authenticated `usage` webhook, a
native UI bundle, a `main-top-bar` contribution, and a Task Manager dialog.
This design adds a lightweight ambient summary path and personal display
settings while preserving the detailed task sampler.

Kandev remains a generic host. The design consumes the existing manifest
`config_schema`, `capabilities.user_state`, `host.storage`,
`host.useSettingsSaveContributor`, `plugin-settings`, and `main-top-bar`
contracts. No change to the Kandev monorepo's runtime or public plugin API is
required. The built-in monitor remains independently owned by the UI and
system-metrics subsystems until a later removal proposal.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-PLUGINS-TASK-MANAGER-MONITOR-001` | [Summary contract](#summary-contract), [Metric semantics](#metric-semantics), [Top-bar rendering](#top-bar-rendering) |
| `REQ-PLUGINS-TASK-MANAGER-MONITOR-002` | [Personal preference model](#personal-preference-model), [Settings surface](#settings-surface), [Synchronization and lifecycle](#synchronization-and-lifecycle) |
| `REQ-PLUGINS-TASK-MANAGER-MONITOR-003` | [Operator configuration](#operator-configuration), [Collection flow](#collection-flow), [Failure and recovery](#failure-and-recovery) |

## Existing baseline

- `server/sampler.go` computes per-process and per-task CPU as a delta of
  cumulative CPU seconds. One core is 100%, and `snapshot.cpu_cores` exposes
  the logical-core denominator.
- `server/sampler.go` refreshes Linux PSS on a five-second TTL. PSS dominates
  the sampling cost, so the ambient chip must not call the full `usage`
  webhook.
- `ui/bundle.js` currently polls `usage` every four seconds for the top-bar
  chip and every 1.2 seconds while the dialog is open. It sums task CPU for the
  chip. The `.ktm-fill` element currently has no matching stylesheet rule, so
  progress-fill behavior needs explicit regression coverage rather than being
  assumed from the existing track.
- Kandev's built-in monitor reports whole-host CPU on a 0%-to-100% scale plus
  host memory, disk, CPU temperature, and one-minute system load. Platform
  collectors can report an individual metric as unavailable.

## Components and responsibilities

### Plugin backend

- A new host-summary collector reads machine CPU, memory, filesystem, CPU
  temperature, and one-minute load through platform-specific files. It returns
  raw values and per-metric availability rather than presentation strings.
- The existing task sampler gains a CPU-only summary operation. It reuses the
  existing process identity and cumulative-CPU baseline under the plugin's
  sampling mutex but skips `memoryBytes`, PSS, title annotation, process
  serialization, and task history concerns.
- `taskManagerPlugin.HandleWebhook` keeps `usage` backward compatible and adds
  the authenticated `summary` key. It validates the bounded summary request,
  loads cached operator configuration from `Host.GetConfig`, invokes only the
  requested collectors, and returns partial success.
- The plugin process caches successfully loaded configuration because a config
  save restarts an active plugin. A Host-not-ready or transient config read is
  retryable and never permanently freezes defaults through a one-shot failure.

### Plugin frontend

- A shared preference controller owns the confirmed value, draft value,
  `updatedAt`, loading/error state, and subscribers for the current browser
  generation. Top-bar and settings components consume the same controller, so
  a successful same-tab save updates both even though `host.storage.subscribe`
  suppresses the writer's own echo.
- A `plugin-settings` component renders the personal display card and registers
  one `host.useSettingsSaveContributor`. It uses host-owned Settings/Card,
  Switch, Checkbox, Select, Input, Button, Tooltip, and accessible label
  primitives.
- The `main-top-bar` component derives the requested metric families from the
  confirmed preference, polls `summary` at the interval returned by the
  backend, and renders one compact clickable contribution. It calls
  `host.api.fetch("webhooks/summary", ...)`, which the host scopes to this
  plugin's authenticated webhook route. It never starts a timer when no
  reading is enabled.
- The existing modal continues to poll `usage` only while mounted. It is not
  restyled or reinterpreted by ambient display preferences.

## Operator configuration

`manifest.yaml` adds two required schema properties:

```yaml
config_schema:
  type: object
  properties:
    refresh_interval_seconds:
      type: integer
      title: Ambient refresh interval
      description: Seconds between Task Manager top-bar updates.
      minimum: 1
      maximum: 300
      default: 5
    disk_path:
      type: string
      title: Disk path
      description: Filesystem path measured by the disk reading.
      default: /
  required: [refresh_interval_seconds, disk_path]
```

The generic schema form and backend validator enforce the inclusive
1-to-300-second range through their shared numeric minimum/maximum subset.
`disk_path` must be non-empty and is never accepted from the summary request.
An invalid cached refresh interval returns a bounded configuration error. An
invalid cached disk path makes only the disk metric unavailable; CPU, memory,
temperature, and load requests continue without sampling an arbitrary fallback
path. Config is install-wide and administrator-owned; the existing plugin save
and restart lifecycle applies.

The manifest also declares `capabilities.user_state: true` and a second
authenticated webhook key, `summary`. No new capability is required for
operating-system reads because the plugin process already runs as trusted host
code.

## Personal preference model

The frontend stores one JSON object at
`("instance", "profile", "topbar-settings-v1")` through `host.storage`:

```ts
type TopbarSettingsV1 = {
  version: 1;
  metrics: Array<
    | {
        id: "cpu";
        enabled: boolean;
        mode: "host_relative" | "tasks_relative" | "tasks_per_core";
        show_bar: boolean;
      }
    | {
        id: "memory";
        enabled: boolean;
        unit: "percent" | "gb";
        show_bar: boolean;
      }
    | {
        id: "disk";
        enabled: boolean;
        show_bar: boolean;
        visibility: "always" | "threshold";
        threshold_percent: number;
      }
    | { id: "cpu_temperature"; enabled: boolean }
    | { id: "system_load"; enabled: boolean }
  >;
};
```

Defaults are CPU first, enabled, `tasks_per_core`, with a bar. Memory, disk,
temperature, and load follow disabled. Normalization removes duplicate known
IDs, validates enum values and the disk threshold, preserves the user's known
order, and appends newly introduced known IDs in default order. An absent,
malformed, or unsupported version produces defaults in memory; it is not
written until the user explicitly saves.

Settings writes use `ifUnmodifiedSince`. On a conflict, the controller keeps
the local draft, refetches the confirmed record and timestamp, and shows a
retryable conflict rather than silently overwriting another client. Discard
restores the last confirmed value.

## Summary contract

The UI calls the authenticated `summary` webhook with a small JSON request:

```json
{
  "metric_ids": ["cpu", "memory", "disk"],
  "cpu_source": "tasks"
}
```

`metric_ids` is de-duplicated and restricted to `cpu`, `memory`, `disk`,
`cpu_temperature`, and `system_load`. `cpu_source` is required only when CPU
is requested and is either `host` or `tasks`. The request has no path, interval,
unit, threshold, or arbitrary collector input.

The response is presentation-neutral:

```json
{
  "sampled_at": "2026-09-05T16:00:00Z",
  "refresh_interval_seconds": 5,
  "cpu_cores": 16,
  "metrics": {
    "cpu": {
      "available": true,
      "source": "tasks",
      "core_percent": 273.0,
      "relative_percent": 17.1
    },
    "memory": {
      "available": true,
      "used_bytes": 11682311045,
      "total_bytes": 34359738368,
      "percent": 34.0
    },
    "disk": {
      "available": true,
      "path": "/",
      "used_bytes": 450971566080,
      "total_bytes": 1000204886016,
      "percent": 45.1
    }
  }
}
```

Temperature adds `celsius`; system load adds `one_minute`. Every metric object
has `available` and can carry a bounded `error`. Omitted metric IDs are not
sampled and do not appear. HTTP 200 with partial metric errors distinguishes a
collector failure from invalid input, invalid configuration, cancellation, or
an unavailable plugin runtime.

## Metric semantics

- Host-relative CPU compares aggregate idle and total host CPU counters across
  a sampling window. The result is clamped to 0%-100%.
- Task per-core CPU sums the deltas for attributed Kandev processes. Task
  relative CPU divides that sum by the logical-core count and clamps the result
  to 0%-100%. A task-scoped summary uses the existing start-key protection,
  counter regression clamp, identity cache, stale-baseline reset, and serialized
  sampling window.
- Host memory uses physical memory used and total. Linux honors a meaningful
  cgroup limit when the plugin is itself container-constrained; otherwise it
  uses machine memory. Absolute UI values use 1024-based units and the existing
  `GB` product label for compatibility.
- Disk reports the filesystem capacity containing the configured path. It uses
  capacity available to the current user, matching the built-in monitor's
  semantics.
- Temperature and load use the current platform's available system interfaces.
  Unsupported metrics remain explicit unavailable records.

Platform collectors are implemented from operating-system interfaces and public
documentation. Kandev's built-in collector is an acceptance oracle, not source
to transplant: the monorepo is AGPL-3.0 while the plugin is MIT, so copied code
would blur the plugin's license boundary.

## Collection flow

1. The top-bar component loads confirmed personal preferences. If no metric is
   enabled, it renders `null` and performs no request.
2. It immediately requests enabled metric IDs and the selected CPU source. The
   server loads cached operator config and samples only those families.
3. Host CPU and task CPU establish a short baseline when none is usable, then
   take the second reading needed for a rate. Other metrics are instantaneous.
4. The response supplies the effective refresh interval. The UI schedules one
   next poll after the current request settles; it does not use overlapping
   `setInterval` requests.
5. Opening the dialog mounts its independent detailed `usage` consumer. Closing
   the dialog cancels its request/timer and leaves only the ambient summary
   cadence.

One serialized backend sampling boundary protects cumulative CPU baselines.
Cancellation releases waits. Requests never create an unbounded goroutine for a
potentially blocking disk call.

## Settings surface

The owner-scoped `plugin-settings` card lists all five readings. Each row has a
drag handle, enabled control, relevant formatting controls, and explicit Move up
and Move down actions. Pointer drag uses a visible insertion target and commits
only to draft state. Keyboard controls keep focus on the moved row. Disabled
rows remain orderable so re-enabling returns them to their selected position.

The Disk row places a keyboard-focusable information icon beside its label.
Hovering the icon or focusing it shows the same tooltip: "Reports capacity for
the filesystem containing the configured path. It reads filesystem metadata;
it does not scan files or directories. The visibility threshold hides this
reading from the top bar but does not stop sampling." The icon has the
accessible name `About disk monitoring`, and the tooltip is associated with it
for screen readers. This help belongs to the plugin-owned personal settings
row; the administrator-owned disk-path field retains the manifest description
rendered by Kandev's generic configuration form.

The card explains that display choices are personal. The schema-driven fields
below it remain the administrator-owned Sampling section. Personal changes use
the page's shared Save/Discard bar and do not restart the plugin; install-wide
schema changes use the existing plugin config contributor and restart behavior.

All new strings in the native UI bundle use `registry.registerTranslations`
and `host.i18n`. English is the fallback catalog; Portuguese and Kandev's
Simplified and Traditional Chinese locales ship with the official plugin
package. The install-wide schema labels remain manifest data rendered by the
host's generic configuration form.

## Top-bar rendering

The contribution is one button with ordered metric segments and one click target
that opens Task Manager. Desktop uses a compact height consistent with the
existing contribution. Mobile uses the host's horizontal action strip and a
minimum 44 px touch height without adding an inner scroller. Because this is a
rich status control rather than an icon action, it marks its root with
`data-main-top-bar-rich`; the host preserves its dimensions while continuing
to normalize unmarked icon buttons to 32 px.

The frontend view model derives each bar's capacity percentage: CPU uses the
summary's `relative_percent`, and memory and disk use their `percent` fields.
Widths are clamped independently from displayed values. A task per-core CPU
value can display above 100% while its bar represents the task share of total
machine capacity. Memory in GB retains a capacity bar when enabled.
Temperature and load have no bar control. Every segment has an accessible
name, a formatted value, and tooltip/detail text that names source, units,
capacity, sampled time, and any error.

Disk threshold filtering occurs after availability is known. Below-threshold
disk is omitted without leaving spacing. An unavailable disk remains visible so
configuration and collector failures are not mistaken for healthy low usage.

## Synchronization and lifecycle

`host.storage.subscribe` listens for the settings key in other tabs/clients and
refetches it. Incoming changes replace confirmed state but do not erase a dirty
local draft; the settings card reports that the saved value changed elsewhere.
The shared controller publishes a successful same-tab save directly.

`initialize` creates one generation-scoped controller and registers translations,
settings, keybinding, and top-bar contributions. `destroy` aborts requests,
unsubscribes storage listeners, clears scheduled timeouts, closes any modal
handle, and removes the injected style. Re-enable starts from a fresh controller
and authoritative storage read.

## Failure and recovery

- The top bar renders nothing before its first successful settings and summary
  load, avoiding a false 0% value.
- A transient summary failure keeps the last successful snapshot, marks its
  details stale, and retries at the configured cadence. If no snapshot ever
  succeeds, the compact contribution remains hidden and the settings card shows
  a retryable status.
- A single collector failure produces one unavailable metric and leaves the
  response, schedule, and other readings intact.
- An invalid summary body returns 400. An invalid cached refresh interval
  returns a bounded 500/configuration error until an administrator corrects it.
  An invalid cached disk path produces an unavailable disk metric while other
  requested metrics remain usable.
- A failed personal-settings read disables Save. A failed write keeps the draft
  dirty. A conflict never discards the local draft.
- Unmount, plugin reload, disable, and uninstall fence late promise callbacks so
  they cannot recreate timers or mutate the next plugin generation.

## Security and privacy

- Both webhooks remain authenticated. The summary request is an allowlisted
  selector and never accepts a filesystem path.
- Disk path and cadence are administrator-owned config. Personal settings are
  isolated by Kandev's authenticated `plugin_user_state` rows.
- Responses contain aggregate machine readings and task-attributed totals, not
  new process command lines beyond the existing detailed `usage` response.
- Error strings are bounded and do not include environment blocks, credentials,
  or arbitrary file contents.

## Verification strategy

Pure calculations, preference normalization, threshold rules, ordering, and
formatting receive deterministic unit tests. Backend tests inject clocks,
platform readers, and process tables. The browser harness exercises real React
mounts, storage synchronization, Save/Discard, pointer and keyboard ordering,
disk-help hover/focus behavior and accessible naming, poll rescheduling,
responsive containment, and modal activation. Artifact tests inspect the
packaged manifest, UI modules, platform executable, and generated checksums
before a disposable Kandev smoke test.

## Related decisions

- [Resource Metrics Sampling](../../../decisions/0017-resource-metrics-sampling.md)
- [Host-Provided Per-User Plugin Storage](../../../decisions/2026-08-01-per-user-plugin-storage.md)
