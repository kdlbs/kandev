---
status: draft
system: platform
created: 2026-08-05
owners:
  - carlosflorencio
---
# Session subscription recovery Requirements

## Overview

A fast task session can change state or persist a reply while the browser is waiting for the backend to register its `session.subscribe` request. The live notification is then unavailable to that client, leaving the task looking busy or the transcript missing a reply until the user reloads the page.

## Requirements

### REQ-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001: Session subscription recovery

**Intent:** A fast task session can change state or persist a reply while the browser is waiting for the backend to register its `session.subscribe` request. The live notification is then unavailable to that client, leaving the task looking busy or the transcript missing a reply until the user reloads the page.

#### Acceptance criteria

- **AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001.1:** A session subscription exposes a readiness point that resolves only after the backend has acknowledged registration.
- **AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001.2:** Transcript reconciliation for a newly mounted or reconnected session starts after that readiness point, so the persisted message view covers the interval before registration and live notifications cover the interval after it.
- **AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001.3:** Ref-counted consumers share an in-flight registration readiness point and do not unregister one another's subscription.
- **AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001.4:** Reconnect and delayed-session retry paths use the same acknowledgement-aware registration behavior.
- **AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001.5:** A failed or disconnected registration does not create an unhandled client rejection or leave a stale readiness promise that prevents the next connection from hydrating the session.
- **AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001.6:** **GIVEN** a browser sends `session.subscribe` while a session state change or reply is persisted before the backend registers the subscription, **WHEN** the backend acknowledges the subscription and the client reconciles messages, **THEN** the client displays the authoritative state and persisted reply without a page reload.
- **AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001.7:** **GIVEN** one client-side consumer is already registering a session and a second consumer mounts, **WHEN** the second consumer requests hydration, **THEN** it waits for the shared registration acknowledgement and does not race an earlier `message.list` request ahead of registration.
- **AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001.8:** **GIVEN** a session subscription is restored after a WebSocket reconnect, **WHEN** the restored registration is acknowledged, **THEN** message reconciliation can run against the restored subscription and later events are delivered normally.

## Migrated source detail

## Why

A fast task session can change state or persist a reply while the browser is
waiting for the backend to register its `session.subscribe` request. The live
notification is then unavailable to that client, leaving the task looking busy
or the transcript missing a reply until the user reloads the page.

## What

- A session subscription exposes a readiness point that resolves only after the
  backend has acknowledged registration.
- Transcript reconciliation for a newly mounted or reconnected session starts
  after that readiness point, so the persisted message view covers the interval
  before registration and live notifications cover the interval after it.
- Ref-counted consumers share an in-flight registration readiness point and do
  not unregister one another's subscription.
- Reconnect and delayed-session retry paths use the same acknowledgement-aware
  registration behavior.
- A failed or disconnected registration does not create an unhandled client
  rejection or leave a stale readiness promise that prevents the next
  connection from hydrating the session.

## Scenarios

- **GIVEN** a browser sends `session.subscribe` while a session state change or
  reply is persisted before the backend registers the subscription, **WHEN** the
  backend acknowledges the subscription and the client reconciles messages,
  **THEN** the client displays the authoritative state and persisted reply
  without a page reload.
- **GIVEN** one client-side consumer is already registering a session and a
  second consumer mounts, **WHEN** the second consumer requests hydration,
  **THEN** it waits for the shared registration acknowledgement and does not
  race an earlier `message.list` request ahead of registration.
- **GIVEN** a session subscription is restored after a WebSocket reconnect,
  **WHEN** the restored registration is acknowledged, **THEN** message
  reconciliation can run against the restored subscription and later events are
  delivered normally.
- **GIVEN** the registration fails or the socket disconnects before the
  acknowledgement, **WHEN** the client retries on the next connected socket,
  **THEN** the retry can establish a fresh readiness point and no stale promise
  or unhandled rejection blocks hydration.

## Out of scope

- Adding durable event sequence numbers, server-side replay cursors, or a
  general WebSocket backlog.
- Changing the persisted session/message schema or event publication order.
- Removing the existing state snapshot, backfill, or foreground recovery paths;
  they remain defense-in-depth for later disconnects and long-running turns.
- Retaining reload-based E2E workarounds after the repair is proven; their
  cleanup is an implementation and validation task, not a new recovery
  contract.
