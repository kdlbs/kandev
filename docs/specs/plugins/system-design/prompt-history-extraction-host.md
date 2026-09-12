---
status: current
system: plugins
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-001
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-002
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-003
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-004
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-005
created: 2026-09-06
owners:
  - kandev
---

# Prompt History Plugin Host Prerequisites System Design

## Purpose and boundaries

This design adds the narrow public browser Host contracts required for a future external prompt-history plugin. The host continues to own authorization, transcript persistence, transport reconciliation, task-panel placement, native navigation, custom-prompt disclosure, and browser-local favorite state. The future plugin owns prompt-list derivation and presentation through those contracts.

The existing core feature remains the reference implementation. This package does not extract it. The [UI prompt-history requirements](../../ui/requirements/prompt-history-panel.md) remain authoritative for the product outcome; this plugin-system design is authoritative for the prerequisite boundary.

## Requirement mapping

|Requirement|Design sections|
|-------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
|`REQ-PLUGINS-PROMPT-HISTORY-HOST-001`|[Task-panel contract](#task-panel-contract), [Mobile design contract](#mobile-design-contract)|
|`REQ-PLUGINS-PROMPT-HISTORY-HOST-002`|[Browser conversation facade](#browser-conversation-facade), [Backend read routes](#backend-read-routes), [Snapshot and event reconciliation](#snapshot-and-event-reconciliation)|
|`REQ-PLUGINS-PROMPT-HISTORY-HOST-003`|[Scoped transcript navigation](#scoped-transcript-navigation)|
|`REQ-PLUGINS-PROMPT-HISTORY-HOST-004`|[Host-owned display dependencies](#host-owned-display-dependencies)|
|`REQ-PLUGINS-PROMPT-HISTORY-HOST-005`|[Compatibility and extraction sequence](#compatibility-and-extraction-sequence), [Verification architecture](#verification-architecture)|

## Contact-point inventory and classification

Classification meanings:

- **sufficient API contract:** a future plugin can consume the existing public contract without depending on Kandev internals;
- **lacking API contract:** behavior exists only as private application code or state;
- **covered but contract change required:** a public or shared contract exists, but lacks fields, scoping, lifecycle, or presentation behavior required for parity.

### Frontend integration points

|Current contact point|Current responsibility|Classification|Target|
|----------------------------------------------------------------------------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
|`apps/web/components/task/prompt-history-panel-content.tsx` (`PromptHistoryPanelContent`)|Coordinates active session, prompt pages, turns, loading/error/empty state, pagination sentinel, and rows.|lacking API contract|Future plugin composes public conversation hooks; core component stays until extraction.|
|`apps/web/components/task/prompt-history-panel-row.tsx` (`PromptHistoryRow`)|Expansion, timestamps, durations, favorites, prompt mentions, accessible row navigation, touch/fine-pointer sizing.|lacking API contract|`host.ui.PromptMentionText`, `host.conversation.useMessageFavorite`, scoped `openMessage`, existing UI/i18n utilities.|
|`apps/web/lib/prompt-history.ts` (`buildPromptHistoryEntries`)|Filters user prompts, stable ordering, duration bounds, sender-task marker, absolute prompt index.|lacking API contract, but plugin-owned logic|Copy behavior into the future plugin through public DTOs; no host utility is required.|
|`apps/web/hooks/domains/session/use-session-prompts.ts`|Gap-aware initial user-message read and private prompt projection.|lacking API contract|`host.conversation.useSessionMessages`.|
|`apps/web/hooks/use-lazy-load-prompts.ts` and `use-lazy-load-sentinel.ts`|Older-page loading, stale request guards, and intersection rearming.|lacking API contract for data; UI sentinel is plugin-owned|Host hook owns request lifecycle and `loadMore`; plugin may own intersection presentation.|
|`apps/web/lib/state/slices/session/prompt-message-actions.ts`, `session-slice.ts`, and `types.ts`|Private prompt cache, ordering, merge/delete, loading metadata, generation fencing.|lacking API contract|Private implementation behind the Host hook; never exposed to SDK consumers.|
|`apps/web/lib/api/domains/session-api.ts` (`listTaskSessionMessages`, `listSessionTurns`)|First-party REST wrappers.|lacking API contract|Host implementation calls plugin-scoped routes; plugins never import or call these wrappers.|
|`apps/web/lib/ws/handlers/messages.ts` and `lib/types/session-events.ts`|Typed core handling for added/updated/deleted message events.|covered but contract change required|Host facade consumes the transport and publishes stable SDK DTOs; raw `registerWsHandler` remains available but is not the prompt-history contract.|
|`apps/web/hooks/domains/session/use-session-turns.ts` and `turns.bySession`|Private turn hydration and live completion state.|lacking API contract|`host.conversation.useSessionTurns`.|
|`apps/web/components/task/prompt-history-panel-host.tsx` and `lib/state/dockview-store.ts` (`scrollTranscriptToMessage`)|Desktop chat focus/create, around-window target load, and scroll ownership.|lacking API contract|Generation-bound task-panel `conversation.openMessage`.|
|`apps/web/components/task/mobile/session-mobile-layout.tsx` (`handleNavigateToPrompt`, `mobileScrollTarget`)|Mobile switch-to-chat and local scroll target.|lacking API contract|Mobile panel host injects the same scoped navigation capability.|
|`apps/web/hooks/domains/session/use-message-favorite.ts` and `lib/state/slices/message-favorites/`|Reactive per-tab favorite state in `sessionStorage`.|lacking API contract|Read-only `host.conversation.useMessageFavorite`.|
|`apps/web/components/task/chat/messages/prompt-mention-components.tsx` and `hooks/domains/settings/use-custom-prompts.ts`|Prompt alias recognition, previews, live prompt settings, hover-card/drawer adaptation.|lacking API contract|Host-owned `host.ui.PromptMentionText`; custom-prompt data remains private.|
|`apps/packages/plugin-sdk/src/index.ts` (`HostReact`)|Runtime-free React structural type.|covered but contract change required|Add `useLayoutEffect`; do not add a React runtime dependency.|
|`apps/packages/plugin-sdk/src/index.ts`, `apps/web/lib/plugins/types.ts`, `apps/web/lib/plugins/host-api.ts`|Public Host interface and concrete implementation.|lacking API contract|Add `host.conversation` and display dependencies as one mirrored contract.|
|`apps/packages/plugin-sdk/src/index.ts` (`TaskPanelRegistration`, `PluginTaskPanelProps`)|Generic desktop/mobile panel contribution.|covered but contract change required|Add localized title key, visibility context, session kind, and scoped navigation capability.|
|`apps/web/components/task/plugin-task-panel.tsx`, `lib/plugins/registry.ts`, and `lib/state/layout-manager/plugin-panels.ts`|Resolve, render, persist, and revoke generic plugin panels.|sufficient API contract for identity/lifecycle; contract change required for new props|Keep one `plugin-panel` component and extend its context plumbing only.|
|`apps/web/components/task/dockview-add-panel-items.tsx`|Desktop add-panel entry and passthrough filtering for the core panel.|covered but contract change required|Evaluate generic task-panel visibility instead of prompt-history-specific plugin logic.|
|`apps/web/components/task/mobile/plugin-panel-picker.tsx`, `session-mobile-bottom-nav.tsx`, `session-mobile-layout.tsx`|Grouped mobile Panels entry and full-height plugin panels.|sufficient API contract for placement; contract change required for visibility/navigation|Reuse the picker/full-height surface and inject context/capability.|
|`apps/web/lib/state/layout-manager/constants.ts`, serializer, `apps/web/lib/layout/layout-profiles.ts`, `lib/state/layout-manager/plugin-panels.ts`, and layout editor|Built-in `prompt-history` identity and saved layout behavior.|covered but contract change required|Extend reusable-panel validation, serializer/restore, late plugin registration, removal/drop behavior, and editor definitions for `plugin:<pluginId>:<panelKey>`. All title reads use the reactive `resolveTaskPanelTitle`; built-in-to-plugin saved-ID migration remains later extraction work.|
|`apps/web/src/locales/*/task.json` prompt-history keys|Core panel copy in five shipped locales.|sufficient core contract, not public plugin API|Future plugin registers its own catalogs through existing `host.i18n`; core keys stay in this package.|
|Existing `host.ui`, `host.i18n`, `host.utils`, and `useResponsiveBreakpoint`|Design-system primitives, translation, relative time, `cn`, and pointer/viewport state.|sufficient API contract|Reuse unchanged except for the prompt-mention component and React lifecycle type above.|

### Backend integration points

|Current contact point|Current responsibility|Classification|Target|
|---------------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------|---------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
|`internal/task/repository/sqlite/message_prompt_index.go`, `message.go`, and migrations|Atomic durable `prompt_seq`, absolute prompt indexes, deterministic keyset pagination, SQLite/Postgres parity.|sufficient API contract behind services|Reuse unchanged.|
|`internal/task/handlers/message_handlers.go` and `internal/task/dto/dto.go`|First-party session message endpoint with user filtering, order, cursor, around-window, and `prompt_index`.|covered but contract change required|Share parsing/service behavior with a plugin-scoped authenticated read route; do not expose the first-party URL as plugin API.|
|`internal/task/handlers/task_http_handlers.go` and `dto.TurnDTO`|First-party session turn list.|covered but contract change required|Add a plugin-safe route that omits metadata and runtime fields.|
|`internal/task/models.Message.ToAPI` and `pkg/api/v1.Message`|Browser message DTO including metadata and optional raw content.|covered but contract change required|Map a narrower browser-plugin DTO with sanitized content and explicit `senderTaskId`; never reuse raw fields blindly.|
|`internal/task/service.Service` and repository interfaces|Authorized task/session reads and message/turn access.|covered but contract change required|Add a narrow conversation read interface exposing actor/session authorization, plural author filters, deterministic keyset pages, turns, and minimal DTO inputs; plugin handlers call this interface, never SQLite directly.|
|`internal/backendapp/helpers.go`, backend composition, and plugin route registration|Wires plugin handlers with their dependencies.|lacking API contract|Inject the narrow conversation read interface into `internal/plugins` and preserve normal user/workspace authorization; add composition-level tests proving inaccessible sessions do not leak existence.|
|`internal/plugins/host_data.go`, mappers, `pkg/pluginsdk`, `plugin.proto`|Separate Go backend-plugin conversation contract.|unchanged|No browser prompt-history change; it is not used by the UI bundle.|
|Task message/turn event publication and WebSocket session subscription|Sanitized message/turn/removal events.|contract change|`SessionEventStream` is the sole durable per-session log, sequencer, and fanout. Mutation services append transactionally; migrate subject subscriptions and direct broadcasts. Client coalescing preserves sequence, holds gaps, and never publishes a second stream.|

### Shared types, contracts, and documentation

|Current contact point|Classification|Target|
|------------------------------------------------------------------------|------------------------------------|-------------------------------------------------------------------------------------------------------|
|`docs/plans/plugins/PLUGIN-API.md`|covered but contract change required|Canonical prose/type contract gains conversation and task-panel additions.|
|`apps/packages/plugin-sdk/src/index.ts`|covered but contract change required|Runtime-free source of consumer types for the same additions.|
|`apps/web/lib/plugins/types.ts`|covered but contract change required|Internal aliases and concrete refinements remain assignable to the SDK.|
|`docs/public/plugins-authoring.md` and `docs/public/plugins-manifest.md`|covered but contract change required|Document browser reads, `api_read:messages`, lifecycle, and minimum host version during implementation.|
|`docs/specs/ui/requirements/prompt-history-panel.md` and system design|sufficient product behavior contract|Remain unchanged; link this prerequisite package.|
|ADR 0047 and ADR 2026-08-01 task-panel contributions|sufficient related decisions|New browser-facade ADR narrows the missing browser boundary without changing either decision.|

### Current test coverage

|Coverage|Current evidence|Classification|Planned proof|
|-----------------------------------------------------------------------|----------------------------------------------------------------------------|---------------------------------------|----------------------------------------------------------------------------------------------------------------|
|Prompt ordering, durable indexes, deletion, pagination, SQLite/Postgres|backend prompt-index and message-list repository/handler tests|sufficient existing regression coverage|Keep; add plugin-route mapping/authorization cases only.|
|Prompt derivation and formatting|`apps/web/lib/prompt-history.test.ts`|sufficient core reference|Future plugin parity fixture asserts public DTOs permit the same output; do not move core tests in this package.|
|Prompt panel states, aliases, favorites, accessibility, pagination|`prompt-history-panel-content*.test.tsx`, row tests|sufficient core reference|Add Host facade/component tests through public APIs.|
|WS message add/update/delete and prompt-index mapping|`apps/web/lib/ws/handlers/messages.test.ts`|sufficient transport coverage|Add facade reconciliation and lifecycle tests.|
|Desktop navigation/around-window|dockview renderer, task-chat scroll-target, and load-message-window tests|sufficient core mechanism|Add scoped capability tests and fixture-plugin E2E.|
|Plugin panel registration, restore, lifecycle, and mobile picker|registry, plugin-task-panel, layout manager, and mobile plugin-panel tests|sufficient base coverage|Add title, visibility, session-kind, and navigation-capability cases.|
|End-to-end prompt history and saved layouts|`apps/web/e2e/tests/task/prompt-history-panel.spec.ts`, layout profile specs|sufficient core reference|Add external-style fixture plugin desktop and `mobile-*.spec.ts` parity paths.|

## Public browser API contract

[`PLUGIN-API.md`](../../../plans/plugins/PLUGIN-API.md#browser-conversation-facade) is the canonical declaration for the runtime-free TypeScript types, Host-only wire envelopes, retryability mapping, lifecycle, and compatibility policy. The SDK and web Host must remain structurally assignable to that declaration.

Implementation invariants:

- Browser conversation state exposes sanitized message and turn DTOs, including non-empty `updatedAt`, nullable task selection, hydration, pagination, retry, and terminal removal state. It never exposes cursors, tokens, sidecars, event payloads, Kandev stores, or React runtime values.
- `taskId: undefined` inherits the active panel task, `null` selects all tasks in the session, and an explicit string must equal the panel task. Mismatches fail before network activity.
- `HostReact` includes structural `useLayoutEffect`; task-panel registration keeps literal `title` as the fallback and adds optional localized title, typed visibility, session kind, and scoped message navigation.
- The Host owns custom-prompt disclosure and read-only favorite observation. Plugins receive only `host.ui.PromptMentionText` and `host.conversation.useMessageFavorite`.
- Persisted update timestamps map to `updatedAt`; a missing row update timestamp falls back to `createdAt`. An unseen timestamp-less add may use `createdAt`, while a timestamp-less update is rejected for snapshot repair. Deletes remain terminal by ID.

## Browser conversation facade

`buildHostApi` creates a plugin-scoped conversation implementation alongside `storage`. The task-panel wrapper injects `history` as an opaque, generation-bound handle; its hook IDs must match the wrapper's task/session context, while no panel context yields only the nullable empty state. Hook returns are immutable SDK DTOs; no SDK type imports `AppState`, Zustand, Kandev HTTP types, or session-event payloads. Before import, the loader calls authenticated `GET /api/plugins/{id}/conversation/binding`. Success is `{bindingToken,generation,expiresAt}` with RFC3339 UTC `expiresAt` <=10m; every response, including errors, sends `Cache-Control: no-store`. Plugin-visible errors use the canonical envelope with only `401/unauthenticated/false` or `404/not_found/false`; binding-only `409/generation_superseded/true` stays loader-internal with bounded retry and never reaches plugin code. On `401` or `404` the current load is skipped without disabling persisted state; the loader never imports until binding succeeds. revoked grants rebind or abort. `host.ts` keeps grants Host-only; reload stages grant/import/init, preserving old state until successor commit and fencing only pending state on failure. Disable, uninstall, or replacement revokes old grants.

Messages are stored internally in ascending deterministic order and projected into requested order. Equal in-flight requests join per scope/query. Cache identity includes panel/plugin ID, session ID, tri-state task ID, author filter, sort, page size, and generation; cursor fingerprints use the same identity. Live matching requires exact session/task equality for inherited or explicit task strings, accepts every task ID for explicit null, and accepts `session.removed` for the selected session regardless of event task ID. Each panel has an independent cache, Host-minted consumer identity, and abort controller. Concurrent calls for one continuation join; exhausted and removed states perform no network activity. Reads never initialize `messages.bySession`.

## Backend read routes

Add authenticated plugin-scoped routes before the wildcard webhook routes:

```text
GET  /api/plugins/{pluginId}/conversation/task-sessions/{sessionId}/messages
GET  /api/plugins/{pluginId}/conversation/task-sessions/{sessionId}/turns
GET  /api/plugins/{pluginId}/conversation/binding
POST /api/plugins/{pluginId}/conversation/continuation/renew
```

Queries bound `limit` (default 20, max 100), opaque keyset `cursor`, repeated supported `author_type`, and `sort`; invalid/empty filters return `400`. Public responses are `{ messages, hasMore, cursor }` with `cursor: string | null` and `{ turns }`; `cursor` is null when no next page exists. Exact Host-only wire request/response and event payload DTOs are canonical in the `PLUGIN-API.md` Host-only conversation wire contract. Internally responses carry `snapshotCutoff` and row-sequence sidecars. The Host sends the subscribe ACK's binding `snapshot_token` in the Host-only `X-Kandev-Snapshot-Token` header; server validation applies the committed cutoff and `rowSequence <= cutoff` to every page, seeking the cursor key directly. The cursor encodes `(created_at,id,direction,cutoff,fingerprint,expiry,generation)` and rejects tampering or mismatch with `400`; it is never an offset. Before 80% expiry Host-only `continuation/renew` accepts cursor/token and returns the same cutoff/fingerprint with new expiry; renewal never picks a new cutoff. Renewal success and error responses use `Cache-Control: no-store`. If retention/generation cannot preserve it, the Host-only renew returns `409`, surfaced to plugin code as retryable `upstream_failure`; `loadMore` leaves state/cursor unchanged and explicit retry starts fresh. Each panel scope has monotonic `continuation_revision` sentinel `0` over `{cursor,snapshot_token,cutoff,fingerprint,generation}`. Renewal and `loadMore` serialize per scope and CAS-install matching revisions; page, cursor, and `hasMore` commit atomically. Stale responses drop; failed renewal leaves the tuple unchanged; close/retry invalidates it.

Both handlers:

1. require an authenticated Kandev identity;
2. resolve an active installed plugin and require `api_read:messages`;
3. resolve the task session through the normal user/workspace authorization boundary;
4. call task services and repository interfaces, not SQL;
5. map to the narrow camel-case browser-plugin DTO;
6. strip Kandev system content and omit raw content and arbitrary metadata;
7. return stable error codes for authentication, capability, input, inaccessible session, and server failures.

Handlers use a conversation-specific resolver: absent, inactive, and capability-missing plugins collapse to sanitized `404` before session authorization, unlike `activeRecord`'s `503`. All responses use `{"error":{"code":"...","message":"...","retryable":false|true}}`; the stable errors are `401/unauthenticated/false`, `404/not_found/false`, `400/invalid_query/false`, and authorized `5xx/upstream_failure/true`. Binding and continuation-renewal success and error responses use `Cache-Control: no-store`. A narrow exact-path auth error-writer branch emits the same `401` envelope; no broad plugin-route exemption is allowed.

Message mapping reads only the allowlisted `metadata.sender_task_id` key; non-empty string values become `senderTaskId`, while missing, empty, or non-string values omit that field. Turns omit metadata/runtime; first-party REST and Go RPC contracts remain unchanged.

`SessionEventStream` owns session sequencing and fanout for message, turn, and removal events. Subscribe is discriminated: a core request requires the Host-minted `wire_id`; a plugin request requires `plugin_id`, `generation`, `binding_token`, and its Host-minted `consumer_id`. Identity fields are mutually exclusive. Bindings check user, capability, and generation, and registration atomically captures the event watermark.

The exact Host-only subscribe/ACK envelopes, legacy parsing rules, result states, and token field presence are canonical in the `PLUGIN-API.md` Host-only conversation wire contract. `SessionEventStream` validates outer IDs, session/binding identity, integer ranges, contiguous ACK advancement, and current/prior retry tokens. It binds resume and snapshot tokens to user, plugin, session, consumer, generation, and cutoff; snapshot commit precedes ACK and releases buffered post-cutoff events. `fresh`, `replay`, `gap`, and `invalid_resume` retain their documented reconciliation semantics. Events use `{type:"session.event",protocol_version,event_type,session_id,task_id, sequence,event_id,payload}`. Canonical policy is `{1:{ignorable:["session.workspace_sources.updated"]}}`; only a type listed by its matching version may advance ACK without projection. Missing, malformed, or unsupported versions and other unknown types remain durable poison: the cursor stays before the event, diagnostics retain its ID/version, and advancement stops. Rebind atomically snapshots past poison at a new cutoff, commits that cutoff, and returns a replacement token/cursor; the poison is not projected. `session.removed` is terminal regardless of whether it precedes or follows poison, and never becomes a legacy data update.

Snapshot reads are current-state reads captured at committed cutoff, not an as-of query. `SessionMessageVersion` and `SessionTurnVersion` are immutable, keyed/indexed by `(entity_id,row_sequence)` plus session/task identity; each entity selects maximum `row_sequence <= cutoff`. A tombstone at or below cutoff excludes its entity, while a later tombstone remains post-cutoff; apply the terminal session barrier after selection. The adapter strips sidecars, seeds row sequences, buffers post-cutoff events, deduplicates reflected rows, then replays. Core delivery uses a per-wire `legacy-core/<wireId>` cursor: reconnecting the same wire resumes its cursor, a new wire starts at its registration watermark, and each wire gets at most one projection per `event_id`/sequence. Projection precedes ACK; `message.added|updated|deleted` and `session.turn.started|session.turn.completed` map to existing core notifications, while removal is terminal. Merge uses ID with sequence as ordering/fencing authority; `updatedAt` is freshness only.

The durable append API returns `appended`, `duplicate`, `late_terminal`, or `sequence_conflict`; identity/payload conflicts are poison and late-terminal events drop after fencing. Version, tombstone, and log rows allocate one locked per-session sequence in one transaction; immutable `SessionEventLog` rows and `SessionDeliveryCursor` rows hold ACK, identity, epoch, lease/retry/generation. `SessionDeliveryDispatcher` owns poison: 30s lease, five attempts, 30s-capped backoff. Host-only `session.event.poison.requeue` requires `session_events:requeue`, audits actor/epoch, is idempotent, and never auto-skips. Startup reclaims leases. Tokens and reconnect grace are 10m; GC and tombstones through both bounds. Bus publication is not replay truth.

## Test-only transition controls

The fixture exposes response-bounded message patch/delete and turn-completion controls. Handlers persist before typed events; completion takes RFC3339. Fixture source is `apps/web/e2e/fixtures/plugins/prompt-history-plugin/`; `e2e-plugin-ui` cleans/builds before packaging, and archive checks source/output hashes, capability, min version, and panel key.

Deletes and session removal are terminal barriers. All production callers, including plugin `PluginOwnedTaskTree`, workspace resource/task FK cascades, task/session repositories, handoff/task-agent cascades, quick-chat expiry, profile/automation/office cleanup, workflow replacement/compensation, reference cleanup, and MCP handlers invoke one transaction-aware `SessionDeletionService`. It locks/captures sessions, reserves terminal sequences, and inserts unique outbox rows before deletion/commit; pre-listing, standalone deletes, and unobserved FK cascades are forbidden. Partial cascades, rollback, retries, and duplicates are idempotent; leased post-commit dispatch handles startup drain, recovery, backoff, requeue, duplicate ACK, and poison.

## Task-panel contract

`visible` receives typed context only; omitted predicates stay visible. With no task, desktop/mobile choices hide plugin panels without calling predicates; restored panels are unavailable until context exists. Predicate exceptions are caught, logged with plugin/panel identity, and treated as hidden. `resolveTaskPanelTitle(registration,i18n)` drives menus, tabs, restored layouts, and previews; locale changes update titles without changing panel identity. `PluginTaskPanel` passes current context and revokes its generation-bound conversation capability on plugin/panel/task/session/presentation changes.

## Scoped transcript navigation

The task-panel capability exposes only `openMessage(messageId)`. `accepted` means a current mounted adapter validated the ID/context and queued asynchronous native processing; it does not mean the target was found. `unavailable` covers empty ID, stale/unmounted context, missing adapter, or mismatch. Target loading, failure, deletion, supersession, and timeout remain native Chat outcomes.

Desktop delegates to `scrollTranscriptToMessage`; mobile delegates to `MobileSessionLayout`'s local `mobileScrollTarget` and Chat switch. Neither path globalizes mobile state. The adapter validates identity before returning and Chat owns around-window loading, scroll completion, timeout, and token clearing.

## Host-owned display dependencies

`useMessageFavorite` exposes only existing boolean favorite state. `host.ui.PromptMentionText` owns alias loading, Markdown, and fine/coarse disclosure; the plugin owns list markup, expansion, duration, formatting, translation, and icons through host UI/i18n/responsive utilities. The surface is a persistent full-height panel with one `min-h-0 flex-1` scroller; picker remains a temporary inset drawer. Shared data, pagination, derivation, state, and navigation remain shared; desktop/mobile vary only in entry, hit areas, and preview hover-card versus drawer. Touch targets are at least 44 px; Pixel 5 coverage proves older loading, preview, navigation, and no overflow.

## Security and privacy

Browser routes are authenticated/capability-gated; plugins remain same-origin, not a hostile sandbox. Routes return authorized sanitized data only, never system blocks, `raw_content`, metadata, runtime configuration, credentials, or content values in logs.

## Compatibility and extraction sequence

All API additions are optional/additive under the current plugin API version:

- old task-panel registrations omit `titleKey` and `visible`;
- old task-panel components ignore added props;
- any `api_read:messages` manifest requires `min_kandev_version: "0.91.1"`; one validator enforces this in manifest/archive/install, including dev/E2E.
- existing REST and Go RPC contracts remain unchanged;

Required sequence:

1. land and verify this host prerequisite package while the core panel remains active;
2. in a separate repository/package, implement the plugin through the public SDK only and prove parity;
3. in a later Kandev extraction package, migrate saved built-in panel identities, remove core panel/state/tests/locales, and update ownership documentation;
4. publish/enable the plugin according to the release decision for that later package.

## Verification architecture

Each work order uses RED-GREEN-REFACTOR. Backend tests cover auth, capability, authorization, filters/cursors, sanitized DTOs, event ordering, deletion, outbox, and restart. Frontend tests cover readiness, deterministic merge, stale generations, reconnect, lifecycle abort, typed failures, panel title/visibility/context/navigation, and host UI privacy. SDK assignability proves runtime-free types match the Host; desktop/mobile fixture Playwright uses public APIs and keeps the core prompt-history E2E green.

## Related decisions

- [ADR 0047: Plugins read conversation content via a capability-gated Host RPC](../../../decisions/0047-plugin-host-conversation-reads.md)
- [ADR: Browser plugin conversation facade](../../../decisions/2026-09-06-browser-plugin-conversation-facade.md)
- [ADR: Plugin task panel contributions](../../../decisions/2026-08-01-plugin-task-panel-contributions.md)
- [ADR: Plugin contribution lifecycle authority](../../../decisions/2026-08-04-plugin-contribution-lifecycle-authority.md)
