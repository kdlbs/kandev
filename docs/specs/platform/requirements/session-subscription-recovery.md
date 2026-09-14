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

### REQ-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002: Recover session entry from temporary delays

**Intent:** Users can open an existing conversation during a temporary connection delay without a false startup failure or lost history.

#### Acceptance criteria

- **AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.1:** A successful subscription or status response delayed by seven seconds shall complete session entry without a startup-failure alert.
- **AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.2:** After a temporary entry timeout, the visible session shall retry automatically without a reload, navigation, or visibility change.
- **AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.3:** On a continuously connected socket, unsuccessful automatic recovery shall stop within 45 seconds. The user shall then have an explicit Retry action.
- **AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.4:** Until history loads successfully, an uncached conversation shall show loading, retrying, or unavailable feedback. It shall never show the empty-conversation invitation.
- **AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.5:** A failed refresh shall preserve cached messages, drafts, and scroll position. Successful recovery shall clear its stale feedback.
- **AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.6:** A status timeout shall identify a status-check problem, not a failed session start. Technical details shall remain collapsed by default.
- **AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.7:** Recovery shall not duplicate agent launches or bypass archive, permission, or manual-start restrictions. Changing sessions shall cancel obsolete recovery effects.
- **AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.8:** Desktop, phone, and task preview shall expose equivalent recovery outcomes. Feedback shall preserve chat access and keyboard focus without horizontal overflow.
- **AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.9:** Phone and coarse-pointer recovery controls shall have touch targets of at least 44 pixels. Progress shall have a localized status announcement.
- **AC-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002.10:** Permission and missing-session failures shall stop automatic retries. A successful empty history response shall still show the normal empty-conversation invitation.

## Implementation plans

- [Acknowledgement ordering](../../../plans/session-subscription-recovery/plan.md)
- [Delayed session entry](../../../plans/session-entry-recovery/plan.md)

## Out of scope

- Adding durable event sequence numbers, server-side replay cursors, or a
  general WebSocket backlog.
- Changing the persisted session/message schema or event publication order.
- Removing the existing state snapshot, backfill, or foreground recovery paths;
  they remain defense-in-depth for later disconnects and long-running turns.
- Retaining reload-based E2E workarounds after the repair is proven; their
  cleanup is an implementation and validation task, not a new recovery
  contract.
