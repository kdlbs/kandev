---
status: current
system: agents
requirements:
  - REQ-AGENTS-RUNTIME-NOTIFY-003
created: 2026-10-04
owners:
  - Kandev
---

# Agent runtime update summary design

## Boundary and mapping

This design extends the [runtime notification design](runtime-update-notifications.md).
Agents owns availability grouping. Platform retains recipient resolution, provider subscriptions, and delivery claims.

| Criteria | Design section |
| --- | --- |
| 003.1, 003.2, 003.6, 003.7 | Controller collection |
| 003.3, 003.4, 003.8 | Payload and presentation |
| 003.5, 003.9 | Delivery and recovery |

All criterion numbers refer to AC-AGENTS-RUNTIME-NOTIFY-003.

## Controller collection

Extend `StartRuntimeUpdateBackground` with one lifecycle-owned availability collector.
Start it before the initial sweep and reconnect listener can publish notices.
Use the existing controller cancellation and wait group; shutdown and restore quiesce stop and drain both workers.

`publishRuntimeStatus` submits availability state to the collector and sends terminal outcomes directly to the existing notifier.
Every observed status also removes obsolete pending availability for that agent/runtime.
Pending state contains at most one notice per registered agent/runtime, replacing an older target with the latest observed target.
Custom or unsupported sources cannot create additional identities outside registered runtime metadata.

The first eligible arrival starts a fixed 30-second timer. Repeated arrivals do not reset it.
Startup `RunRuntimeUpdatePass`, periodic passes, and `ReplayRuntimeUpdateNotices` share this collector.
They do not create timers per subscriber or agent. Status reads remain read-only and do not enqueue automatic jobs.

At expiry, use the existing shared `ListAgentUpdateStatuses` path to revalidate membership.
Its cache, bounded source lookups, and cancellation remain authoritative.
Keep only matching current agent/runtime/target identities that remain enabled, available, and `update_available`.
An unknown or failed source drops that member without a failure toast.
Serialize collection and flush; arrivals during revalidation belong to the next window and cannot extend the expired window.
Revalidation can add the existing bounded lookup time after the 30-second deadline.

Add a batch method to `RuntimeUpdateNotifier` for available notices.
Keep `HandleAgentRuntimeUpdate` for outcomes. Update notifier test doubles and prohibit the individual available path from bypassing collection.
Tests inject a clock/timer boundary; no shipped timing override or feature flag is needed.

## Delivery and recovery

The notification service resolves recipients and providers through its existing methods.
For each enabled provider subscribed to `system.update_available`, claim each member's existing occurrence ID with `InsertDelivery`.
These IDs retain agent/runtime/target identity. Do not store a summary hash as the durable deduplication identity.

Build the message only from newly inserted member claims for that recipient/provider.
Skip existing claims. If a claim fails, omit that member and leave it eligible for later replay.
Send nothing when no claims succeed. Send a singular notice when one claim succeeds; otherwise send one summary.
Sort members by trusted identity and derive the presentation occurrence ID from the sorted child occurrence IDs.
Controller serialization prevents two availability batches from splitting the same window.
Database uniqueness remains the guard against competing replays.

If provider delivery fails, delete only the claims created for that attempted message.
`ErrNoEligibleSubscriber` uses the same rollback path. Log rollback errors through existing notification logging.
Successful providers retain their claims even when another provider fails.
Later reconnect replay or a scheduled pass retries unclaimed members; there is no new rapid retry loop.

Pending collector state is transient. A restart reconstructs notices from status and retained outcomes.
No table or uniqueness change is required. Claims use the same SQL paths for SQLite and PostgreSQL.
The existing crash interval between claim insertion and provider send remains; this feature does not add an outbox.
The summary is per delivery channel, not an exactly-once guarantee across browser, native, and system providers.

## Payload and presentation

Keep `system.update_available` and its existing subscription preferences.
Add an optional typed `runtime_updates` list to the notification message and local WebSocket payload.
Each member carries `occurrence_id`, `agent_name`, `runtime_id`, `display_name`, `previous_version`, and `version`.
Add `notification_kind: agent_runtime_summary` and `url: /settings/agents#runtime-updates` to grouped messages.
Keep the provider `Payload` string map for existing scalar messages; do not encode the member list into display copy.
Use a small notification-model item type to avoid provider dependencies on controller DTOs.

External and system providers receive concrete summary title/body through `Message`.
The local provider preserves the structured list for locale-aware rendering.
Update `UpdateAvailablePayload`, `UpdateAvailableNotification`, the WS handler, the UI queue, and `useUpdateAvailableToast` together.
Grouped messages do not require a fictitious single version. Make the top-level version optional and retain validated per-member versions.
Recognize the summary kind before the legacy Kandev-release fallback.
Legacy singular payloads and terminal outcomes remain supported.

For two or more members, render `4 agent runtime updates available`, a short Settings explanation, and one Review updates action.
Use `count` with locale plural keys. Show full version details in Settings rather than expanding the toast into runtime cards.
For one member, use existing singular copy and its runtime-specific action.
The browser/native notification path receives the same summary as the app toast and retains its current transport selection.
Refresh shared runtime status for both summary and singular messages in `UpdateAvailableToastBridge`.

Reuse `ToastProvider` for a compact notice, including its polite live region and responsive action sizing.
Use the existing Settings target registry for `runtime-updates` to expand the section.
Phone users navigate directly to Settings, rather than opening a new drawer or a stacked detail overlay.
`AgentRuntimePolicies`, `SettingsPageTemplate`, and the existing runtime notice action are the nearest mobile exemplars.
Settings owns detail scrolling. Keep safe-area offsets, wrapping text, 44px phone targets, and desktop 28px actions.
No floating indicator, new preference, or viewport-specific grouping state is introduced.

## Alternatives and limits

Frontend-only grouping leaves backend system and external notifications separate.
Snapshot-only batching can split overlapping startup discovery and reconnect replay.
A fixed collector window covers both paths and avoids unbounded debounce delays.
Generic notification rules add unrelated event policies; this collector accepts runtime availability only.

The [public runtime-notification guide](../../../public/agents-and-profiles.md) documents the 30-second startup/reconnect summary and its Settings action.
Use existing structured notification logs for failures; no new metric labels or logs containing member lists are required.
