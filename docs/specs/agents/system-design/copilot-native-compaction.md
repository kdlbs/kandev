---
status: current
system: agents
requirements:
  - REQ-AGENTS-COPILOT-NATIVE-COMPACTION-001
  - REQ-AGENTS-COPILOT-NATIVE-COMPACTION-002
  - REQ-AGENTS-COPILOT-NATIVE-COMPACTION-003
  - REQ-AGENTS-COPILOT-NATIVE-COMPACTION-004
---

# Copilot Native Compaction System Design

## Ownership and scope

Agents owns the interpretation of Copilot's native observations and their provider-specific consumer guarantees.
This vertical design includes transport, normalization, durable counter projection, the root gauge, and its visible coverage.
It implements the [provider requirements](../requirements/copilot-native-compaction.md).
The [shared count design](../../ui/system-design/context-compaction-count.md) retains ordinary inference and reusable presentation.
The [standard ACP timeline design](acp-session-events.md) supplies the existing timeline renderer.

The [decision](../../../decisions/2026-11-05-copilot-native-compaction-evidence.md) records the evidence and transaction boundaries.
The existing [delivery contract](../../platform/system-design/durable-agent-delivery.md) remains authoritative for authenticated ownership, inbox admission, projection, and recovery.
No native file reader or parallel recovery authority is introduced.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-AGENTS-COPILOT-NATIVE-COMPACTION-001 | Qualification, transport, envelope validation, normalization |
| REQ-AGENTS-COPILOT-NATIVE-COMPACTION-002 | Counter ledger, atomic projection, replacement and replay |
| REQ-AGENTS-COPILOT-NATIVE-COMPACTION-003 | Root usage and accounting |
| REQ-AGENTS-COPILOT-NATIVE-COMPACTION-004 | Compatibility, reporting, presentation, verification |

## Evidence and qualification

The [issue comment](https://github.com/kdlbs/kandev/issues/5089#issuecomment-6672451872) supplies sanitized frames for CLI 1.0.103, ACP v1.
Two child calls produced no compaction. Manual `/compact` produced one successful applied completion with `compactionAttemptId`.
Root native usage omitted `params.agentId`; child usage supplied it.
A later `session/load` did not replay native compaction notifications.

The [native schema](https://github.com/github/copilot-sdk/blob/f2b720c8ec368e035a4d5c72b012fd41b0d533e4/nodejs/src/generated/session-events.ts) at the CLI 1.0.103 snapshot supports optional attempt identity and outcome fields.
The [event reference](https://docs.github.com/en/copilot/how-tos/copilot-sdk/features/streaming-events) documents root/child attribution and separate usage observations.
Neither source proves ACP passthrough compatibility for every CLI version.

At branch base `ce9918b094855c38e5b3b4c79c1b565fe5c379cb`, the managed CLI pin was 1.0.88.
The 1.0.88 generated schema lacks `compactionAttemptId` and `outcome`; the 1.0.103 schema contains both plus the `threshold` automatic trigger.
Implementation moved only the Copilot managed pin to 1.0.103 and qualified it with version-pinned primary-source schemas and sanitized wire fixtures.
The ACP SDK replacement is `v0.13.6-0.20261106000654-ec6d622b1999`, which adds explicit exact-name notification opt-in.
Manual evidence is a live 1.0.103 capture. Automatic compaction is qualified from the pinned schema and the shared completion rule; no separate live automatic capture was available.

Use explicit `success == true`, `outcome == "applied"`, and non-empty attempt identity.
Absent fields remain non-countable on every version in this package.
`checkpointNumber`, pre/post token differences, and trigger text are not identity or success fallbacks.
If 1.0.88 lacks the needed contract, update only the Copilot managed pin to the qualified version.
Unknown or older versions remain unavailable unless a validated native frame establishes the required shape.
A successful initialize response or silence is not subscription acknowledgment.

## Transport prerequisite and negotiation

`clientCapabilitiesForAgent` in `adapter_helpers.go` builds the live initialize request.
For an enabled Copilot session, merge `_meta["github.com/copilot"].events` containing exactly:

- `session.compaction_start`
- `session.compaction_complete`
- `session.usage_info`

The diagnostic `subagent.*` subscriptions are unnecessary in production.
Keep standard `session.compaction` advertisement for its existing independent renderer.
Do not add session-only subscriptions to utility/inference probes merely because `ClientCapabilityMeta` is shared.
Preserve its Cursor/Devin metadata and other clients' capability sets.

The exact vendor method is `github.com/copilot/sessionEvent`, without a leading underscore.
The pinned SDK's `extensions.go` filters that method before Kandev's existing callback can receive it.
Add an opt-in SDK connection option for exact additional inbound notification names, provisionally `WithAllowedExtensionNotifications`.
The default remains underscore-only. The option must not alter requests, known standard methods, or outbound extension validation.
Publish/merge that SDK change and resolve its immutable dependency before changing `go.mod` and `go.sum`.
No local module-cache patch or temporary `replace` path can satisfy the work order.

Kandev enables the additional method only on Copilot connections.
`handleExtensionNotification` retains Claude dispatch and routes Copilot payloads to a provider-specific decoder in `dialect_copilot_session_events.go` (new).
Pure wire recognizers remain dialect functions under ADR 0044.
The adapter owns state, delivery, and barriers.

Advertise an optional agentctl capability `copilot-native-compaction.v1` through the existing `/identity` capability set in `server/api/identity.go`.
This must not become a globally required survival capability: older retained processes remain adoptable.
Use an optional `CopilotNativeSessionEvents` request/config bit through `CreateInstanceRequest`, instance configuration, process adapter configuration, and `shared.Config`.
A new backend enables it only when its durable projector and peer capability are available.
An old backend omits it; a new agentctl then leaves the vendor subscription off.
An older agentctl has no capability and yields unavailable reporting, with no counting fallback.
This is protocol negotiation, not a runtime feature flag or operator setting.

## Ordered dispatch and binding

Extend the existing `notifWork` union in `adapter_updates.go` with a typed vendor-notification work item.
The SDK callback enqueues an immutable payload and captured prompt/native binding; it does not normalize or persist on the receive goroutine.
Standard updates, native events, barriers, and pending-shell flushes retain one FIFO owner in `runUpdateWorker`.
Add no parallel worker or global mutex held across channel sends.
Queue admission and shutdown use the existing lifetime cancellation.

Validate `agentID == copilotAgentID`, exact method, object envelope, known type, and non-empty native `sessionId`.
Match that identity against the authorized active native conversation before normalization.
Root means the `agentId` property is absent. A present non-empty string identifies a child; null, empty, or another type is malformed attribution.
Child events are ignored for both the parent count and root gauge.
Wrong-session events cannot change reporting for the active session.

Notifications that arrive during session creation/load need the same bounded ordered binding barrier as the corresponding response.
Bind buffered observations only after the native identity is established; never attach them to whichever session happens to be current later.
Proven historical replay retains existing suppression. An `isLoadingSession` flag alone must not misclassify a genuinely live native completion as history.
Serialize successful native replacement with the same FIFO so old queued raw events cannot cross into the new conversation.

## Normalized observations and timeline

Add a typed `ContextCompactionObservation` carrier on `streams.AgentEvent` and a new event type `context_compaction_observation`.
Names are proposed additions. Preserve the immutable observation through snapshot/JSON and agentctl journal paths.
The carrier has two closed kinds:

| Kind | Minimal data |
| --- | --- |
| `completion` | Provider, native session ID, attempt ID, explicit success/applied outcome, and optional trigger |
| `reporting` | Provider/native binding, availability, coverage, and a bounded reason |

A completion is emitted only after full root-envelope and outcome validation.
The provider field uses the canonical registry identity `copilot-acp`.
A valid observation can arrive outside an active prompt; native/session ownership, not prompt existence, authorizes its count.
Use the existing durable bootstrap-event path when no submission is active. Never create a prompt, turn completion, or workflow transition to count it.
No summary, prompt, provider request ID, billing detail, or arbitrary native data is needed in the counting payload.
`dataOmitted` or replacement `{omitted: ...}` data cannot create a completion carrier.
A recognized unusable root completion emits partial reporting using only safe bounded classification.

Normalize valid starts/completions into the existing `ContextCompactionPayload` and `convertCompactionUpdate` timeline representation.
The identity remains native conversation plus attempt ID, including a completion received without a start.
Failed/non-applied completions can close their timeline entry without emitting a countable completion observation.
Timeline entries are presentation consumers; counting never occurs in tool-rendering or tool-update callbacks.
A pending start never increments and can remain unfinished if no usable completion arrives.
The existing summary and display budgets still apply to any optional timeline summary.

## Root usage and accounting

Translate valid root `session.usage_info.data.currentTokens` and `tokenLimit` into existing context-window events.
Require finite integral non-negative token values and a positive window size under existing reliability limits.
`isInitial` carries no identity or counting authority.
Root readings keep `source: "acp"` and `compaction_inference_disabled: true`.

When the native path is enabled, ordinary `usage_update` still updates existing usage/cost trackers but does not emit a competing gauge event.
Keep that accounting update before the gauge suppression branch in `convertUsageUpdate`.
Do not move cost baselines to `session.usage_info` or interpret occupancy differences as provider billing.
A fresh native binding invalidates a previous mixed or replaced gauge baseline without changing lifetime count.
Until a root sample arrives, use the existing unmeasured/cleared reading behavior.
Unsupported legacy runtimes retain their current gauge behavior and explicit unavailable counting status.

## Counter ledger and schema

Add `task_session_compactions` with a primary key on:

`(session_id, provider, native_session_id, compaction_attempt_id)`.

Store first accepted incarnation/generation, stream/sequence, and observed timestamp as audit evidence.
The unique key excludes process, stream, and execution identifiers so reconnect duplicates cannot increment again.
The same attempt ID in a different native conversation is a distinct operation.
The Kandev session owns the rows, with deletion cascading or explicit transactional cleanup using the repository's existing session-deletion path.
Retain receipts for that session's lifetime; timeline/history cleanup and delivery receipt retirement cannot prune them.
Task/workspace transfer must preserve session-owned receipts and include them in preservation checks through their owning session.
No historical rows or counts are backfilled.

Implement DDL in `session_compactions_schema.go` and wire it after session/continuity schema creation in `base_schema.go`.
Both SQLite and PostgreSQL must support fresh creation, existing-database adoption, and repeated initialization.
This is a new table. Any later existing-table column requires the normal migration path, not CREATE-only alteration.

The existing `context_compaction_count` remains the displayed lifetime aggregate, preserving prior inferred increments.
New receipts add to it; the label must never imply that historical values are wholly confirmed.
No second competing total or process-local deduplication authority is added.

## Atomic canonical projection

Handle the new carrier inside `ProjectCanonicalAgentDeliveryEvents` / `projectCanonicalDeliveryBatchGroupTx`, before marking the inbox event projected.
The receive-and-project combined path must call the same transaction-owned helper.
Use a focused `session_compactions.go` helper for SQL and a small branch in canonical projection, rather than growing that file's existing functions.

Under existing authenticated session/stream locks:

1. Verify retained delivery ownership, provider `AgentType`, and the observation's native identity against the recorded harness generation/rebind history.
2. Lock the session row before reading or changing its aggregate/reporting metadata.
3. Insert the unique receipt with conflict-do-nothing behavior.
4. Increment the latest count only when the receipt is newly inserted.
5. Merge reporting metadata without replacing context usage or unrelated metadata.
6. Mark the inbox/effect/cursor using the existing canonical transaction, then commit.

SQLite retains immediate writer admission. PostgreSQL follows existing sorted session/stream advisory lock order and row locking.
A failure after receipt insertion or count mutation must roll back everything, including projection state.
Concurrent explicit events and existing context-window writes must not lose increments or metadata keys.
Malformed observation data fails closed and cannot poison or grant another session's receipt.

Extend `CanonicalAgentDeliveryProjectionResult` only as needed to return committed count/reporting metadata for post-commit publication.
Publish through the existing `session.state_changed` metadata path after commit.
No event-bus callback performs the authoritative increment.
A failed notification can be repaired by normal hydration or republishing current stored metadata; it must not repeat the increment.

`ReplayRecoveredDeliveryWithoutLifecycleCallbacks` still executes canonical projection and therefore retains the count effect.
Live projection and recovery must not depend on a tool callback executing successfully after its generic receipt was completed.
For peers without a compatible durable path, report unavailable instead of adding best-effort increments.

## Native replacement and retained history

A newly arriving raw event for a previous native conversation is rejected at the adapter binding boundary.
An already journaled completion remains bound to its original authenticated generation.
It can increment the owning Kandev lifetime count once during recovery, even after native replacement, but cannot overwrite the new gauge.
Consult both `harness_session_generations` and `harness_native_session_rebinds` when a reset moved the native ID within one generation.
Do not validate historical accepted evidence solely against the current `resume_token`.

Context reset, model change, and native replacement retain the lifetime total and receipt table.
Reusing the same native session after a reconnect preserves the deduplication domain.
A new task session receives a separate domain.

Initially classify new observation payloads as protected retention barriers.
Do not add them to the generic control-pruning allowlist as part of this package.
The authoritative ledger survives existing transport receipt retirement; a later retention extension must prove that separately.
Existing delivery expiry and immutable-hash rules remain unchanged.

## Reporting and presentation

Persist `context_compaction_reporting` adjacent to the existing count with this additive shape:

| Property | Values / meaning |
| --- | --- |
| `source` | `usage_drop` or `copilot_native` |
| `availability` | `available` or `unavailable` for the current runtime |
| `coverage` | `observed` or `partial`; never a completeness claim |
| `revision` | Monotonic per-session revision for native count/reporting projection |

Missing metadata preserves legacy parsing and help for ordinary inferred counts.
The backend derives this state; React does not inspect provider identity or raw native events.
Available means qualified collection is configured or validated native evidence arrived, not that a silent stream proves zero provider activity.
Known evidence loss makes partial coverage sticky for the Kandev session lifetime, including later successful events and native resets.
Availability can change with the runtime; changing it does not erase the lifetime coverage limitation.
Unknown dropped notifications cannot be detected from silence. Every native explanation therefore discloses potentially missed events.

Extend `ContextWindowEntry`, `parseContextWindowEntry`, live session patches, boot hydration, and `useSessionContextWindow` to preserve the reporting object.
A count-only native update must work before or after a gauge sample without inventing a zero-token context window.
Increment the reporting revision atomically with every native count/reporting mutation.
Clients reject older reporting revisions and merge lifetime counts monotonically across live patches and HTTP/boot hydration.
An older gauge patch can still update its eligible gauge fields, but cannot lower a newer count.
These are projection freshness rules; only backend SQL increments the authoritative total.
Historical replay can add its accepted count, but only current-runtime reporting can change current availability.
Render a count whenever the existing reliable context surface is present; retain its established visibility rules otherwise.

Reuse `ContextCompactionCount` in `token-usage-display.tsx`:

- Available/observed: show the count and info. Explain successful parent event increments and retained historical estimates.
- Partial: show the count with a short localized `Partial` status and the explanation.
- Unavailable: show the retained count with localized `Unavailable`; never describe zero as proof of no compaction.

No new overlay, navigation, action, or scroll owner is needed.
Desktop uses the current hover/focus interaction. Phone uses the current tap-pinned surface and info toggle.
The nearest phone exemplar is `mobile-context-window-source.spec.ts`.
Add seven-language copy, generate Traditional Chinese and pseudo, and cover long translated status/help content on phone.
The default inferred-provider explanation remains intact.

Update the public context-window section in `docs/public/sessions-and-review.md` during implementation.
Explain supported native collection, unavailable old peers, observed/partial totals, and historical estimates.
Do not promise native-history backfill or recovery of provider-side dropped notifications.

## Verification and security

Use transport-level JSON-RPC tests for the exact unprefixed method; directly calling the callback cannot prove SDK dispatch.
Wire fixtures must include root/child identity, success/non-applied/unknown/missing outcomes, omitted data, and start/completion ordering.
Use controlled barriers around load/reset, full queues, and shutdown; arbitrary sleeps do not establish ordering.

SQLite and real PostgreSQL tests cover atomic rollback, duplicate attempts across streams, concurrent writers, reopen, native replacement, and projection-only recovery.
A real E2E fixture must traverse the Copilot-scoped raw method, adapter, durable stream, SQL, live WS update, and reload.
Use `NewMockAgentWithID` / `KANDEV_MOCK_PROVIDERS` to register the synthetic peer as Copilot in isolated tests.
Do not weaken provider validation for the generic mock agent or seed the visible count through the browser store.

Provider payloads remain untrusted. Bound parsing, copy only required fields, sanitize logs, and retain existing markdown sanitization for timeline summaries.
No extra metric is required. Existing diagnostics may record bounded rejection reasons and IDs only in session-scoped logs.
Native summary text, provider tracing fields, private file paths, and billing records are not generic diagnostic data.

## Delivery package

- [Plan and work orders](../../../plans/copilot-native-compaction-count/plan.md).
- [Prior safe exclusion](../../../plans/copilot-compaction-inference/plan.md).
