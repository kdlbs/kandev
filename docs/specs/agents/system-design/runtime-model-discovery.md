---
status: current
system: agents
created: 2026-10-04
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-003
---

# Runtime context for model discovery

## Purpose and boundaries

Agents owns runtime identity, profile discovery, and runtime management.
This design attaches an observation to the model snapshot and refreshes open profiles after runtime activation.
Runtime management remains in the existing Agents runtime settings.
It extends [runtime updates](runtime-updates-01.md) without changing version selection, candidate validation, or installation ownership.
[Profile discovery](profile-capability-discovery.md) continues to own launch validation, authorization, cache identity, and draft reconciliation.
[Runtime notifications](runtime-update-notifications.md) continues to own shared release status and automatic consent.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-AGENTS-RUNTIME-UPDATES-003 | Observation contract, Collection, Snapshot flow, Model settings, Failure and security |

## Observation contract

Add an optional `runtime_info` field to `DynamicModelsResponse` and its web counterpart in `lib/types/http-agents.ts`.
Carry the same typed observation through utility `ProbeResponse`, hostutility `AgentCapabilities`, and `ProfileCapabilityResult`.
Keep existing fields and context-free callers compatible.

The observation contains `scope: host`, `observed_at`, and a bounded `components` list.
Each component has a `role` (`bridge` or `provider`), a trusted display name, and an optional trusted package identity.
It also has `source` (`managed`, `bundled`, `external`, or `unknown`) and `owner` (`kandev`, `external`, or `unknown`).
Optional `effective_version` names configured managed selection. Optional `observed_version` names verified runtime evidence.
An absent version represents unknown evidence. Versions never contain raw command output.
The enclosing `context_revision` binds the observation to the same models, options, and launch context.

The bridge's ACP `agentInfo.version` is bridge evidence. It cannot identify an underlying Codex executable or Claude SDK version.
Do not substitute a latest release, dependency range, global CLI version, or saved selection for an observed version.
Keep the configured version and observation distinct. Configuration does not prove an observed version.

Runtime update status remains separate and install-wide. It describes the managed package, not a profile-specific external dependency.
Keep strict comparison and unknown release states in `ListAgentUpdateStatuses`.
No new endpoint, persisted observation, registry query, or updater mechanism is required.

## Collection

Define optional trusted observation descriptors beside registered agent metadata in `internal/agent/agents`.
Codex ACP declares `@openai/codex` and its `CODEX_PATH` override.
Claude ACP declares `@anthropic-ai/claude-agent-sdk` as a bundled SDK component.
Label that component as the SDK. Do not call its package version the Claude Code CLI version.
Other agents retain generic primary-runtime evidence. Missing descriptors mean unknown underlying components, not an inferred provider relationship.

Collect evidence inside the existing agentctl utility probe, after the provider has initialized.
Use the actual sanitized child environment, resolved command, work directory, and trusted descriptors.
The backend supplies descriptors from registration. A browser cannot supply packages, executable recipes, or arbitrary commands.

For an exact managed npm command, resolve the npm cache root with the same project prefix and child environment.
Reuse the read-only `npm config get cache` recipe from `hostRuntimeUpdater.npxCacheRoot` through a narrow shared helper if needed.
Do not import the settings controller into agentctl.
Use `NpxExecutionCacheKey` and verify the installed bridge manifest matches the trusted package and selected exact version.
Resolve only the declared dependency from that bridge's installed location using Node-compatible nearest dependency lookup.
Handle both nested and hoisted dependencies inside that exact execution tree.
Read the installed dependency manifest, not registry metadata or dependency ranges.
Missing, invalid, replaced, or inaccessible evidence yields an unknown version.
Do not scan other npm trees or load executable modules to inspect package metadata.

For a nonempty effective `CODEX_PATH`, mark the Codex provider external.
Resolve its executable with the probe's environment and work directory. Do not use the backend's unrelated PATH.
Run only that executable with the fixed `--version` argument through the existing owned process lifecycle.
Use a two-second child deadline, bounded output, and a strict Codex version parser.
Never invoke a shell formed from the environment value. Cover executable paths with spaces and supported Windows command shims.
Version failure preserves external source identity with an unknown version and never falls back to bundled Codex.
An empty profile override follows existing launch precedence, including an inherited nonempty override.

A command prefix can change the executable or environment after Kandev starts it.
When that context cannot be attested, report the configured bridge separately and mark provider observation unknown.
Do not run a diagnostic outside the prefix and claim that it describes the wrapped provider.
Native/custom/virtual protocols retain their existing model behavior and expose only verified evidence they already provide.

All additional inspection shares a two-second total deadline within the existing discovery timeout.
Bound each manifest to 64 KiB and version output to 4 KiB. Cleanup owns and terminates descendant processes.
Inspection cannot prepare or repair npm trees. Metadata failure cannot turn a successful model probe into a failed catalog.
Recheck manifest identity before publication when its files change during inspection.

## Snapshot flow

`ACPInferenceExecutor.Probe` collects runtime evidence with its model result.
`capabilitiesFromProbe` and `FetchProfileDynamicModels` preserve that evidence instead of dropping the observed bridge version.
The host utility stamps configured effective package identity from the command captured before the probe.
Do not reread the active selection after the probe and attach a successor version to older models.

Reuse existing profile generation checks for cache reads, writes, and response publication.
`PublishCapabilities` already invalidates model-option and profile caches after validated activation.
An older in-flight observation must fail the same generation check as an older model list.
Explicit refresh recollects versions, including an external executable replaced at the same path.
An ordinary cached snapshot retains its original observation time.

`useProfileModelCapabilities` keeps runtime information in the same component-local response as model choices.
Changing launch settings makes both stale. Navigation and refresh sequence guards reject both together.
Do not merge an agent-wide runtime observation into an authoritative profile response.

Subscribe the open concrete-profile discovery hook to `updateJobs.byAgent` through the existing store.
Refresh once for a newly completed successful job for that agent, including already-authorized automatic jobs.
Record the current terminal job on mount so historical success does not trigger duplicate discovery.
Deduplicate by job ID. Pending, failed, other-agent, and duplicate events do not refresh the profile.
On success, immediately invalidate local request sequences and mark the snapshot loading.
Then probe the current complete draft and resolve dependent options through the existing refresh path.
An unsaved launch edit remains unsaved. Late pre-update results cannot replace the new snapshot.
A refresh failure after activation reports discovery failure separately from the successful managed update.
Never publish the default-context candidate catalog directly into a configured profile.

## Model settings and responsive composition

Profile model settings contain model and mode controls, discovery status, refresh, and model-specific options.
Do not show runtime identity, versions, ownership, release status, or update actions in this region.
Do not add a tooltip or collapsed runtime panel as a replacement.
Runtime evidence remains optional discovery metadata and does not create a presentation requirement.

Runtime information and manual or managed update controls remain in Settings > Agents > Agent runtime updates.
Reuse its existing policy rows, administrator restrictions, desktop dialog, and phone drawer.
Provider ownership and update authority retain their existing contracts.

The profile hook observes successful runtime activation independently of runtime presentation.
It refreshes the current complete draft and preserves selected models and unsaved launch settings.
Failures retain the prior catalog and use the existing discovery error and retry states.

Desktop and phone use the same profile discovery state and existing page scroller.
The nearest mobile exemplar is the existing profile capability controls.
Phone and coarse-pointer refresh controls retain at least 44px touch targets.
No new panel, scroll region, disclosure, or action is introduced.
Desktop and phone E2E must prove that runtime metadata does not produce a profile runtime panel.
They must also prove that a successful update in another tab refreshes the current draft without changing its selection.

## Failure and security

Preserve existing profile execution authority and secret scope checks before any version process starts.
Return trusted names, source enums, bounded parsed versions, and timestamps only.
Do not return raw paths, environment values, secret contents, command prefixes, arguments, or provider stderr.
Log only agent identity, elapsed inspection time, and a closed failure class when diagnostic logging is needed.
No new metrics, schema migration, or long-lived process is required.

Unknown evidence stays useful beside a successful catalog. It cannot authorize a guessed update command.
An unavailable release source does not disable existing explicit version management.
Active task sessions, separately installed login CLIs, passthrough CLIs, and remote installations retain their ownership.

## Related decisions and delivery

- [Validated managed version selection](../../../decisions/2026-08-12-validated-managed-runtime-version-selection.md)
- [Runtime update authority](../../../decisions/2026-10-01-agent-runtime-update-authority.md)
- [Implementation package](../../../plans/model-discovery-runtime-visibility/plan.md)

The existing decisions settle activation ownership. This extension adds read-only observations without changing those boundaries.
