---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001
  - REQ-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002
---

# Session subscription recovery

## Ownership and mapping

Platform owns shared session subscription readiness and recovery. This design extends that contract across chat, task preview, and session-status feedback.

| Requirement | Design |
| --- | --- |
| REQ-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001 | Registration ordering and ownership |
| REQ-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-002 | Entry recovery, history state, and presentation |

## Registration ordering and ownership

The gateway registers membership before acknowledging `session.subscribe` in `apps/backend/internal/gateway/websocket/client.go`.
The frontend `WebSocketClient` owns reference counts and a shared readiness promise per session and connection generation.
`fetchAndStoreMessages` in `use-session-messages.ts` waits for readiness before requesting history.
This ordering remains mandatory. Recovery must not fetch history ahead of registration.

## Entry recovery

Use explicit 10-second deadlines for subscription registration and entry status checks. Keep the general request default unchanged.
Add a typed timeout error in `apps/web/lib/ws/request-error.ts`, retaining its existing diagnostic message.
Only this typed timeout permits the new automatic retry. Backend rejection codes retain their current meaning.

The client owns subscription retries within one shared readiness promise: two attempts, with one second between attempts.
Do not increase reference counts for a retry. Concurrent consumers and unknown-session retries join an active registration episode.
Only the current connection generation can settle readiness. Last unsubscribe cancels its timer and invalidates pending results.
A disconnect cancels the episode; existing reconnect registration starts a fresh generation after reconnection.
Do not queue episode retries while disconnected. Existing inactive-thread subscription limits remain unchanged.

The resumption hook separately retries only the `task.session.status` request with the same two-attempt policy.
Keep `processResumeStatus` outside that retry operation. It runs once after a current successful status response.
Never wrap resume, restore, launch, or prompt mutations in a retry loop.
Preserve archive-state checks, request-identity guards, and existing resume eligibility policy.

After registration, history uses its existing 10-second request deadline and at most one additional timeout retry after one second.
A continuously connected initial episode therefore takes at most 42 seconds: 21 for registration and 21 for history.
Status progresses independently and takes at most 21 seconds. Automatic failure feedback appears within the 45-second requirement.
Use timers with cleanup and fake-timer tests. Background throttling is outside the continuously visible timing guarantee.
Foreground refresh and reconnect remain recovery triggers, but they join an active episode instead of resetting its budget.
After exhaustion, explicit Retry starts a new bounded episode. Disable it while that episode is active.

## History state

Extend the existing message hooks with explicit load outcome and retry action: loading, retrying, ready, or unavailable.
Keep this state associated with session identity and hydration generation. Share in-flight fetches across mounted consumers.
The hook result supplies the state to `use-chat-panel-state.ts`, `task-chat-panel.tsx`, and the shared message-list components.
A successful snapshot alone establishes history initialization, including a successful empty snapshot.

`doFetchMessages` in `use-session-message-fetch.ts` must not clear messages on rejection or mark failed hydration as successful.
Preserve message reconciliation, prompt-history independence, draft state, and scroll anchoring.
An obsolete response cannot overwrite another session or clear a newer episode's error.
Partial live messages remain visible while initial reconciliation is pending.

## Presentation

Use the existing dedicated phone chat layout in `components/task/task-layout.tsx` as the mobile composition exemplar.
Reuse the inline loading region in `components/task/chat/message-list-shared.tsx`.
History feedback lives inside the chat region. Cached messages remain below a compact notice during recovery.
The transcript remains the scroll owner. Do not add an overlay, drawer, or separate nested scroller for short feedback.
The existing dynamic viewport and composer safe-area behavior remain intact.

Use localized copy such as `Loading conversation...`, `Taking longer than usual. Retrying...`, and `Conversation could not load.`
For exhausted status checks, use `Session status is unavailable.` with Retry and a collapsed Details disclosure.
Route entry-status feedback through a typed failure kind in `SessionRecoveryFeedback`, including task page and preview consumers.
Do not globally rename actual launch failures in `describeEnsureError` or suppress provider, profile, and workspace recovery actions.
A status-only failure must not replace the transcript loading result or hide usable cached chat.
When both fail, show one history notice in chat and retain status details without a duplicate large banner.

Desktop actions use the normal 28-pixel control size. Phone or coarse-pointer actions use at least 44-pixel targets.
Phone notices place actions below the message; desktop notices keep actions on the same row when space permits.
Use one polite `role="status"` per visible operation and keep Retry's accessible name stable.
Details use a keyboard-accessible disclosure with wrapped text. Do not move focus on automatic progress.
All new copy uses existing i18n catalogs in five languages, with generated Traditional Chinese variants.

## Persistence, security, and observability

No database, protocol, public API, or permission changes are required.
Existing server-side authorization and mutation admission remain authoritative.
Retries preserve typed backend errors and expose only existing transport diagnostics in Details.
Tests correlate request action and ID; they do not record chat payloads or credentials.
The cause of the observed seven-second delivery delay is unconfirmed. This design does not claim to remove that underlying latency.

## Implementation plans

- [Original ordering repair](../../../plans/session-subscription-recovery/plan.md)
- [Delayed session entry](../../../plans/session-entry-recovery/plan.md)
