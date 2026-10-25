---
status: draft
system: agents
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-003
created: 2026-09-26
updated: 2026-09-26
owners:
  - Kandev
---
# Harness Self-Update System Design

## Purpose and boundaries

The `agents` system owns the operator-facing runtime-update contract. This
design adds the harness-owned updater flavor to the existing Settings update
surface without making the harness an npm-managed launch runtime. The
pinned-runtime flavor remains defined by
[`runtime-updates-01.md`](runtime-updates-01.md),
[`runtime-updates-02.md`](runtime-updates-02.md), and
[`runtime-default-activation.md`](runtime-default-activation.md).

The first harness-owned updater is Oh My Pi (`omp-acp`). Its trusted metadata
package is `@oh-my-pi/pi-coding-agent`; its self-update argv is `omp update`;
its ACP probe remains `omp acp`. OMP's existing `BuildCommand`, `Runtime().Cmd`,
`InferenceConfig`, `InstallScript`, session configuration, and passthrough
command do not change. In particular, OMP does not implement
`ManagedNPMRuntimeAgent`, does not enter the managed npm pin catalogue, and does
not acquire an npx launch path. This preserves the current Docker, Sprite,
remote, and local command behavior.

The harness owns artifact installation and verification. Kandev's Settings
job runs the harness updater, then ACP-probes the local harness and publishes
capabilities only after the probe succeeds. It cannot undo an in-place update
if that probe fails; it retains the prior capability catalogue and reports the
failure. See [ADR-2026-09-26](../../../decisions/2026-09-26-harness-owned-runtime-updates.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-RUNTIME-UPDATES-003` | [Components and responsibilities](#components-and-responsibilities), [Data and contracts](#data-and-contracts), [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery), [Security](#security) |

## Components and responsibilities

- `agents.HarnessUpdateAgent` is an optional built-in agent capability. Its
  `HarnessUpdateSpec` carries the trusted metadata package and the full update
  argv. The capability is separate from `ManagedNPMRuntimeAgent`; implementing
  it does not change launch command construction.
- `agents.OmpACP` declares package `@oh-my-pi/pi-coding-agent` and update argv
  `omp update`. Its probe command is its existing ACP runtime command, `omp
  acp`.
- `agent/settings/controller` adapts both update flavors into a controller-
  internal update target. Existing HTTP handlers and the
  `RuntimeUpdater`/`AgentUpdateJobStore` maintenance boundary remain shared.
  The harness target has no version-selection store or npm cache repair path.
- Runtime metadata cache and singleflight keys include both update mode and
  package. This keeps the same package from sharing status semantics between
  pinned and self-update targets.
- `RuntimeUpdateDTO`, status, preview, and job DTOs carry the closed
  `update_mode` values `pinned` and `self_update`. The self-update mode exposes
  the trusted package as informational metadata, the ACP-reported current
  version, and the upstream stable latest as a reference; it does not claim a
  Kandev default, active selection, or update target.
- The Agents settings page composes `useAgentRuntimeUpdates` with
  `useAgentRuntimeUpdateStatuses`. A terminal approval response with an empty
  `job_id` and `operation: "up_to_date"` is returned to the dialog without
  calling `upsertAgentUpdateJob`; the page starts a status refresh for every
  such response without awaiting it. Normal job responses remain in the shared
  job store and refresh status through the existing successful-job path.
- `refreshRuntimeUpdateStatuses` owns status reads at the shared Settings store.
  It shares an active request, queues one forced refresh behind it, and keeps
  the last successful status map when a request fails. Terminal update jobs
  trigger this shared refresh path.
- `AgentRuntimeUpdateControl` reuses the existing update trigger, responsive
  dialog/drawer, job phases, output stream, and result states. The self-update
  mode hides `RuntimeVersionPicker` and shows the stable reference, or an
  unknown value, with copy that explains the updater follows its configured
  channel and may install a different version. Self-update `update` and
  `repair` operations are actionable without a target. An empty-ID terminal
  `up_to_date` response is rendered from dialog-local state without registering
  or polling it as a job. Clear that state when the dialog resets or the user
  selects another target or default.

## Data and contracts

`HarnessUpdateSpec` contains:

- `Package`: non-empty trusted npm package name used only to resolve registry
  version metadata. The backend fetches its packument over HTTPS from the fixed
  `registry.npmjs.org` endpoint; it does not invoke `npm view` or require the
  `npm` executable. The package is not a command argument and is never supplied
  by an API request.
- `UpdateCommand`: non-empty trusted argv for the harness's own updater. OMP
  declares `[]string{"omp", "update"}`. It is executed directly, not through a
  shell.

The update mode is an explicit wire field on runtime-update, status, preview,
and job representations. `pinned` retains all existing exact-version behavior.
`self_update` carries no selectable version catalogue, active selection, or
default-generation identity. Its stable latest metadata comes from the
`dist-tags.latest` field of the trusted package's packument fetched directly
over HTTPS from the fixed npm registry endpoint, then validated as a stable
version. This lookup does not depend on the `npm` CLI. The response body is
limited to 16 MiB. For status comparison, the effective version is the latest
ACP capability version reported for the local installation. If that version
is unknown, check state is `unknown`; if it is older than the registry stable
latest, check state is `update_available`; if it is equal to or newer than
stable latest, check state remains `unknown` because the configured channel is
not known. Stable latest is a reference, not a source of truth for whether the
trusted updater can update the installed channel.

The self-update preview returns the resolved stable latest in
`stable_latest_version` as reference-only metadata and leaves `target_version`
unset; `stable_latest_version` is never an approval input or update-command
argument. It includes the trusted update argv for review and an empty
`available_versions` list. An explicit target-version request or `use_default`
request is rejected for this mode. For an enqueued job, `current_version`
is the post-update ACP-reported `AgentVersion`. Kandev does not persist a
version selection.

The existing `POST /api/v1/agent-update/{agent}` route keeps its pinned-runtime
request forms. A self-update approval sends an empty JSON object (`{}`); the
backend derives update mode from the built-in agent capability, not request
input. A target-free request is accepted only for a self-update agent. For that
mode, a non-empty `target_version` or `use_default: true` is rejected.

Self-update approval does not compare the current version with the stable
reference. It revalidates the built-in agent capability, then creates a job so
the trusted updater can decide whether its configured channel has a new
version. A registry metadata failure does not block approval. Stable latest is
never an approval input.

## Control flow

1. The discovery projection exposes a runtime-update DTO only when the agent is
   available and implements either `ManagedNPMRuntimeAgent` or
   `HarnessUpdateAgent`. For a harness-owned updater, the DTO includes its mode,
   trusted metadata package, and current ACP-reported version.
2. The status endpoint collects both target flavors. For a harness-owned
   updater, it fetches trusted package metadata directly from the fixed HTTPS
   npm registry endpoint, without invoking the `npm` executable. One package
   lookup failure yields an `unknown` entry and does not fail the other entries.
   Stable latest newer than the ACP version is an update hint. An equal or
   older stable version produces `unknown`, because the installed channel may
   have a newer release.
3. Preview resolves the stable latest version for display when available and
   classifies the harness as `update` or `repair` (the current version is
   unknown or invalid). A registry failure leaves the stable reference unknown
   but does not block preview. The response leaves `target_version` unset and
   includes the trusted update argv for review.
4. Approval revalidates the built-in agent capability, then enqueues an update
   or repair job without comparing stable metadata. The job runs the trusted
   command without selecting a version or channel, streams its output, and
   invokes the agent's ACP probe command. The command follows the harness's
   configured channel, so the probed version may differ from
   `stable_latest_version`. If a successful command leaves the ACP-reported
   version unchanged, the job fails, retains the updater's output, and does
   not publish the probe result.
5. A changed successful probe publishes the new capability catalogue and
   records the reported current version in the job/status projection. A failed
   command, failed probe, or unchanged probe version fails the job and does not
   publish capabilities or write a Kandev selection.

The maintenance target is the Kandev host installation, matching the existing
Update agent boundary. It does not install or prepare a copy in a task's Docker,
Sprite, SSH, or Kubernetes environment.

## Failure and recovery

- Registry metadata failure returns an unknown status for OMP and an unknown
  stable reference in preview. The trusted update action remains available.
- An `omp update` failure, including the harness refusing a Nix-owned
  installation, fails the job and retains the updater's output. Kandev does not
  substitute npm, Bun, Homebrew, mise, or a binary-download recipe.
- A zero exit code with an unchanged ACP-reported version fails the job and
  retains the updater's output. This includes an updater that declines an
  unsupported installation but exits normally.
- A successful harness update followed by an ACP probe failure fails the job
  and retains the prior advertised capability catalogue. The harness has
  already replaced its installation; Kandev does not claim rollback. Recovery
  is through the harness's own updater, `omp update --force`, or the operator's
  installation manager.
- A successful ACP probe is the capability-publication boundary. Active agent
  sessions continue with their existing processes, as with pinned updates.

## Persistence

Harness-owned updates do not create or modify managed-runtime version
selections, default-generation records, or npm execution-cache entries. The
current version comes from the host capability cache and the post-update probe.
Job retention and streamed output use the existing update-job store and
WebSocket events.

## Security

Only built-in agent metadata supplies the metadata package, update argv, and
ACP probe command. The HTTP request remains limited to the agent name and
existing request fields; harness mode rejects version selection and default
reset. Commands are direct argv with no shell interpolation. The public status
lookup is read-only; it does not execute the harness updater. The explicit
maintenance action is the only Kandev path that runs `omp update`.

## Observability

Existing update-job status, output chunks, terminal result, and WebSocket
notifications expose the operation. No new metric is required. The job output
is the source for package-manager detection and updater failure details; do not
add package-manager names, task identifiers, or version strings as metric
labels.

## Related decisions

- [Delegate updates to harness-owned updaters](../../../decisions/2026-09-26-harness-owned-runtime-updates.md)
- [Validate and persist pinned managed-runtime selections](../../../decisions/2026-08-12-validated-managed-runtime-version-selection.md)
