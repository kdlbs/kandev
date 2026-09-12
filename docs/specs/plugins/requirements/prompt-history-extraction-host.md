---
status: active
system: plugins
created: 2026-09-06
owners:
  - kandev
---
# Prompt History Plugin Host Prerequisites Requirements

## Overview

Prompt History can move to an external plugin only when the Host exposes the
narrow browser contracts that the existing core panel currently obtains from
private stores, task services, WebSocket reconciliation, task-panel context,
native navigation, custom-prompt rendering, and browser-local favorites. This
package inventories those contact points and defines the additive boundary. It
does not extract or remove the shipped core panel.

## Requirements

### REQ-PLUGINS-PROMPT-HISTORY-HOST-001: Prompt History Contact-Point Inventory

**Intent:** Identify every core-owned dependency that a future external prompt-history plugin must consume through a public Host contract or that must remain Host-owned.

#### Acceptance criteria

- **AC-PLUGINS-PROMPT-HISTORY-HOST-001.1:** The inventory covers task/session identity, task-panel registration and placement, desktop and native-mobile navigation, message and turn reads, live WebSocket events, session removal, transcript cache boundaries, task generation and lifecycle invalidation, custom-prompt aliases, favorites, and localization.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-001.2:** Each contact point is classified as a public plugin contract, a Host-owned prerequisite, a core-only implementation detail, or an extraction blocker. No future plugin contract requires importing `apps/web`, reading Zustand state, or calling an unscoped `/api/v1` route.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-001.3:** The inventory distinguishes the prompt-history plugin's responsibilities (derivation, filtering, and presentation) from Host responsibilities (authorization, persistence, sanitization, transport ordering, lifecycle, navigation, and private state).
- **AC-PLUGINS-PROMPT-HISTORY-HOST-001.4:** Desktop and native-mobile entry points remain part of one task-panel contract. Mobile uses native panel navigation and touch behavior rather than a compressed desktop layout.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-001.5:** The inventory records that core prompt history remains registered and behaviorally unchanged until a separate extraction package proves parity, migrates saved layout IDs, and removes core ownership.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-001.6:** The inventory identifies generic task-panel title, visibility, saved-layout, lifecycle, and mobile-picker contact points as shared infrastructure to extend without prompt-history-specific branches.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-001.7:** The inventory identifies the browser-facade requirements, system design, SDK/API reference, ADR, and implementation plan as the durable artifacts that must agree before extraction work begins.

### REQ-PLUGINS-PROMPT-HISTORY-HOST-002: Browser Conversation Host Boundary

**Intent:** Provide a sanitized, capability-gated, generation-safe browser conversation facade with deterministic pagination and ordered live reconciliation.

#### Acceptance criteria

- **AC-PLUGINS-PROMPT-HISTORY-HOST-002.1:** The public message shape contains only plugin-safe fields: message, session, turn, and nullable task IDs; author and message type; stripped content; created and updated timestamps; durable prompt index; and an optional sender task ID derived from approved metadata. It never exposes raw content, injected system blocks, arbitrary metadata, credentials, or private store objects. The query has tri-state `taskId`: `undefined` inherits the panel task, `null` selects the complete session, and a string must match the panel task.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-002.2:** `host.conversation.useSessionMessages` exposes loading, hydrated, pagination, retryable error, and observable terminal `removed` state. `loadMore()` resolves the count of newly projected messages; concurrent calls for one continuation join; exhausted, closed, or terminal handles resolve `0` without network activity; failures reject with `PluginConversationError` and preserve committed state.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-002.3:** `host.conversation.useSessionTurns` returns typed turn IDs, start/completion timestamps, and freshness state. It applies the same task-selection rules and terminal lifecycle as messages.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-002.4:** Message pages preserve deterministic `(created_at,id)` ordering, use opaque keyset cursors, expose absolute prompt indexes that do not renumber after deletion, and can load the selected session without initializing or changing the chat transcript cache.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-002.5:** The Host establishes subscription readiness before resolving the initial snapshot, buffers and merges later adds and updates by stable ID, removes deleted messages, and reconciles after reconnect. No message created between subscription and snapshot is lost or duplicated.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-002.6:** Requests, pages, and events are scoped to the requested session generation and every mounted panel owns an independent Host scope, cache, consumer identity, and abort controller. Session switch, unmount, plugin reload, disable, or uninstall aborts pending reads, releases the consumer, closes the handle, and prevents stale completion from mutating a replacement surface.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-002.7:** Authenticated browser read routes require an active plugin declaring `api_read:messages`, verify access to the task session, and reuse task services and repositories. Unknown or inactive plugins, missing capabilities, invalid cursors, and inaccessible sessions fail closed with typed HTTP errors.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-002.8:** External plugins do not need a Go backend to consume the browser facade. The existing capability-gated Go Host `Messages().List` API remains available for backend plugins but is not the browser contract and is not expanded solely for prompt history.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-002.9:** The turns facade consumes typed `session.turn.started` and `session.turn.completed` events after readiness, merges completion by turn ID and freshness, and updates derived prompt durations without retry or remount. Session removal is a terminal barrier for queued turn events.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-002.10:** Browser failures use `{"error":{"code":"...","message":"...","retryable":false|true}}`: 401 maps to `unauthenticated`, 404 to `not_found`, 400 to `invalid_query`, and authorized upstream 5xx to retryable `upstream_failure`. The first three are non-retryable. Binding-only `409/generation_superseded` is Host-loader-only; continuation-renewal 409 is retryable `upstream_failure`; binding and renewal responses use `Cache-Control: no-store`.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-002.11:** Test-only fixture controls can causally request message update/delete and turn-completion transitions. Each control response is tied to its emitted event, so parity tests do not use timing sleeps.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-002.12:** Every snapshot and public live message/turn DTO has a non-empty `updatedAt`. A timestamp-less unseen add may normalize to its valid `createdAt`; a timestamp-less update to an existing ID is rejected and repaired by the next snapshot. Fixture transitions provide explicit update/completion timestamps.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-002.13:** Subscription readiness yields a per-session monotonic watermark. Plugin subscribe requests carry an authenticated loader binding for plugin, consumer kind, generation, and a Host-minted per-panel `consumer_id`; core wires carry a distinct Host-minted `wire_id`. Each identity survives reconnect, is never shared, and is released on unsubscribe or after retention. Binding tokens expire no later than 10 minutes after issuance, and the loader rebinds before expiry. Strict results include event watermark, committed snapshot cutoff, and Host-only snapshot token.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-002.14:** Before buffering or merging, the Host applies the selected task/session matching rule. Inherited `undefined` and explicit task strings require exact session and task equality. Explicit `null` accepts every task ID in the selected session, including `event.task_id:null`. `session.removed` is accepted for the selected session regardless of event task ID and closes both hooks. Wrong-session and wrong-task events do not mutate selected state.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-002.15:** The facade consumes the production ordered WebSocket envelope and readiness handshake. Raw envelopes are validated against a versioned event registry before projection; outer and payload event types must match. Missing, malformed, unsupported-version, and unknown-type events are durable poison: advancement stops, diagnostics retain event ID and version, and atomic rebind returns a replacement cursor/token past the poison. `session.removed` is terminal whether it precedes or follows poison; its replacement cursor is closed. ACK accepts only the highest contiguous sequence and rejects forward gaps, malformed identity, and invalid tokens without advancing state. The protocol has explicit success and failure ACK envelopes, with no success-only fields on failure.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-002.16:** Event append, version rows, deletion tombstones, and sequence allocation share one transaction and lock. Delivery cursors persist ACK, owner epoch, lease, retry, and generation state. Poison dispatch is owned by `SessionDeliveryDispatcher`: records use `pending`, `leased`, `exhausted`, and `requeued` states, a 30-second lease, five attempts, exponential backoff capped at 30 seconds, immutable diagnostics, and audited idempotent operator requeue. Startup reclaims expired leases and never silently skips poison. Tokens and reconnect grace are each bounded at 10 minutes; retention preserves resumable cursors, event rows, and terminal tombstones through both bounds.

A previously authorized consumer that reconnects to a removed session enters
the same terminal state as a live removal event: committed rows remain visible,
`removed` is true, `hasMore` is false, and later reads and retries make no
network request. A fresh unauthorized or nonexistent lookup remains an ordinary
not-found response and does not reveal prior existence.

### REQ-PLUGINS-PROMPT-HISTORY-HOST-003: Task-Panel and Navigation Capabilities

**Intent:** Make the future plugin appear in the existing task workbench without taking ownership of native navigation or mobile composition.

#### Acceptance criteria

- **AC-PLUGINS-PROMPT-HISTORY-HOST-003.1:** Task-panel registration supports an optional localized title, visibility predicate, session kind, and generation-bound message-navigation capability while preserving existing registrations.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-003.2:** The navigation capability accepts a public message ID and returns a typed outcome. It validates session and generation, opens the native chat surface, selects the message, and scrolls through the existing local adapter without exposing transcript stores or scroll refs.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-003.3:** Desktop uses the existing add-panel menu and dockview panel. Native mobile uses grouped Panels bottom navigation, the existing inset picker, a full-height panel, one vertical scroll area, and a distinct 44 px expansion control.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-003.4:** Panel and navigation capabilities fail closed after unmount, task/session switch, plugin reload, disable, uninstall, or generation change. The capability never mutates the chat transcript cache.

### REQ-PLUGINS-PROMPT-HISTORY-HOST-004: Host-Owned Display Dependencies

**Intent:** Keep private settings, browser stores, and locale behavior behind small Host-owned capabilities.

#### Acceptance criteria

- **AC-PLUGINS-PROMPT-HISTORY-HOST-004.1:** `host.ui.PromptMentionText` renders a curated custom-prompt alias or title through Host-owned settings and localization, without exposing settings or arbitrary markdown rendering.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-004.2:** `host.conversation.useMessageFavorite` observes the existing session-storage favorite store and exposes read-only favorite state with a stable subscribe lifecycle. The plugin cannot access or mutate the store directly.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-004.3:** All new public copy uses the reactive locale API and the five-language catalog contract. Pseudo-locale and mobile previews remain valid.

### REQ-PLUGINS-PROMPT-HISTORY-HOST-005: Compatibility and Extraction Boundary

**Intent:** Make the prerequisite package independently adoptable while preventing accidental extraction of the core implementation.

#### Acceptance criteria

- **AC-PLUGINS-PROMPT-HISTORY-HOST-005.1:** The SDK contract is runtime-free and assignable to the web internal aliases. Existing plugin registrations remain source-compatible; no public contract imports Kandev stores, React runtime, or backend types.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-005.2:** A fixture plugin consumes only the public SDK and proves message pagination, live add/update/delete, turn completion, removal, favorites, alias display, task navigation, and desktop/mobile panel placement.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-005.3:** Parity verification compares the fixture plugin with the shipped core panel for ordering, filtering inputs, navigation, alias display, favorites, live updates, removal, and mobile behavior. It includes wrong-session, wrong-task, reconnect, gap, poison, and terminal-removal cases.
- **AC-PLUGINS-PROMPT-HISTORY-HOST-005.4:** The package documents the future extraction sequence. It leaves core registrations, layout IDs, stores, locale keys, and core tests untouched; external plugin creation, publication, saved-layout migration, and core removal require a later explicitly approved package.

## Source inventory

The implementation plan and system design record the current contact-point
inventory. The public browser contract is canonical in
[`docs/plans/plugins/PLUGIN-API.md`](../../../plans/plugins/PLUGIN-API.md). The
existing UI prompt-history requirements remain authoritative for shipped
product behavior; this document is authoritative only for Host prerequisites.

## Decision and implementation boundary

This is a draft prerequisite package, not approval to implement or extract the
plugin. The plan assigns each criterion to one work order and stops after a
fixture parity proof. Any implementation must preserve the Host-owned security,
lifecycle, and transport invariants above.

See [the system design](../system-design/prompt-history-extraction-host.md) and
[the implementation plan](../../../plans/prompt-history-plugin-host/plan.md).
