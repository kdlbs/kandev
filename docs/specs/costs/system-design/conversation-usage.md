---
status: draft
system: costs
requirements:
  - REQ-COSTS-CONVERSATION-USAGE-001
  - REQ-COSTS-CONVERSATION-USAGE-002
  - REQ-COSTS-CONVERSATION-USAGE-003
  - REQ-COSTS-CONVERSATION-USAGE-004
---

# Built-in conversation usage system design

## Boundary and existing implementation

The costs system owns accounting semantics and chat usage projections.
The [agent design](../../agents/system-design/codex-app-server.md) supplies normalized native observations.
Keep one task ledger writer in `internal/task/usage` and the existing shared pricing logic in `internal/common/costs`.
Do not depend on Office or a plugin for collection, pricing, or display.

Existing `task_usage_events` rows contain a Kandev turn ID, token components, pricing provenance, and a deduplication key.
Existing HTTP routes expose task and session totals, but not response-level detail.
`lib/ws/handlers/prompt-usage.ts` currently stores the latest prompt counts only.
`TokenUsageDisplay` shows context occupancy, not monetary spend.

This design extends the [legacy ledger contract](../../task-cost-ledger/spec.md).
Its original AC-23 arithmetic remains authoritative for existing token columns.
Native observations add a separate event admission path and optional attribution/breakdown columns.
No historical rows change meaning. The implementation must amend the legacy scope wording with a link to this extension.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-COSTS-CONVERSATION-USAGE-001 | Measurements, attribution, persistence |
| REQ-COSTS-CONVERSATION-USAGE-002 | Prices and estimates |
| REQ-COSTS-CONVERSATION-USAGE-003 | Read surface and presentation |
| REQ-COSTS-CONVERSATION-USAGE-004 | Persistence, compatibility, validation |

## Verified protocol surface

Local schema generation on Codex 0.154.0 established these shapes without a model turn:

| Surface | Scope and fields | Availability |
| --- | --- | --- |
| `thread/tokenUsage/updated` | `threadId`, `turnId`, `tokenUsage.total`, `tokenUsage.last`, `modelContextWindow` | Standard notification, including resume replay |
| `rawResponse/completed` | `threadId`, `turnId`, `responseId`, nullable `usage` and `usageMetadata` | Marked internal-only upstream; not a universal compatibility promise |
| `TokenUsageBreakdown` | `inputTokens`, `cachedInputTokens`, `cacheWriteInputTokens`, `outputTokens`, `reasoningOutputTokens`, `totalTokens` | Counts, not currency |
| `ResponseUsageMetadata` | Nullable string `amount`, nullable arbitrary `metadata` | Schema does not establish monetary units |
| `account/usage/read` with `threadId` | Optional `threadUsage`, including `estimatedUsageUsdMicros` and credit micros | Billing-route dependent, nullable thread USD estimate |

References: [native thread schema](https://github.com/openai/codex/blob/rust-v0.154.0/codex-rs/app-server-protocol/src/protocol/v2/thread.rs),
[response metadata](https://github.com/openai/codex/blob/rust-v0.154.0/codex-rs/protocol/src/response_usage.rs),
and [resume replay](https://github.com/openai/codex/blob/rust-v0.154.0/codex-rs/app-server/src/request_processors/token_usage_replay.rs).
The local generated schemas are investigation evidence in temporary storage, not checked-in product contracts yet.

## Measurements and normalized observations

Add a normalized `usage_observation` event rather than synthesizing `complete` events for accounting.
Proposed payload fields:

- Stable observation ID, source (`response` or `turn_fallback`), provider thread/turn/response IDs, and optional provider item ID.
- Kandev session and originating turn binding, parent native thread, and direct-versus-child scope.
- Observation schema version, token presence, measurement provenance, and completeness.
- Observed model/provider/service tier when available. Never infer a child's model from an unrelated parent profile.

Normalize native counts into the existing disjoint ledger components before publication.
Set `InputTokens` to native input minus cached input, and `CachedReadTokens` to native cached input.
Set `OutputTokens` to native output, which already includes reasoning, and leave native `ThoughtTokens` zero.
Add nullable `ReasoningOutputTokens` as an informational subset, excluded from the existing total and price sums.
Persist that subset in a new nullable ledger column. Existing `TokensThought` retains its existing meaning for other adapters.
The ledger total formula and contract version stay unchanged because existing columns keep their meaning.
Preserve a differing native reported total separately for diagnosis, not as a second authoritative aggregate.
Reject invalid negative values, overflow, and impossible subsets as unusable measurements.

Example: input 1,000, cached input 600, output 200, reasoning output 80 means 1,200 total tokens.
The price uses 400 uncached input, 600 cached input, and 200 output tokens.
Reasoning 80 is visible detail, not an additional price term.
For a nonzero native cache-write category, preserve it as optional `ReportedCacheWriteInputTokens` detail.
Do not populate the existing independently additive `CachedWriteTokens` until a reviewed source-specific rule establishes its meaning.
Until that rule exists, keep the price unavailable and mark the breakdown incomplete.
Do not silently discard that field or invent an additional input charge.

## Attribution and aggregation

Persist each attributable `rawResponse/completed` measurement once when it arrives.
Derive observation identity from Kandev session, native thread, and native response ID, excluding process execution ID.
This makes reconnect replay idempotent without suppressing a distinct later response.
Validate the native thread binding before the orchestrator publishes the accounting event.
Never trust a provider string as a Kandev task or session ID.

Root native turns bind to the actual Kandev turn at dispatch.
Children inherit the original root-turn association at spawn and keep their own native turn IDs.
Unbound events remain pending in a bounded buffer. On expiry, record a diagnostic gap instead of guessing attribution.
Root-turn cost includes direct and attributable child observations, with separate subtotals.
Session cost sums each observation once. Descendant rollups are display projections, not additional ledger rows.

Capture the model and Kandev prompt generation when a provider turn is first bound.
Delayed response observations and fallback snapshots use that captured attribution even if a later prompt changes the selected model or active generation.
Child observations use the child binding's model and generation, while retaining the originating root Kandev turn.
Duplicate start notifications must not replace an established attribution.

`tokenUsage.total` is a cumulative snapshot, not an increment.
`tokenUsage.last` is last-response/context detail, not whole-turn work.
On resume or fork, use restored snapshots as baselines only.
When exact response observations exist, do not also record their cumulative totals or terminal prompt summary.

If no response observations arrive during a turn, a complete monotonic baseline-to-final delta can produce one `turn_fallback` row.
Use a deterministic session/native-thread/native-turn fallback key and label it estimated.
If boundaries are missing, counters reset, or history replays, leave the turn incomplete and do not invent a delta.
If response observations cover only part of a turn, retain them as partial. Do not add an unproven remainder.
Serialize fallback selection at terminal processing and persist the selected mode.
Late response frames cannot add a second accounting mode for the same finalized turn.
The inspection fixture must establish normal event ordering before exact completeness is advertised.

Persist in-flight response observations so interrupted or crashed turns retain known work.
A completion changes attribution/completeness state, not previously recorded token amounts.
A later child completion updates its originating turn projection without generating another root completion.

## Persistence and compatibility

Extend the ledger with nullable native thread/turn/response/item identities, scope, and measurement-source columns.
Add optional subset and attribution handling to the task writer and its mirrored wire decoder.
Keep the existing bounded queue, integer arithmetic, append-only rows, ownership validation, and transactional session rollup.
Preserve old columns and API fields for plugin compatibility.
Add a small attribution/completeness record keyed by session and native turn for baseline, mode, terminal status, and recovery gaps.
This record can change state; usage amounts remain immutable.

Introduce one usage-observation publisher in the orchestrator that reuses the existing usage bus subject with versioned payloads.
The existing `publishPromptUsage` path remains for ACP.
Suppress native terminal-summary publication when native observation accounting owns the turn.
Both task and Office consumers must accept the additive observation fields before native publication is enabled.
Keep shared token/price arithmetic in `internal/common/costs`; task code must not import Office.
Office deduplicates the same observation IDs and must not treat a child observation as a new agent run.
No second task-ledger writer or plugin-side cost calculator is introduced.

Schema changes support SQLite and PostgreSQL through the current repository migration patterns.
Existing rows remain immutable. New fields default absent, and unknown observation schemas fail visibly rather than guess their meaning.

## Prices and estimates

Keep existing provenance: `provider_reported`, `models_dev_list`, and `unpriced`.
Separate token completeness from cost provenance. Exact tokens can still have an estimated price.
Use the recorded actual model and service tier when a reviewed rate exists.
Missing model identity or an unpriced tier produces an unavailable cost, not a guessed default-model price.

Do not map `usageMetadata.amount` to `ProviderReportedCostSubcents` until currency and units have authoritative evidence.
The debugger can expose the raw field. Production only retains allow-listed typed metadata.
The first version can show per-response and per-turn calculated USD estimates using existing catalogue rates.
Subscription wording states that this is an API-equivalent estimate, not an additional subscription charge.

When available, `threadUsage.estimatedUsageUsdMicros` produces a separate provider-estimate snapshot.
Store amount, currency, thread scope, observed time, and stale/unavailable state without adding it to ledger totals.
Use int64 decimal parsing and decimal strings across browser boundaries when values exceed safe JSON integer precision.
Credit micros stay credits and never pass through a USD formatter.
Refresh on explicit detail requests and completed root turns with bounded coalescing, not per text delta.
Failure does not block chat or erase the last known snapshot.
Forked thread estimates can include inherited history, so label their native-thread scope explicitly and keep session recorded totals separate.

## Read surface and presentation

Keep existing task/session totals routes.
Add authorized `GET /tasks/:id/sessions/:sessionId/usage/turns` with cursor and limit.
Add `GET /tasks/:id/sessions/:sessionId/usage/turns/:turnId` for one Kandev turn and its bounded response detail.
Include counts, per-category presence, completeness, direct/child subtotals, price sources, and last-response detail.
Native thread IDs never replace Kandev route ownership.

Publish a small `session.usage_updated` invalidation after a committed ledger write.
Frontend reads the committed projection; it does not calculate USD from streamed tokens.
Coalesce invalidations and refresh on reconnect, session activation, and explicit detail opening.
Guard late fetches by task/session identity. A pricing delay shows pending state rather than zero.
Store a latest response summary without implying a one-to-one match to the last rendered message.
Attach turn usage to the turn footer, not to every text fragment.

Desktop: a compact turn footer opens a usage popover, and session chrome opens a totals disclosure.
Phone: the same visible Usage control opens an inset drawer using `useTouchDrawer` and the `MobilePickerSheet` geometry pattern.
Order: this turn, last response, direct/child split, token breakdown, price provenance, then session totals.
A long response list uses one scrollable body with a fixed header and safe-area padding.
Use 44px touch targets, 28px desktop controls, localized labels, focus return, and no horizontal page overflow.
Context occupancy stays in its existing control with corrected native provenance.

## Validation

Prove two responses within one turn, duplicate replay, resume baselines, interrupted turns, late child usage, and fork history exclusion.
Test 1,000/600/200/80 breakdown arithmetic and mixed ACP/native aggregates.
Test exact zero, missing counts, unavailable prices, unknown currency, unknown tiers, and integer overflow.
Test both Office and task consumers against the same observation, with the plugin absent and Office disabled in the UI scenario.
Test authorization and stale response guards for every read route.
Desktop and mobile E2E cover detail access, pending/error states, reload, background activity, and provenance labels.
