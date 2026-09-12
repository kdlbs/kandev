---
id: "02-add-plugin-conversation-reads"
title: "Add plugin conversation read routes"
status: done
wave: 2
depends_on:
  - "01-publish-browser-conversation-contract"
plan: "plan.md"
spec: "../../specs/plugins/requirements/prompt-history-extraction-host.md"
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-002
acceptance_criteria:
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.1
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.2
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.4
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.7
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.8
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.10
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.13
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.15
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.16
system_design:
  - "../../specs/plugins/system-design/prompt-history-extraction-host.md"
---

# Task 02: Add Plugin Conversation Read Routes

## Scope

Add the authenticated backend and Host-only transport prerequisites for browser
conversation reads. Reuse task services and repositories, return only the
sanitized SDK DTOs, enforce plugin capability and session access, and provide
snapshot, keyset pagination, binding, continuation renewal, and ordered event
transport. This task does not implement React hooks, task-panel UI, or the
external prompt-history plugin.

## Acceptance

- `GET /api/plugins/{pluginId}/conversation/task-sessions/{sessionId}/messages`
  and `GET /api/plugins/{pluginId}/conversation/task-sessions/{sessionId}/turns`
  require an active plugin, `api_read:messages`, the current user, and access
  to the requested session. Unknown, inactive, missing-capability,
  malformed-query, inaccessible-session, and invalid-cursor cases return the
  documented typed JSON error envelope.
- Query decoding preserves tri-state task selection: omitted `task_id` inherits
  the resolved panel task only at the Host facade, explicit `task_id=null`
  selects all tasks in the session, and an explicit string must match the panel
  task. The route never broadens an inherited or explicit task query and never
  accepts a task from another session.
- Message rows map to the public sanitized shape, including nullable task and
  turn IDs, stripped content, author/type, timestamps, absolute prompt index,
  and only the approved sender task ID. Raw content, system blocks, arbitrary
  metadata, credentials, and store objects are omitted. Every returned DTO has
  a non-empty `updatedAt`.
- Pages use deterministic `(created_at,id)` keyset ordering, bounded page size,
  opaque session/query/generation-bound cursors, and stable prompt indexes.
  Cursor tampering, expiry, query mismatch, generation mismatch, and session
  mismatch fail closed. Reads do not initialize or mutate the chat transcript
  cache.
- Binding and renewal are Host-loader-only operations. Binding responses and
  every error use `Cache-Control: no-store`; a binding `409` maps to bounded
  loader retry and never reaches plugin code. Continuation renewal preserves
  cutoff and fingerprint, rotates expiry/token encoding only, and maps an
  authorized `409` to retryable `upstream_failure` without changing committed
  page state. Binding tokens expire no later than 10 minutes after issuance.
- The shared ordered session stream uses Host-only messages and the production
  gateway/client path. Plugin panels carry a Host-minted opaque `consumer_id`
  and core consumers carry a distinct Host-minted `wire_id`; neither identity
  is caller-selectable or shared. Consumer binding includes plugin, generation,
  session, and the applicable identity, and reconnect preserves the same
  consumer cursor within the bounded window.
- The raw stream envelope preserves numeric protocol version, arbitrary event
  type, session/task identity, sequence, event ID, and unknown payload. The
  compatibility adapter validates the versioned registry, requires outer and
  payload event types to match, and projects only the migrated vocabulary:
  `message.added`, `message.updated`, `message.deleted`,
  `session.turn.started`, `session.turn.completed`, and `session.removed`.
  Unknown or malformed events remain durable poison and never advance ACK.
- Subscribe and ACK responses use the strict response envelope and explicit
  success/failure discriminated payloads. A success returns the ordered
  watermark, committed snapshot cutoff, snapshot token, result-specific replay
  fields, current resume token, and applicable identity. A failure returns only
  the known session and `{code,message,retryable}`. ACK rejects forward gaps,
  malformed identity/range/token input, and stale generations without advancing
  the durable cursor; only the highest contiguous sequence is acknowledged.
- Event append, version rows, deletion tombstones, and sequence allocation share
  one locked transaction. `SessionEventLog` stores immutable rows and
  `SessionDeliveryCursor` stores ACK, owner epoch, lease/retry, and generation
  state. `SessionDeliveryDispatcher` owns poison records with states
  `pending -> leased -> exhausted -> requeued`, a 30-second lease, five
  attempts, and exponential backoff capped at 30 seconds. The Host-only
  `session.event.poison.requeue` command requires `session_events:requeue`,
  accepts session, event ID, and expected owner epoch, audits actor, prior
  state, attempts, and timestamp, increments owner epoch, resets attempts, and
  returns `pending` without changing sequence or payload. Requeue is idempotent;
  stale epochs fail without mutation. Startup reclaims expired leases and never
  drops or silently skips poison. Retention preserves cursors, event rows, and
  terminal tombstones through the 10-minute token expiry and 10-minute
  reconnect grace.
- `session.removed` is a terminal barrier in both orderings relative to poison.
  A live removal and a reconnect that returns `session_removed` or a terminal
  replacement cursor expose the same terminal state to the facade: committed
  rows remain, `removed` is true, message `hasMore` is false, and later reads
  and retries do not issue network requests. A fresh not-found lookup does not
  reveal prior existence.

## TDD sequence

1. RED: add focused route, cursor, binding, transaction, stream-envelope, ACK,
   poison, retention, and terminal-removal failures.
2. GREEN: implement the smallest service, route, compatibility-adapter, and
   durable-delivery changes that satisfy the contract.
3. REFACTOR: keep HTTP DTOs separate from private repository rows and keep the
   Host-only stream types out of the plugin SDK.

## Likely files

- `apps/backend/internal/agentctl/` or the existing plugin HTTP route package
- `apps/backend/internal/plugins/`
- `apps/backend/internal/office/` and session repositories
- `apps/backend/internal/websocket/` or the shared ordered event gateway
- `apps/backend/proto/`
- `apps/packages/plugin-sdk/` only if the published type contract needs alignment
- Focused backend route, repository, stream, and compatibility tests

## Verification

```bash
cd apps/backend && go test ./internal/plugins/... ./internal/office/...
cd apps/backend && go test ./...
```

Do not implement browser hooks, task-panel plumbing, fixture UI, or core prompt
history extraction in this task.

## Current result

Added authenticated, capability-gated message and turn reads with sanitized
DTOs, signed binding/snapshot/cursor/resume tokens, deterministic repository
pagination, and typed error envelopes. The durable primary SQLite journal now
records source mutations, immutable versions/tombstones, and event sequences in
the same transaction, with startup backfill and idempotent sidecar sync.
Ordered Host subscriptions use strict identity, atomic registration, poison
delivery state, operator-only requeue, startup lease recovery, terminal
deletion fencing, atomic replacement-cursor rebind, and live retention
maintenance.

Verified:

- Focused journal, plugin, and WebSocket backend tests
- Primary journal retention test, backend `golangci-lint run ./...` (0 issues)
- `make build`

The bounded repository-wide test run was attempted with
`go test -p 2 -tags fts5 ./...`; it reached unrelated pre-existing
environment-sensitive failures in agentctl config, update-channel, and invalid
metadata tests, plus temporary disk-quota linker failures. The changed
packages passed their focused tests.
