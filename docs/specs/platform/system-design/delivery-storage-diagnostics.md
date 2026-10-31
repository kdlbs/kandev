---
status: draft
system: platform
updated: 2026-10-28
requirements:
  - REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001
  - REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002
  - REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003
---

# Delivery storage diagnostics system design

## Purpose and boundaries

Classify the storage problem at its source and preserve the classification through chat presentation.
Use the authenticated status path for live observations when journal writes cannot progress.

The [delivery design](durable-agent-delivery.md) retains event and submission authority.
The [recovery design](../../agents/system-design/session-recovery-failures.md) retains failure stamps and recovery controls.
Storage maintenance retains cleanup settings, eligibility, and execution.

This is an additive diagnostic contract. It does not add a second transcript or storage admission policy.
Existing delivery and recovery documents remain authoritative for their outcomes.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-001 | Source classification; Wire and persistence; Compatibility |
| REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002 | Live observations; Ownership and failure behavior |
| REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003 | Shared presentation; Mobile composition; Recovery actions |

## Current behavior

`journal.reserveDiskCapacity` returns `ErrJournalFull` for insufficient filesystem capacity.
Logical journal limits and oversized producer queue items can return the same sentinel.
Oversized journal events instead use `ErrStreamFull`, which also represents a stream limit.

`process.DeliveryHealth` already exposes `journal.Capacity` through `/api/v1/status`.
The backend `agentctl.StatusResponse` currently retains only state and two control booleans.
It drops the capacity object.

`persistLastAgentError` preserves typed `AgentErrorCause` records for other failure classes.
The shared recovery card selects its headline through `recoveryCopy`.
Unknown failures use the generic session recovery title.

The October 27 incident establishes host exhaustion and delayed cleanup.
It does not establish the exact first recovery action or every session's capacity at failure.
Committed artifacts must not contain that incident's private session records.

## Source classification

Add a typed journal storage error with a stable reason and optional failure measurements.
Keep `errors.Is` compatibility with the existing sentinels through wrapping.
Classify the rejected operation before another sample or retry can change its evidence.

| Reason | Authoritative source | Recovery guidance |
| --- | --- | --- |
| `disk_space_low` | Physical byte-capacity admission guard | Free space on the affected runtime filesystem |
| `filesystem_full` | Actual `ENOSPC` | Free allocation capacity on the affected filesystem |
| `disk_quota_exceeded` | Actual `EDQUOT` | Free quota or increase that filesystem quota |
| `journal_limit_reached` | Explicit logical journal limit | Inspect retained output and delivery progress |
| `stream_limit_reached` | Explicit logical stream limit | Inspect stream retention and delivery progress |
| `event_too_large` | Serialized event or producer item bound | Report the unsupported output size |
| `storage_write_failed` | Other source-owned storage write error | Inspect runtime storage without inventing a cause |

Do not classify a provider's prose or tool output as a host storage diagnostic.
Host process code supplies the diagnostic after adapter normalization.
An ordinary provider event cannot set or override it.

Physical measurements use the journal's filesystem and its existing capacity calculation.
Expose the actual required bytes, including reserve, headroom, and applicable allocation cost.
Do not label reusable bbolt pages as free filesystem bytes.
An OS allocation error can establish the reason without establishing a numeric free-space value.
`ENOSPC` alone does not establish byte exhaustion, because allocation can also exhaust inodes.
Use a general storage-full explanation unless a separate sample proves low byte capacity.

## Wire and persistence

Define `DeliveryStorageDiagnostic` in a new `internal/agentctl/types/streams/delivery_storage.go`.
The source wire type contains this bounded evidence:

| Field | Contract |
| --- | --- |
| `version` | `1`; unknown versions use generic presentation |
| `reason` | One closed reason from the source table |
| `observed_at` | UTC observation time |
| `disk_available_bytes` | Optional nonnegative integer |
| `disk_required_bytes` | Optional nonnegative integer for the rejected operation |
| `retained_bytes` | Optional nonnegative logical retention value |
| `limit_bytes` | Optional positive configured limit |
| `event_bytes` | Optional nonnegative serialized size for an oversized item |

Absence means unknown. Reject negative, non-integral, overflowing, and invalid combinations.
Browser normalization accepts numeric values only when `Number.isSafeInteger` succeeds.
Bound serialized diagnostics to 2 KiB and omit unknown fields during normalization.
The DTO contains no raw error, host path, prompt, payload, or arbitrary remediation URL.

Attach the optional diagnostic to host-generated `streams.AgentEvent` errors and delivery HTTP error bodies.
Extend `DeliveryHTTPError` to decode it separately from its raw body.
Preserve existing HTTP status, legacy error codes, and unsupported-peer handling.
Do not route a storage diagnostic through provider quota fallback or automatic provider retry.

Forward the same snapshot through `AgentEventPayload`, `watcher.AgentEventData`, and failure handling.
Normalize it into `AgentErrorCause` with operation `delivery` and code `delivery_storage`.
Its optional `storage` object carries the reason and validated measurements.
Backend normalization adds trusted location context from the owning executor.
Use location kinds `local_runtime`, `executor_runtime`, and `unknown`.
Use existing escaped executor labels, without exposing a connection address or runtime secret.

Update cause normalization, equality, status-summary copies, and WebSocket error projection together.
`LastAgentError.Causes` preserves the snapshot in existing session metadata.
Transcript metadata retains the same stamp and evidence for the existing history renderer.
No SQL schema or journal version change is required.

## Live observations

Add source-owned `RuntimeStorageSnapshot` version 1 and a read-only agentctl `GET /api/v1/runtime/storage` route.
Also expose the cached snapshot as optional `runtime_storage` on the existing status response.
Keep this advisory capability independent of durable delivery and the existing `delivery_health` field.
Construct one source sampler per shared agentctl process and pass it to the instance process managers.
Twenty instances must not construct twenty independent advisory caches.
Retain journal pressure, paused-producer, and cancellation fields for instances that have a journal.
An historical write failure and current capacity remain separate values.

The dedicated route uses the same instance transport authorization and a new backend `Client.RuntimeStorage` method.
It accepts no browser-selected path or source identity.
It reads the shared advisory cache without invoking `DeliveryHealth()` or its per-journal pressure refresh.
This keeps advisory requests from causing a separate fresh capacity read for every journal.
Existing safety checks and ordinary status/recovery semantics remain unchanged.
A missing route on an older agentctl means unavailable advisory capability, without a delivery downgrade.

The agentctl inside the task's executor is the authoritative capacity source, including for local tasks.
When a journal is configured, measure its filesystem. Otherwise measure the configured workspace filesystem.
Report `storage_kind` as `journal` or `workspace` so the UI identifies what the measurement covers.
An unreadable selected path stays unavailable. Do not fall back to an unrelated root filesystem or the backend host.
The current Sprites `agentctlInstanceRequest` does not configure a journal path.
Its advisory therefore uses workspace storage without claiming durable journal support.

Expose filesystem total bytes alongside available bytes and their sample time.
Use a `filesystem_capacity` object with `total_bytes`, `available_bytes`, `observed_at`, and a bounded source-scoped `filesystem_id`.
The opaque filesystem identity is private to source/backend grouping and is omitted from browser diagnostics and metric labels.
An unavailable identity prevents grouping, without discarding otherwise valid capacity measurements.
Use the raw `metrics.DiskUsage` capacity, before `Collector` rounds the display percentage.
The advisory starts at 90% usage and clears below 85% after a fresh successful sample.
Calculate usage from `(total - available) / total`, matching `diskCapacityFromBytes`.
Between 85% and 90%, retain the previous advisory state.
Unknown or failed samples do not clear an active warning or invent a percentage.

The advisory is separate from `DeliveryHealth.State` and actual write pressure.
A healthy journal state does not clear a 90% usage warning.
The advisory never requests cancellation, blocks a prompt, or creates a terminal cause.

Extend the backend client to decode the dedicated route and retain optional capacity and diagnostic status fields.
Add one lifecycle-owned observation coordinator through `Manager.Start` in `manager_lifecycle.go`.
Follow the manager's existing loop lifetime and shutdown conventions.

The coordinator refreshes registered agentctl observations through `Client.RuntimeStorage` every 30 seconds.
Each agentctl process owns a shared `DiskCapacitySampler` in the existing metrics package.
Register each instance's selected storage target with that sampler.
The sampler coalesces concurrent requests and caches capacity for 30 seconds per proven filesystem within that agentctl environment.
Reuse a fresh raw metrics sample only when it came from that same source and monitored filesystem.
Backend display metrics and their configured path do not define executor warning coverage.
When no suitable source sample exists, agentctl uses the existing bounded `DiskUsage` call.
This reads filesystem metadata without scanning directories.

Key sharing by the verified agentctl environment/runtime namespace plus filesystem identity.
Local sessions sharing one agentctl process and filesystem reuse one metadata read per interval.
Different Sprite VMs, containers, pods, SSH environments, or mounts retain separate source keys.
Identical filesystem identifiers can occur in independent virtual machines and do not permit merging.
A shared executor profile, hostname, path string, or label is also insufficient.
For Sprites, use its provider-verified VM/proxy ownership. Do not require an agentctl token that this executor intentionally omits.
When identity is unavailable, retain separate targets instead of guessing equivalence.
Release source cache entries after their final instance unregisters.
Share only capacity measurements, never submission, producer, cancellation, or owner-specific write requirements.

Use at most four concurrent advisory requests with two-second per-request deadlines, through each executor's current client or provider proxy.
Poll existing active execution environments only. Never launch, restore, or wake an inactive environment just for a warning refresh.
Use the existing remote connection or proxy. An unavailable or sleeping environment keeps an unavailable or stale observation.
Backend requests do not run filesystem checks themselves.
Per-instance advisory requests can still occur. They read the shared capacity cache and retain independently timestamped cached pressure details.
Do not add a separate ordinary `GetStatus` poll for every task just to produce the percentage warning.
Only lifecycle holds live observation state. Browser tabs do not start source polling.
Publish threshold and pressure changes immediately after observation, with a 30-second nonhealthy freshness refresh.
Do not write a session metadata record or transcript message on each sample.

The 30-second advisory cache must never supply journal write admission.
Keep the existing journal capacity cache, reserves, write checks, and cancellation rules unchanged.
Write-time errors continue through the immediate typed error path.

Expose the current snapshot through a `RuntimeStorageHealthProvider` at the task DTO boundary.
The existing cancellation snapshot provider is the pattern for runtime-only boot projection.
Add optional `runtime_storage_health` to task-session detail and boot payloads.
Publish `task_session.runtime_storage_changed` through the existing session-authorized gateway routing.
Extend the common event registry and gateway route explicitly. Do not broadcast runtime observations install-wide.

Each observation contains execution, incarnation, native generation, backend epoch, runtime epoch, and a monotonic revision.
Use the established current execution and runtime lease as the source of this identity.
The backend revision increases across execution replacements within one backend epoch.
The frontend accepts a newer session revision from the established backend epoch.
A replacement boot snapshot establishes a new backend epoch before later events can update it.
Missing fields from older responses do not become a healthy sample.
Freshness uses backend receipt age and monotonic elapsed time after browser receipt.
Source clocks supply display times, without deciding freshness.

## Ownership and failure behavior

Capture ownership before status I/O and revalidate it before cache replacement or publication.
Never hold execution locks over HTTP, persistence, or event-bus publication.
Stop, execution replacement, failed authentication, and manager shutdown release observation work.

A failed status request marks a prior observation unavailable or stale.
It does not establish process death, capacity recovery, or a terminal error.
An older peer without the new fields remains eligible for existing recovery behavior.

Live health is observational. It cannot settle a turn, change admission, resolve a block, or replay a prompt.
Existing pressure cancellation and recovery logic remain unchanged.
A healthy write-capacity sample clears matching pressure, while the usage advisory follows its separate 85% clearance threshold.
The existing failure stamp retains its controls in both cases.

The live observation cache and event path do not require SQL writes.
They can therefore show pressure while the database filesystem rejects writes.
After backend restart, resample current owners instead of treating an old observation as current.
Historical terminal diagnostics retain the ordinary persistence guarantee when storage permits their commit.
Do not fabricate an acknowledgment when storage cannot commit that record.

## Shared presentation

Add a shared normalized storage view model below components.
Read typed causes from `session-last-agent-error.ts` and current health from the session runtime projection.
Do not derive a reason from `message`, `details`, or raw HTTP text.

Use the existing recovery card for terminal failures and its existing technical disclosure.
Use one compact advisory row immediately above the visible chat composer, after the transcript.
`ComposerStatusNotices` is the existing status-region composition point; preserve `ChatInputBody` and its draft/focus owner.
Apply 8px top and bottom padding, with content-sized height and the existing subtle warning treatment.
Keep the icon, short usage label, and relevant action in one row.
Do not stack twenty warnings when twenty task sessions exist. Only visible affected chat composers render their own row.
Keep the executor label in the text when it is needed to identify the source.
Keep the usage value visible and truncate a long executor label when necessary. Its full label remains in Details.
On narrow phones, move available-byte text and expanded guidance into the existing explicit details disclosure.
Use a labelled 44px touch target for disclosure instead of a hover-only tooltip.
Opening details expands within the existing chat region and preserves its scroll ownership.
Combine matching current and terminal evidence into one visible region rather than two competing banners.

| State | Proposed headline | Body and controls |
| --- | --- | --- |
| Local usage at or above 90% | Disk <usage>% used | Compact row; available space when it fits; Storage action; details; work continues |
| Remote usage at or above 90% | <executor label>: disk <usage>% used | Compact row; executor details and remediation; work continues |
| Fresh local disk guard | Host disk space is low | Free space on this host; Storage settings; current measurements |
| Fresh remote disk guard | Executor disk space is low | Identify executor storage; remote remediation; no backend cleanup link |
| Terminal low-space failure | Session interrupted by low disk space | Failure time and measurements; Storage settings when applicable; eligible Resume |
| Usage falls below 85%, without write pressure | Remove the advisory | Keep normal running state and unrelated recovery controls |
| Fresh capacity recovery after failure | Keep the historical interruption headline | Explain that capacity is available; retain normal recovery controls |
| Old or unavailable sample | Keep its supported cause | Label numbers as last reported; do not claim current capacity |
| Missing typed evidence | Existing generic recovery fallback | Sanitized existing details; no invented disk cause or numeric values |

Quota, retention, and oversized-event reasons use their own localized explanations.
An absent sample never displays `0 B available`.
Format byte values and observation times with the existing locale helpers.

## Recovery actions

For local/worktree runtime storage, offer the registered Storage settings entry when the user can access it.
Reuse settings navigation and its existing Storage/Data route. Do not construct a new settings screen.
Opening settings preserves the session identity, recovery stamp, transcript, and composer draft.
The settings page already owns cleanup policy and its busy-state explanation.
The session card does not automatically infer that a Go cache caused a new incident.

For SSH, Docker, Kubernetes, and other remote runtime storage, explain the affected executor scope.
Do not offer backend cache cleanup as a remedy for a remote filesystem.
An unknown location receives general storage guidance.

Retain eligible Resume and Stop behavior from the shared recovery owner.
Explain an existing blocked or pending state without bypassing it in the view model.
Do not recommend or promote Start fresh for a physical capacity problem.
Retain unrelated recovery choices only when their original cause requires them.
Opening settings never toggles cleanup while busy or runs maintenance.

## Mobile composition

For the warning, reuse the existing chat footer's inline status treatment and the shared technical disclosure behavior.
The historical failure exemplar remains `SessionRecoveryCard` and `RecoveryActions`.
Keep one shared view model for both surfaces and viewports.
The advisory is a padded row above the composer on desktop and phone.
It does not require an additional modal, drawer, title block, explanatory paragraph, or stacked buttons.

Desktop keeps ordinary 28px controls inside the padded row.
Phone retains 44px touch targets with concise text and an explicit details control.
Only historical recovery cards keep stacked full-width phone actions.
The existing recovery region owns bounded vertical scrolling and expanded details.
Keep title and primary action reachable on short screens and with enlarged text.
Preserve focus and transcript position on a background update.
The existing successful user-initiated recovery focus rule remains unchanged.

The plan owns UI-01 through UI-03 previews.
Every new string uses `t()` or `<Trans>` with all seven real catalogs.
Generate the Traditional Chinese catalogs through the existing conversion command.

## Observability and compatibility

Retain existing journal error counters and use a closed reason classification for storage errors.
Log state transitions with bounded reason and location kind.
Keep execution identifiers in structured context, outside metric labels.
Do not emit one log or metadata write for every healthy poll.

| Peer and runtime | Behavior | Fallback |
| --- | --- | --- |
| Current local/worktree agentctl | Shared source samples; local storage action; journal diagnostics when configured | Missing capacity stays unknown |
| Current SSH/Docker/Kubernetes agentctl | Source-local measurements and executor guidance | Missing source data never uses backend disk values |
| Current Sprites agentctl | Per-Sprite workspace capacity through its provider-verified proxy | No durable journal claim without a configured journal |
| Other capable executors | Advisory when source status supplies version 1 | Missing capability stays unavailable |
| Older agentctl | Existing delivery and recovery remain usable | Generic failure, without text-based cause inference |
| Current legacy-delivery or passthrough instance | Advisory when `runtime_storage` is available | No new durability claim |
| Corrupt, locked, or unauthenticated source | Existing typed unavailable handling | No false healthy state or downgrade |

This uses existing recovery ownership and runtime-only projection patterns.
No separate ADR is needed for the additive evidence fields.

## Verification

Use fake capacity readers and source errors for physical guard tests.
Never fill the user's filesystem for a reproduction.
Exercise the actual HTTP decoder, terminal event mapping, metadata normalization, and live event projection.
Use deterministic ownership replacement tests with blocked status responses.

Browser coverage uses isolated seeded sessions and the existing error-recovery fixture pattern.
Seed typed failures through `ApiClient.seedTaskSession`, then assert visible copy and actions after reload.
Use status-event fixture frames for live pressure changes and stale-owner rejection.
Backend integration tests prove those frames come from authenticated status without a journal append.
Desktop and phone tests also verify settings navigation, safe details, and unchanged recovery identity.
Add deterministic threshold tests for 89.9%, 90%, 88% after a warning, 85%, and 84.9%.
Prove that shared local sessions reuse one filesystem sample and that advisory caching never affects write admission.
Twenty simultaneous local instances on one shared agentctl filesystem must produce one advisory metadata read per interval.
Twenty separate Sprites must produce twenty independent samples, one inside each VM per interval.
Also cover mixed local/remote tasks and identical filesystem IDs from different VM namespaces.
Use provider/client fixtures for this matrix. No real cloud account is required for contract tests.
Exercise the actual advisory route to prove twenty concurrent shared-instance requests perform one capacity read.
Verify that it never invokes per-journal pressure refresh, and that legacy-delivery Sprites still report workspace capacity.
Verify that inactive remote environments cause no restore, launch, wake, or new proxy creation for capacity polling.
