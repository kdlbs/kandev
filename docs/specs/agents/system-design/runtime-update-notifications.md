---
status: current
system: agents
requirements:
  - REQ-AGENTS-RUNTIME-NOTIFY-001
  - REQ-AGENTS-RUNTIME-NOTIFY-002
created: 2026-10-01
updated: 2026-10-02
owners:
  - Kandev
---
# Agent Runtime Update Notifications and Automation Design

## Boundary and requirement mapping

Agents owns runtime identities, supported activation, and install-wide policy. Notification service owns recipient preferences and durable delivery claims. UI consumes the same status projection on every route and in Settings.

| Requirement | Design |
| --- | --- |
| REQ-AGENTS-RUNTIME-NOTIFY-001 | Capabilities, sources, scheduler, notification delivery, responsive UI |
| REQ-AGENTS-RUNTIME-NOTIFY-002 | Policy persistence, update admission, candidate activation, recovery |

This design extends [managed updates](runtime-updates-01.md) without changing its exact-version selection or executor ownership. It supersedes that design's settings-only check placement and automatic-update exclusion. [ADR on automatic runtime ownership](../../../decisions/2026-10-01-agent-runtime-update-authority.md) records the authority boundary.

## Capabilities and source coverage

A shared capability resolver consumes trusted registered agent metadata. ManagedNPMRuntimeAgent supplies exact package/default metadata. Unmanaged structured npm commands supply only read-only package release discovery; custom commands never authorize installers. Native agents declare verified release sources and vendor guidance through an optional metadata capability. Unknown/custom/virtual cases return explicit unsupported capability. No provider-ID switch controls automatic eligibility.

Status adds display name, runtime identity, owner, mechanism, enabled/available flags, management capability, guidance URL, current version, automatic eligibility/policy, and last outcome to the existing AgentUpdateStatusDTO. Existing check_state and managed version fields remain compatible. Current unknown and source unknown are independent; a latest version alone never proves up_to_date. External npm runtimes compare a host observation only when it matches a published stable package version; an unmatched vendor/adapter observation remains unknown while the raw current and latest values remain independently available.

Only enabled, available runtimes query sources or automate. Registered but unavailable/disabled runtimes remain in status with no current claim. Strict stable SemVer comparison fails closed. Npm uses the existing RuntimeUpdater metadata boundary. Verified native GitHub releases use a bounded unauthenticated request to a trusted repository and reject drafts/prereleases/invalid versions. Opaque vendor channels expose guidance with no guessed latest release.

See the complete [coverage matrix](../../../plans/agent-runtime-notifications/coverage.md). The eight existing managed packages include Codex app-server as well as ACP adapters. Native OpenCode selected by PATH is external: npm ownership cannot be inferred, and its host version comes from the host probe rather than the npm default. The managed version control remains applicable to managed selections; native installations use vendor guidance. Separate Claude/Codex/Muse/Pi dependency and passthrough CLIs are named as outside the managed package's update result.

## Cache and scheduler

The controller's existing status cache keeps the six-hour success and fifteen-minute failure TTLs. Cache identity includes source type and locator. A shared single-flight lookup per source supplements the five-slot concurrency semaphore; slot waits and network/process operations respect cancellation. Status remains read-only.

One controller-owned scheduler runs a batch at startup and every fifteen minutes, after runtime and notification wiring, with Start/Stop and drained cancellation. Subscriber reconnect replays notices read-only and cannot restart automatic mutation after shutdown or restore quiesce. It reuses ListAgentUpdateStatuses and never creates a loop per agent. UI refreshes one shared status snapshot periodically for app-wide indicators and after terminal jobs; simultaneous consumers share the request and reject stale completions.

## Policy persistence and admission

The existing install-wide SystemSettings store persists one namespaced JSON policy per registered runtime identity, with enabled=false by default. Policy includes the trusted source identity, last attempted target, previous version, job/outcome identity and terminal result. A source change does not inherit consent. Settings uses an org.config.manage-gated PATCH endpoint plus the existing mutation interlock; status GET cannot change policy or enqueue a job.

A serialized automatic pass rechecks availability, capability, policy, and selection immediately before enqueue. It records the attempted target before mutation, so a restart or source failure cannot create repeated failed attempts. Existing maintenance admission prevents overlap with installs or manual updates. Explicit disable/re-enable clears the prior attempt. Manual update/default requests disable automation before changing the version.

Jobs carry automatic origin and previous version, immutable for a job ID. The frontend preserves these fields when same-job live updates or readiness snapshots omit them, and clears them for a replacement job. Automatic jobs require RuntimeCandidateUpdater, the verified RuntimeVersionResolver catalogue, and durable selection and never use the legacy Refresh-only/global-install path. A guard rechecks consent and the observed selection before preparation and under the same policy mutex as the activation commit. Opt-out or an intervening selection rejects activation. Candidate validation and selection persistence precede capability publication. Terminal observation persists outcomes outside the job-output retention window. Validation/admission failures before job creation also persist and announce their result immediately, after releasing the policy lock. A backend interrupted job is recorded as interrupted on restart. Its notice says the activation result could not be confirmed: a crash between selection persistence and outcome persistence must not imply success or failure.

## Notifications and recovery

Reuse notification service semantic delivery, update subscriptions (system.update_available), per-user provider ownership, and persistent delivery claims. Runtime payload identifies agent, display name, runtime source, previous/target versions, and outcome. Occurrence identity combines those identities with available/success/failure and target or job identity; Kandev release occurrence IDs remain independent. Local delivery preserves structured runtime payload so the frontend resolves copy at render time; native/external providers receive concrete agent/runtime copy.

Available updates and terminal outcomes are retried for disconnected local subscribers using persisted status/policy, while delivery claims suppress repeats. Source failures produce no notification. Failed automatic updates retain the old version and expose the existing version dialog for retry, rollback, or return-to-default. No automatic rollback after a later launch failure is introduced.

## Responsive UI

An app-wide indicator persists while a known newer runtime is available and links directly to Settings > Agents for the affected runtime. The existing toast bridge handles structured runtime notifications and links to the same destination. Notification preferences govern toasts, not the informational status indicator.

The Agents page exposes a runtime update section, showing named package/runtime, ownership, current/latest or unknown state, supported control or vendor guidance, and per-runtime automatic switch for eligible installed runtimes. Use Settings save coordination for policy drafts; discard and failed save retain authoritative baseline. Direct links focus the requested runtime. Managed update dialogs/drawers retain their existing version selection and recovery.

Phone entry uses a visible 44px control and direct settings route. Settings rows use one column, wrapping copy, full-width touch actions, and page-owned scrolling. The nearest exemplars are InstalledAgentCard, SettingsPageTemplate, and AgentRuntimeUpdateSurface's existing phone drawer. Temporary version selection stays in that drawer; persistent policy belongs on Settings, avoiding a second nested overlay. Desktop keeps 28px actions. All copy uses locale catalogs.

## Validation and operational limits

Deterministic fakes/temp installations cover multiple identities and npm/native sources; no global CLIs or credentials are changed. Source failures, single-flight concurrency/cancellation, default-off policy, restart persistence, opt-out during validation, failed activation, rollback, unchanged running sessions, and delivery deduplication/preferences are boundary tests. Focused desktop and phone E2E cover notifications/direct navigation, policy save/reload, manual guidance, and existing update surfaces. Staging an exact package does not guarantee every future prompt or transitive dependency; existing managed runtime recovery remains authoritative.
