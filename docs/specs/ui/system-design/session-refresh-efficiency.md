---
status: draft
system: ui
requirements:
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-001
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-002
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-003
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-004
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-005
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-006
---

# Session Refresh Efficiency System Design

## Purpose and boundaries

The open task page owns client-side read coordination and render identity. The
trace recorded on 2026-09-29 spans 5.85 seconds. It contains six full session
responses of 77,279 decoded bytes each. Five responses preceded main-thread
tasks of 34–38 ms. Two environment-status consumers also issued near-simultaneous
reads at the start and three seconds later. The trace does not contain response
bodies, so equal lengths do not prove equal content. A 309 ms profiler-start
task and browser extensions make it unsuitable as a page-load benchmark.

The existing [Platform recovery contract](../../platform/system-design/session-subscription-recovery.md)
requires a state snapshot after subscription. This design keeps that snapshot,
the bounded reconciliation loop, and all current state freshness guards.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-SESSION-REFRESH-EFFICIENCY-001` | [Conditional session reads](#conditional-session-reads), [Session publication](#session-publication), [Failure and recovery](#failure-and-recovery) |
| `REQ-UI-SESSION-REFRESH-EFFICIENCY-002` | [Shared environment reads](#shared-environment-reads), [Failure and recovery](#failure-and-recovery) |

Added requirement mappings: `003` maps to Progressive route hydration; `004`
maps to Editor and render lifecycle; `005` maps to PR feedback coordination;
`006` maps to Shared initialization and health reads. These suffixes refer to
`REQ-UI-SESSION-REFRESH-EFFICIENCY-*` below.

## Conditional session reads

`GET /api/v1/task-sessions/:id` remains the full authorized session contract.
After the existing service read and runtime DTO enrichment, the task handler
serializes the exact response representation. It computes an opaque strong ETag
from those bytes and sends it with private revalidation headers. The tag must
include transient fields such as foreground activity, cancellation, parked
state, steering, and pending action. The `updated_at` field alone is insufficient. The
handler evaluates `If-None-Match` only after authorization and DTO assembly.
An exact match returns `304 Not Modified`, the ETag, and no body. A missing or
different validator returns the current `200` JSON response and ETag. The
server never emits session data in the tag. Existing clients without a
validator still receive the full response.

The full-session projection reserves its pending-action revision before the
repository read so a delayed snapshot cannot outrank a newer event. For this
snapshot path, the service retains the revision associated with an unchanged
pending-action value. If a newer projection has already completed while this
read was pending, the response uses that newer action and revision. This keeps
the full response stable across no-op polls without weakening cross-channel
ordering or omitting the revision from the response.

`fetchTaskSession` adds a dedicated conditional-read method returning a typed
`updated` or `unchanged` result, rather than treating HTTP 304 as a generic
`fetchJson` error. Its request uses explicit revalidation semantics so browser
cache behavior cannot silently substitute an old full response. The validator
is owned by the per-store, per-session reconciliation record in
`session-state-reconciler.ts`. It is not a global API cache. A server without
ETag support leaves the current full-response path intact. The client clears
the validator when its owner is released or a connection generation changes.

This choice retains the full response's compatibility and freshness semantics.
A narrow status endpoint requires an audit of every session field that the
reconciliation path currently repairs. Defer it unless measured 304 rates show
that the full DTO changes too often. Unconditional polling would
continue the observed transfer and parse cost.

## Session publication

On `304`, the reconciliation owner does not call `setTaskSession`. It still
advances the existing bounded polling schedule. On `200`, it applies the
response through the current hydration-epoch and stale-event checks. A
matching validator on a duplicate full response also avoids a redundant
publication. The store retains its current merge rules for metadata,
optimistic state, cancellation, parked state, and pending action. Do not use
`updated_at` alone as an equality check or replace a newer WebSocket state
with an older HTTP response.

The server-side ETag is the representation identity. The client does not
deep-compare the 77 KB object every second merely to avoid a render. A
low-cost changed-field guard can be added only if it is proven to preserve
fields that can change without a new `updated_at` value.

## Shared environment reads

`useTaskEnvironment` and `useExecutorEnvironmentAvailability` currently call
`fetchTaskEnvironmentLive(taskId)` through separate timers. A task-keyed,
reference-counted client resource owns one in-flight request, the latest
successful result, and one timer. Each consumer registers its requested mode:
active uses the current 3-second cadence and background uses 7 seconds. The
resource chooses the shortest live interval, then returns to background
cadence when the active consumer releases. Mounting a second consumer joins an
in-flight request and receives the same result. Manual refresh joins the
current request, or starts one immediately when idle.

The shared resource stores raw `TaskEnvironmentLiveResponse`. Consumers derive
their existing status and keep local popover/reset UI state. A task identity
change releases the old owner and rejects its late result for the new task.
Last release clears the timer and retires the resource after any in-flight
request settles. Reset invalidates the cached result and prompts a fresh read.
Do not share environment status across task IDs, app stores, or authenticated
identities.

## Failure and recovery

The session reconciliation loop remains bounded to its existing 30-second
window. A transport error or non-304 HTTP error follows the current retry
path. Authorization and missing-session responses must never be interpreted
as unchanged. A `304` without a locally retained full session is invalid and
forces a new unconditional read. Reconnect obtains a new authoritative full
response before using a validator again.

An environment read error preserves the last successful status and allows the
next scheduled or manual read. A 404 retains the current no-environment
semantics in each consumer. Reset and task switch cannot publish a prior
task's late result. No new user-facing copy or layout is required.

## Validation and observability

Backend tests cover full response, ETag/304, changed runtime projection,
authorization before matching, and no body on 304. Frontend tests use
deferred responses and fake timers. They cover one owner, 304 no-publication,
changed `200` and WebSocket races, and fallback server behavior. They also
cover environment cadence, reset, and late-result rejection. Desktop and phone Playwright flows
verify the visible state/status after changes and count network requests
without relying on a fixed sleep.

Before and after traces use the same task fixture, viewport, browser build,
and a clean browser profile. Record request count, 304 hit rate, decoded
session bytes, and response-following main-thread work. The implementation is
not credited with eliminating the observed 34–38 ms work unless the repeat
capture confirms it. If most steady reads remain `200`, inspect which DTO
fields change and revise the design for a narrow projection before claiming
the bandwidth objective.

## Related designs

- [Task navigation responsiveness](task-navigation-responsiveness.md) owns
  other shared task-page reads.
- [Task surface render isolation](task-surface-render-isolation.md) owns
  repeated-row rendering and virtualization. Preserve those contracts while
  measuring task-switch editor and publication costs.

## Task-switch extension from the second trace

The [second trace report](../../../plans/session-refresh-efficiency/trace-2026-09-29-task-switch.md)
adds a separate task-switch problem. Session ETags cannot remove initial editor
creation, the optional hydration barrier, or concurrent PR reads.

### Progressive route hydration

Implements `REQ-UI-SESSION-REFRESH-EFFICIENCY-003`.
Keep the current Go boot-payload contract. Add a client-navigation path that
publishes authorized task identity and owned-session selection before optional
enrichment. Keep the existing aggregate loader for boot-compatible callers.
Reuse store-scoped outstanding task reads across the route and surface hooks;
include auth generation and request options that change response semantics.
Do not globally cache `fetchJson` or share server-side state across users.

Capture hydration epochs before requests. Publish optional resources through
current slice merge actions only while the navigation/auth generation is valid.
Reuse current loaded workspace/settings resources and their invalidation rules.
Do not reset agent-profile versions or overwrite newer WebSocket state during
late hydration. Full session/model loading must not briefly enable send with a
fallback model. Transcript, terminals, and other surfaces retain their existing
local loading and error states. Missing/forbidden task handling remains authoritative.
No previous task's content becomes interactive under the destination identity.

### Editor and render lifecycle

Implements `REQ-UI-SESSION-REFRESH-EFFICIENCY-004`.
First attribute the editor creation and broad commits using a local profiling
build with mount counters. Check composer, plan editors, closed dialogs, sidebar,
and retained Dockview panels. Record which rendered inputs changed; do not infer
component ownership from a shared bundle name or renderer-wide node count.
Then isolate the demonstrated owner using stable selectors/props and defer closed
surface editor construction until opening. Preserve the visible composer's
readiness and task-keyed drafts. Avoid blanket deep comparisons, dependency
removal, or unconditional memoization. Reuse existing file-tree/sidebar isolation.
The work order records the attributed component and failing lifecycle regression
before the implementation choice is finalized. If no owner reproduces, retain
that uncertainty instead of claiming an editor fix.

#### Composer draft settlement ownership

Implements `AC-UI-SESSION-REFRESH-EFFICIENCY-004.3`–`.6` through the
[bounded draft settlement plan](../../../plans/composer-draft-settlement-ownership/plan.md).
This is a correctness boundary within the existing session draft lifecycle,
independent of the uncompleted editor CPU/mount-cost investigation above.
UI owns reusable composer reply admission and browser-local drafts; Tasks
retains delivery, attachment claims, and conversation lifecycle authority.

`useChatInputState` owns text state, submitted text/attachment snapshots,
`handleSubmit`, and `clearAcceptedPayload`. `TipTapInput` and
`useTipTapEditor` own the real editor and its imperative handle. An imperative
`clear()` calls the current `onChange`, which saves text under the current
session. Editor updates save rich JSON under that session. The storage helpers
in `lib/local-storage.ts` actually use browser-tab `sessionStorage`, with
separate text/content/attachment keys suffixed by session ID. Do not migrate
those keys or introduce a second draft store.

Treat an owner as one hook instance's committed visit to `(taskId, sessionId)`.
Ordinary rerenders, typing, workspace readiness, and `isSending` changes retain
the visit. Committing an identity change retires it, including A-B-A and null
session transitions. Unmount or effect cleanup also retires outstanding work;
StrictMode setup after cleanup receives fresh admission. A render that has not
committed must neither retire the current visit nor authorize its own work.

Keep this admission local to `useChatInputState` and its `useDraftVisit` helper
in `use-chat-draft-visit.ts`. Associate callbacks with
their rendered owner identity and publish an opaque visit token in a layout
effect. Cleanup invalidates that token. A callback must match the committed
owner before reading mutable draft/editor refs or invoking `onSubmit`; capture
the active token before calling the submitter. On acceptance, compare the
captured token with the active committed visit before any editor clear, state
setter, history/height reset, or storage write. A token invalidated by cleanup
must never become valid again. Comparing only session IDs, text, or attachment
values cannot distinguish a returned or replaced visit.

Apply the same admission to retained `clearAcceptedPayload` callbacks and
return `false` when stale. Current callbacks still use the existing payload
matching contract. Do not change the public imperative handle shape or add
source attribution to payloads. Arbitrary callers passing an old payload to
a newly obtained current handle are outside this callback-ownership correction.

Once admission passes, retain the existing content policy: trimmed text
comparison; ordered attachment `id:deliveryMode` snapshot for ordinary submit;
`matchesSubmittedAttachments` for accepted opening payloads; newer text
preservation; text-only clearing when attachments changed; otherwise complete
draft clearing. Retain `false`/promise-rejection preservation and existing
error logging. Keep structured mentions, review comments, context payloads,
upload gating, and attachment restoration/deletion with their current owners.
Do not touch the context snapshot cleanup recently delivered in
`chat-input-area.tsx` or add cancellation/replay of transport work.

**Consumers and mobile:** `ChatInputArea` keys `ChatInputContainer` by its
clarification counter, not session identity. `SessionMobileLayout` mounts
`TaskChatPanel` without a session key. `SessionTabletLayout` passes identity
through `TaskCenterPanel` to the same unkeyed panel. Desktop Dockview, preview,
Threads, Quick Chat, and run-transcript consumers also reach the shared input;
some remount and some retain instances. One hook correction covers both.
Retain all existing surfaces, focus/scroll/touch composition, and readiness
rules. This is pure state ownership without rendered markup, copy, or geometry
changes, so focused real-editor/component tests satisfy mobile parity without
new Playwright or visual checks. If implementation needs presentation changes,
revise this assessment before extending scope.

**Evidence:** permanent behavioral tests must mount production
`useChatInputState` and real `TipTapInput` under `StateProvider` (which creates
the real app store) and `ToastProvider`. Defer only the submission acceptance
boundary. Prove current editor text, hook value, saved text/rich JSON/attachments,
and remount restoration, not only a mocked `clear()` count. Include a rendered
`ChatInputContainer` entry-point case with the real body/hook/editor and submit
control. Cover both session directions, A-B-A, same-ID replacement, retained
callbacks, unmount, StrictMode cleanup, current success/failure/newer edits,
and independent different-session composers/stores. This component boundary
does not itself prove the default message handler, WebSocket, or browser path.

No API, schema, flag, global coordinator, cache, or architectural ownership
change is introduced. Existing per-session ownership is enforced locally;
the paired design preserves the constraint without a new ADR.

### PR feedback coordination

Implements `REQ-UI-SESSION-REFRESH-EFFICIENCY-005`.
Use one store-owned feedback resource for detail, background status, and popover
consumers. Key by authentication generation, workspace, provider, owner, repo,
and PR number. Migrate the existing workspace-less feedback cache readers and
writers together. Track an invalidation generation: same-generation callers join;
a newer invalidation during a read queues one follow-up. Last release cleans up
subscriptions; an obsolete result cannot publish into the replacement scope.
Retain same-PR data on refresh errors, preserve mutation-triggered refreshes and
loading indicators, and bound inactive retention. Slow feedback stays panel-local.
Measure backend handler and upstream spans for one isolated slow read before
proposing server caching or concurrency changes. The trace does not establish
which upstream operation accounts for the header wait.

### Shared initialization and health reads

Implements `REQ-UI-SESSION-REFRESH-EFFICIENCY-006`.
Share the agent-list operation and retry sequence per store/auth generation.
Route enrichment joins that operation. Profile-version changes queue a current
refresh instead of multiplying recursive retry chains across subscribers.
Preserve empty-list settling, discovery completion, and profile-edit invalidation.

Give integration availability an explicit provider/workspace/auth key rather
than using a callback identity as the shared key. Share raw health reads and
one fastest requested cadence. A 204/null result is loaded-unconfigured, distinct
from not loaded or failed. Keep unscoped requests separate. Preserve disabled
integration gating, credential invalidation, 90-second default cadence, error
semantics, and last-consumer teardown. This is client read coordination; backend
credential storage and probing remain unchanged.

### Cross-scope validation

| Context | Behavior and evidence |
| --- | --- |
| Desktop Dockview / phone task drawer | Held enrichment allows destination navigation; local loading and touch actions remain |
| Warm return / cold task / task without session | Preserve selection and drafts; no duplicate startup or fallback model |
| Auth, workspace, PR, A-B-A transitions | Deferred stale results rejected; no cross-scope data reuse |
| PR invalidation during read | One follow-up, updated checks visible; no permanently suppressed refresh |
| Optional failure / unavailable workspace | Existing retry/recovery UI remains; no previous-task action enabled |
| Clean browser / normal plugins | Compare task-switch and steady state separately; label extension overhead |

No new public API, database, or architectural ownership boundary is introduced.
No ADR is required. Client progressive loading and resource lifetimes stay in UI.
