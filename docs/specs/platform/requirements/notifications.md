---
status: active
system: platform
created: 2026-07-24
owners:
  - kandev
---
# Semantic Notifications Requirements

## Overview

Kandev currently treats every transition to a session's generic `WAITING_FOR_INPUT` state as though the agent asked the user a question. That state is also used before the first turn and after an ordinary completed turn, so notifications can claim that input is required when it is not. The same event is deduplicated once per session, which can let an early false positive suppress a later real clarification request.

## Requirements

### REQ-PLATFORM-NOTIFICATIONS-001: Semantic Notifications

**Intent:** Kandev currently treats every transition to a session's generic `WAITING_FOR_INPUT` state as though the agent asked the user a question. That state is also used before the first turn and after an ordinary completed turn, so notifications can claim that input is required when it is not. The same event is deduplicated once per session, which can let an early false positive suppress a later real clarification request.

#### Acceptance criteria

- **AC-PLATFORM-NOTIFICATIONS-001.1:** Notification settings expose two independent session events:
- **AC-PLATFORM-NOTIFICATIONS-001.2:** **Agent turn finished** — notify after each completed agent turn.
- **AC-PLATFORM-NOTIFICATIONS-001.3:** **Agent needs an answer** — notify when the agent explicitly requests a user response through Kandev's clarification flow.
- **AC-PLATFORM-NOTIFICATIONS-001.4:** A user may enable either event, both events, or neither event for each notification provider.
- **AC-PLATFORM-NOTIFICATIONS-001.5:** **Kandev update available** is an independently selectable provider event. It is emitted when the release poller durably observes a version newer than the running Kandev version.
- **AC-PLATFORM-NOTIFICATIONS-001.6:** When a provider-create request omits its event selection, the clarification event is enabled by default and turn-finished remains opt-in. An explicitly empty selection enables neither event.
- **AC-PLATFORM-NOTIFICATIONS-001.7:** Existing `session.waiting_for_input` subscriptions migrate to **Agent needs an answer** only. Migration never opts a provider into **Agent turn finished**.
- **AC-PLATFORM-NOTIFICATIONS-001.8:** Fresh default Local and System providers subscribe to update availability. On upgrade, existing Local and System providers receive that subscription once; a migration marker prevents later startup from re-enabling a choice the user removed. Existing Apprise providers are never opted in automatically.

## Migrated source detail

## Why

Kandev currently treats every transition to a session's generic
`WAITING_FOR_INPUT` state as though the agent asked the user a question. That
state is also used before the first turn and after an ordinary completed turn,
so notifications can claim that input is required when it is not. The same
event is deduplicated once per session, which can let an early false positive
suppress a later real clarification request.

The notification settings page also snapshots an empty provider store during a
cold load. Providers fetched immediately afterward remain absent from the
rendered draft, making an intact configuration look lost.

Update detection has the same need for durable, user-controlled delivery.
Users should learn that a newer Kandev release exists without maintaining a
second preference model that bypasses Local, System, Apprise, or future
notification providers. A denied or unavailable desktop notification must not
hide the update from an open Kandev window.

## What

- Notification settings expose two independent session events:
  - **Agent turn finished** — notify after each completed agent turn.
  - **Agent needs an answer** — notify when the agent explicitly requests a
    user response through Kandev's clarification flow.
- A user may enable either event, both events, or neither event for each
  notification provider.
- **Kandev update available** is an independently selectable provider event.
  It is emitted when the release poller durably observes a version newer than
  the running Kandev version.
- When a provider-create request omits its event selection, the clarification
  event is enabled by default and turn-finished remains opt-in. An explicitly
  empty selection enables neither event.
- Existing `session.waiting_for_input` subscriptions migrate to **Agent needs an
  answer** only. Migration never opts a provider into **Agent turn finished**.
- Fresh default Local and System providers subscribe to update availability.
  On upgrade, existing Local and System providers receive that subscription
  once; a migration marker prevents later startup from re-enabling a choice the
  user removed. Existing Apprise providers are never opted in automatically.
- Session startup, readiness, idle state, and generic
  `WAITING_FOR_INPUT` transitions do not themselves send either notification.
- A turn-finished notification uses:
  - title: `Agent turn finished`
  - body: `The agent finished a turn on "<task title>".`
- A clarification notification uses:
  - title: `Agent needs your answer`
  - body: `The agent asked a question on "<task title>".`
- The local browser and native desktop notification path uses the same semantic
  event and copy as remote providers such as Apprise.
- An update-available notification uses:
  - title: `Kandev update available`
  - body: `Kandev <version> is available. Open Settings > System > Updates to review it.`
- The Local provider keeps an in-app update indication visible even when
  browser/native permission is denied, unsupported, or delivery fails. Native
  permission is requested only from the existing user-initiated control on the
  Notifications settings page.
- Native session-failure alerts remain an independent safety channel whenever
  the Local provider is enabled; they are not controlled by either semantic
  event checkbox.
- On a cold settings-page load, the provider list and saved event selections
  appear after the provider request completes. Initial hydration must not
  overwrite edits the user has already made.

## Data Model

The durable provider subscription keys are:

- `session.turn_finished`
- `session.clarification_requested`
- `system.update_available`

Each delivery records a semantic occurrence ID and, for session-scoped events,
the task-session ID:

- turn-finished occurrence ID: the completed turn ID;
- clarification occurrence ID: the clarification request's pending/request ID.
- update-available occurrence ID: the normalized release version/tag.

Delivery idempotency is scoped to provider, event type, and occurrence ID.
Consequently, replaying one occurrence does not duplicate a delivery, while
separate turns or clarification requests in the same session may each notify.

`session.waiting_for_input` is a legacy subscription key accepted only by the
data migration. It is not offered by the settings API after migration and is
not emitted as a new notification event.

## API Surface

- The notification-provider API returns the two semantic events in its
  available-event catalog together with `system.update_available`.
- Provider create and update requests persist the selected semantic event keys.
- An omitted create-time event field applies the clarification-only default;
  an explicitly empty event array persists no subscriptions.
- The WebSocket gateway may emit `session.turn_finished` and
  `session.clarification_requested` for the local notification provider, plus
  `system.update_available` with `{ version, url, title, body,
  occurrence_id }`.
- Local semantic notifications carry the occurrence ID through the WebSocket
  envelope/payload so native de-duplication uses the same identity as backend
  delivery de-duplication.
- Existing provider URLs, enabled state, and unrelated event subscriptions are
  unchanged.
- Update-notification policy is not exposed through a separate settings route
  or channel selector. Provider enabled state and event subscriptions are the
  authoritative configuration.

## State and Event Semantics

| Domain occurrence | Turn finished | Needs an answer |
|---|---:|---:|
| Session starts or becomes ready | No | No |
| Session enters generic `WAITING_FOR_INPUT` | No | No |
| An agent turn is durably completed | Yes, when selected | No |
| An agent-authored structured clarification request is created | No | Yes, when selected |
| The user answers a clarification | No | No |
| The same domain event is replayed | No duplicate | No duplicate |
| A later occurrence happens in the same session | Yes | Yes |

A clarification bundle containing several questions is one occurrence and
produces at most one notification per provider. User-authored, system-authored,
malformed, or unscoped clarification-shaped messages do not notify.

An update is one install-wide occurrence per release version. Replaying cached
release state after a Local client subscribes retries only providers whose
delivery was not already recorded; Apprise, System, and other successful
providers remain deduplicated.

## Permissions

Notification configuration continues to use the existing settings
authorization rules. This change grants no additional access to task content.
Notification bodies contain the task title but not the question text.

## Failure Modes

- A provider send failure releases that occurrence's de-duplication claim so a
  replay may retry it. It does not mark a later occurrence as delivered.
- If the startup release poll completes before a Local WebSocket subscriber
  exists, Local delivery remains pending. The cached release is replayed when
  an eligible user client subscribes, without refetching GitHub.
- If native/browser delivery is denied, unsupported, or fails, the open client
  retains the in-app update indication. Background delivery never opens a
  permission prompt.
- Replayed event-bus messages do not create duplicate deliveries for the same
  provider and occurrence.
- A provider refresh received while persisted-provider drafts are clean may
  hydrate their baseline. It must not replace unsaved edits to an existing
  provider. A pre-load new-provider form remains intact while the authoritative
  provider list hydrates around it.

## Persistence Guarantees

- Existing provider records and credentials survive the schema migration.
- An enabled legacy `session.waiting_for_input` subscription becomes an enabled
  `session.clarification_requested` subscription. A disabled legacy
  subscription remains disabled unless an already-existing semantic
  subscription is enabled.
- Migration is idempotent and does not create duplicate subscriptions.
- The one-time update-event migration adds `system.update_available` only to
  existing Local and System providers. It records completion separately so a
  later user opt-out survives restart.
- Existing delivery history remains available for audit, but legacy
  session-level deduplication does not suppress new semantic occurrences.
- The de-duplication claim is inserted atomically with its semantic occurrence
  identity. A failed provider send removes that exact
  provider/event/occurrence claim.
- Update delivery history uses the same provider/event/occurrence records as
  every other semantic notification. No independent “last notified update”
  key is authoritative.

## Scenarios

- **GIVEN** a provider subscribed to clarification requests, **WHEN** a session
  starts and waits for its first user turn, **THEN** no notification is sent.
- **GIVEN** a provider subscribed only to turn completion, **WHEN** an ordinary
  agent turn completes, **THEN** it receives `Agent turn finished` and no copy
  claims that user input is required.
- **GIVEN** a provider subscribed only to clarification requests, **WHEN** an
  agent issues a structured question bundle, **THEN** it receives one
  `Agent needs your answer` notification.
- **GIVEN** two completed turns in one session, **WHEN** turn completion is
  selected, **THEN** each turn can produce one notification.
- **GIVEN** a clarification event is delivered and then replayed, **WHEN** both
  copies reach the notification service, **THEN** only one delivery is sent.
- **GIVEN** Local is disabled for `system.update_available`, **WHEN** a newer
  release is detected, **THEN** no Local update toast or desktop notification
  is delivered.
- **GIVEN** Apprise is subscribed to `system.update_available`, **WHEN** a newer
  release is detected, **THEN** Apprise receives the same update occurrence and
  copy as Local and System providers.
- **GIVEN** an existing Local provider predates the update event, **WHEN** the
  notification schema upgrade runs twice and the user removes the migrated
  subscription between runs, **THEN** the second run does not re-enable it.
- **GIVEN** the startup release poll completes before any Local client
  subscribes, **WHEN** the first user client subscribes, **THEN** the cached
  release is delivered once without waiting for the next six-hour poll.
- **GIVEN** native/browser notification permission is denied or delivery
  fails, **WHEN** an open Local client receives an update occurrence, **THEN**
  the in-app update indication remains visible and no background permission
  prompt is shown.
- **GIVEN** an existing provider subscribed to
  `session.waiting_for_input`, **WHEN** the database is upgraded, **THEN** the
  provider is subscribed to clarification requests and not to turn completion.
- **GIVEN** saved providers exist, **WHEN** the notifications settings route is
  opened from a cold frontend store, **THEN** the providers and their selections
  appear after loading without requiring a remount.
- **GIVEN** a 390px mobile viewport, **WHEN** the notification event rows are displayed,
  **THEN** the stacked composition includes the update event and keeps every
  label, description, provider, and control readable and touch operable without
  horizontal scrolling; a changed update subscription survives Save and
  reload.

## Out of Scope

- Inferring that arbitrary prose ending in a question mark requires an answer.
- Including clarification text or other conversation content in notification
  payloads.
- Per-task notification policy, schedules, batching, or quiet hours.
- Retrying provider deliveries beyond the existing notification-provider
  behavior.
- Adding a new notification-settings fetch-error surface.
- A separate update-only enable switch or desktop/in-app/both channel policy.
- Changing session lifecycle states or the chat waiting-state presentation.

## Implementation Plan

- [Semantic notification implementation](../../../plans/semantic-notifications/plan.md)
- [Update notification reliability remediation](../../../plans/update-notification-reliability/plan.md)
