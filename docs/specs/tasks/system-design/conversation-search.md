---
status: draft
system: tasks
created: 2026-10-19
requirements:
  - REQ-TASKS-CONVERSATION-SEARCH-001
  - REQ-TASKS-CONVERSATION-SEARCH-002
  - REQ-TASKS-CONVERSATION-SEARCH-003
owners:
  - kandev
---

# Task conversation search system design

## Ownership and current boundaries

Tasks owns persisted conversation discovery. Keep one vertical contract for
storage, authorization, API results, and desktop/phone presentation.

`useInlineTaskSearchEffect` currently calls `listTasksByWorkspace` through
`fetchMatchingTasks`. The metadata endpoint uses
`Service.ListTasksByWorkspaceWithArchiveMode`, including workspace authorization,
PR augmentation, archive filters, and repository hydration. Retain this path.
`Repository.SearchMessages` uses a session-scoped LIKE scan and remains the
local Find implementation. Do not fan it out across workspace sessions.

`task_session_messages` is canonical. Resolve task ownership through
`task_sessions.task_id`, not the message's optional denormalized task ID.
Eligible rows have `type = 'message'` and `author_type IN ('user', 'agent')`.
No metadata, tool payload hydration, reasoning, or plan reads enter this index.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-TASKS-CONVERSATION-SEARCH-001 | Query contract, Result composition, Presentation |
| REQ-TASKS-CONVERSATION-SEARCH-002 | Projection, Synchronization, Authorization and lifecycle |
| REQ-TASKS-CONVERSATION-SEARCH-003 | Query budgets, Worker lifecycle, Performance evidence |

## Projection and database capabilities

Add feature-local repository files under
`apps/backend/internal/task/repository/sqlite/`, which serves both databases.
Use a separate projection, not Office's `tasks_fts`.

Proposed tables:

- `task_conversation_documents`: stable integer `document_id`, unique
  `message_id`, `session_id`, original `body`, normalized `search_text`, and
  the source change token represented by this document. This is rebuildable data.
- `task_conversation_dirty`: `message_id` primary key, fresh opaque change
  token per mutation, first-pending timestamp, and `live`/`backfill` origin.
  It contains no text. Repeated updates coalesce without moving that timestamp.
  A canonical mutation promotes an existing backfill entry to live.
- `task_conversation_index_state`: schema/normalization version, historical
  high-water ID, last visited ID, and completion state. Checkpoints are durable.

SQLite uses FTS5 with `tokenize='trigram case_sensitive 1'` and full detail,
indexing `search_text` from the projection through an external-content index.
Use the explicit stable integer ID, never the source table's implicit rowid.
Projection insert/update/delete triggers maintain FTS postings in the same
transaction. Prove external-content synchronization, rebuild, and deletion
with actual MATCH queries. Do not copy Office's legacy FTS migration.

PostgreSQL uses a GIN `gin_trgm_ops` index over the locale-independent projection key.
Provision `pg_trgm` during the capability migration when the database role has
permission. If unavailable, report a feature-local capability failure and
continue ordinary startup. Document administrator installation and restart.
Never make the application require a superuser connection. Build the index on
the empty projection before historical backfill to avoid an unbounded startup
index build. SQLite release builds already use `-tags fts5`.

Replayable structural migrations live after canonical message schema migration
in `runMigrations`. Do not run historical INSERT SELECT or full index rebuild
on every startup. Use current migration error classifiers and explicit
capability detection. Do not mark a failed capability setup complete.

## Normalization and query contract

Normalize new conversation text and queries with Go `strings.ToLower`, without
accent removal, whitespace collapsing, or stemming. Trim only query edges.
This new contract is independent of the unchanged metadata dialect behavior.
Use one Go eligibility function: 3..200 runes and a run of at least three
Unicode letters or digits. Reject embedded NUL. Test multilingual behavior
against both real databases, including the PostgreSQL deployment collation.

SQLite uses a bound MATCH phrase, with embedded double quotes doubled, against
the normalized field. Do not use LIKE ESCAPE against FTS: that disables its
LIKE index optimization. PostgreSQL indexes a locale-independent key in `index_text`: ASCII bytes stay
unchanged; each non-ASCII UTF-8 byte becomes `u` followed by two hexadecimal
digits. Keeping ASCII unchanged preserves common-word index selectivity. A bound LIKE substring over that encoding uses pg_trgm
independently of LC_CTYPE, including databases initialized with the C locale.
An exact substring predicate on `search_text` rejects byte-boundary false
positives before the candidate limit. The original normalized field supplies
Unicode excerpt positions. The eligibility rule avoids
punctuation-only scans. SQL operators and user text never share a syntax path.
Retain accents and compare exact normalized text after index candidate lookup.

Proposed endpoint:

`GET /api/v1/workspaces/:id/tasks/conversations?query=...&limit=20`

Register the literal route explicitly alongside workspace task routes.
The service authorizes the workspace before capability checks or searches.
Pending-state disclosures inspect only that authorized workspace. Do not
expose another workspace's dirty counts or indexing activity to the client.
Clamp limit to 1..20. A successful response contains:

- `tasks`: bounded entries containing existing task DTO data plus
  `conversation_match: { session_id, message_id, author_type, excerpt, highlight_ranges }`.
- `state`: `ready`, `indexing`, `query_too_short`, `query_too_broad`,
  `query_too_long`, `unavailable`, or `limited`.
- `has_more`: whether the bounded candidate window left additional work.

Highlight ranges use Unicode code-point offsets into the excerpt. Clients
convert using `Array.from` and render text nodes with semantic marks. No HTML
from indexed content is trusted. Excerpts contain at most 240 runes.
Length and indexability limits return HTTP 200 with the corresponding state
and no conversation rows. An embedded NUL returns `query_too_broad`. Invalid
limits default to twenty, and valid numeric limits are clamped to 1..20.
Authorization failures retain existing
HTTP conventions. Do not return an exact total or accept caller-supplied task
ownership. Send private, no-store responses. Error bodies contain no query text.

## Synchronization and historical backfill

Canonical message INSERT/UPDATE/DELETE triggers only upsert a dirty ID. Register
both dialect implementations. UPDATE triggers watch content, type, author,
and session changes, not unrelated metadata. Deletion triggers also cover
foreign-key cascades and turn rollback. Trigger cost must not tokenize text.

One task-owned worker reads pending IDs, fetches current canonical rows, and
replaces or removes their projections. Live updates coalesce on a one-second cadence, below the two-second lag budget.
They run at most once per message per 500 ms. They do not wait for a streaming message to become quiet.
Use at most 32 IDs per batch and a 256-KiB ordinary text budget. Fetch bodies
one at a time. Process an oversized message alone without silently truncating
it. Memory for that operation is bounded by the largest canonical message,
not by total history. Record this cost in the large-message benchmark.

Read each dirty change token and source row from one reader snapshot. Build the
projection outside the writer transaction. Publish a prepared batch and delete only its observed dirty tokens in one
writer transaction. One commit per batch avoids per-message fsync amplification
on SQLite's FULL synchronous writer. If that
token changed, discard the prepared projection and retry later. Use `FOR UPDATE SKIP LOCKED` for the PostgreSQL dirty-row comparison.
Locked rows remain queued for a later pass; do not acquire canonical message
row locks. Every source deletion fences its dirty row before removing projected
text, including messages whose eligibility changed. This prevents reverse-order
multi-message deletion from deadlocking with a publication batch. Canonical writers lock source rows before triggers touch the
dirty queue, so the worker must never invert that lock order. A new update
committing after projection publication recreates or increments dirty state.
Tests must force both commit orders, delete/reinsert, and worker restart.

Queries exclude any projection whose message currently has a dirty entry.
This intentionally hides an updating message briefly instead of exposing an
old excerpt. Continue indexing streaming messages periodically. A pending or
failed queue produces an indexing/unavailable disclosure, not a false claim
that the whole history was searched.

Install dirty triggers before capturing the historical upper message ID.
Backfill eligible user/assistant message IDs in bounded keyset pages between
an exclusive checkpoint and that upper bound. Filter tool/system records before
the page limit so their bodies do not consume the indexing batch. Source INSERT triggers capture later messages, including
random UUIDs below the checkpoint. Backfill enqueues IDs with ON CONFLICT DO
NOTHING so it cannot overwrite live change tokens. Commit each enqueue page and
checkpoint together. Missing/deleted IDs are harmless. Service three live batches for each historical batch while both queues have
work, using first-pending order within each. A deleted source remains eligible
for cleanup in either queue. This keeps old history from starving live updates. No OFFSET or COUNT sweep.
Historical batches yield for at least 25 ms when live work is idle. Empty queues back off to one second. Historical completion and pending live changes are distinct states.

## Worker lifecycle

Add `internal/task/conversationsearch` for the worker and narrow repository
interface. Wire its Start(ctx, store, log) owner and returned idempotent stop
function in `internal/backendapp`, using
`task_usage_wiring.go` as the lifecycle precedent. Start after schema readiness.
Stop and join before database close and through the storage-restore quiescence
path. No detached goroutines or new event-bus delivery dependency.

The worker yields after each batch. Writer admission waits and batch duration
are measured separately. Retry transient failures with bounded backoff.
Capability absence pauses this worker and never activates a scan fallback.
Structural index corruption reports unavailable and requires a controlled
projection rebuild from canonical data. Rebuild resets the checkpoint and
keeps dirty triggers active. Never erase source messages to repair an index.

## Authorization and lifecycle

Join candidate projections to current messages, sessions, and tasks in the
same search snapshot. Apply workspace, archive, and the palette's existing
ephemeral/configuration/automation visibility predicates before candidate
limits. Share the predicates, not a copied SQL approximation. Existing
workspace and task read authorization remains authoritative.

Do not cache workspace ownership in the index. This makes transfer visibility
follow the canonical task transaction without a full reindex. An absent
message/session/task excludes its projection immediately. Dirty rows exclude
changed text immediately. Delete stale projection text and its postings during
worker cleanup. Task/session hard deletion must also synchronously cascade
projection deletion, so removal does not leave durable transcript duplicates.
No query can bypass these live joins by reading the FTS table directly.

The canonical DELETE trigger removes its projection synchronously after
upserting the dirty ID. Session/task/turn cascades must invoke that trigger.
Do not add source foreign keys to the projection or dirty queue: a worker
projection insert must not acquire a source-row lock behind its dirty-row lock.
Use fresh tokens even when an ID is deleted and recreated, preventing ABA
acceptance of old work. SQLite can use randomblob and PostgreSQL can use its
built-in gen_random_uuid for this opaque token. Canonical message retention
and rollback remain the deletion authority. Backups include the projection as ordinary DB
data, while restore resumes its persisted checkpoint and dirty queue.

## Query budgets and result composition

Run read-only searches with a 750 ms total budget, including admission. Allow
one conversation query per workspace and four instance-wide. Waiting requests
remain cancellable. Use the existing read pool. An expired request releases
its connection and returns limited/unavailable state within the one-second
budget. Prove actual driver cancellation with a subsequent connection probe.

Fetch at most 201 visible matching candidates (200 plus a sentinel). Active
candidates take the budget before archived candidates. Apply live joins and
scope filters before this limit. Materialize the bounded window, then sort it
by archive group, message recency, and message ID for ties. Recency ranks this
window; it does not require a global sort of every matching message. Deduplicate
by task and stop at the requested task limit.
If 200 candidates cover too few tasks, return `limited` and a refine-query
hint instead of continuing an unbounded scan. Do not report an empty completed
search when the candidate budget or deadline prevented completion.

Query plans must begin with the text index for a selective query. A LIMIT after
a full message scan is not acceptable. A global common-term sort exceeded the deadline in the initial 100k-message
measurement; bounded materialization removes that sort. The benchmark must measure that class independently.
Do not turn off PostgreSQL sequential scans globally to conceal a poor plan.

The palette starts metadata and conversation requests independently after the
same 300 ms debounce. Keep metadata results visible if conversation work fails.
Reuse current metadata/PR search and archived fallback. Merge by task ID in
this order: active metadata, active conversation, archived metadata, archived
conversation. Fetch up to the scope limit from each source before deduplication.
Metadata rows retain their existing order and navigation. A duplicate remains
one metadata row. Conversation-only rows carry their snippet and session ID.
No cross-query or cross-workspace cache is added.

Abort both requests and fence results by query, workspace, scope, and open-state
generation. Preserve a user's explicit keyboard selection when a later source
arrives. Do not select a different task solely because conversation results
completed. An index-ready transition may trigger one debounced refresh. Avoid
continuous polling from a closed palette.

## Presentation

Desktop retains the existing command palette, scope tabs, and keyboard flow.
Conversation-only rows add a secondary two-line excerpt and an author label.
Keep title and archive cues primary. Truncate context text, not the match.
Selecting a conversation row uses `linkToTask(task.id, { sessionId })` through
`useCommandPanelTaskNavigation`, preserving sidebar reveal. If the session was
removed after the result loaded, show a localized stale-result notice and
refresh. Do not substitute another task or start an execution.

Phone adds a labelled Search tasks action to `app-nav-sheet.tsx` that closes
the menu and opens the existing Tasks scope. The palette remains the shared
focused search surface. Use the inset `Drawer` treatment from
`session-task-switcher-sheet.tsx` for the phone shell in
`command-panel-dialog.tsx`, while desktop retains CommandDialog. Share the
same Command content, hook state, selection, and navigation. Preserve the
existing MobileConfirmationHost for command actions.

The phone header and dismissal stay fixed above one scrolling result list.
Use dynamic viewport height, bottom safe-area padding, and a 16px input on
coarse pointers. Keep title/state above the excerpt in one column. All phone
result rows and scope controls have 44px targets. Do not enlarge desktop
controls. Opening a match navigates directly to its session. No preview pane
or nested overlay is needed. Test dismissal and focus return as well as search.

Keep selection static and obvious. Do not animate arriving results or reorder
rows through motion. Reuse semantic colors, Tabler icons, and existing radius
and border tokens. All new state/author/help copy uses i18next in seven
languages plus pseudo. Use `i18n:zh-hant` for Traditional Chinese.

## Performance evidence and rollout

Before exposing results, measure SQLite and PostgreSQL on disposable corpora
at 100k and 1m messages. Record hardware, database versions, query plans, index
bytes, build throughput, p50/p95 by query class, allocations, cancellation
latency, write overhead, and peak memory for a 1-MiB message. Compare identical
streaming workloads with indexing enabled and disabled. Treat thresholds as
delivery gates, not claims that the design already meets them.

The durable checkpoint and dirty queue drive the indexing disclosure. Startup
logs fixed capability, extension-permission, connection, or generic setup
failure reasons. Setup warnings suppress lifecycle cancellation and exclude raw
database error details. Runtime indexing failures log one fixed stage reason at most once per minute across the worker. Error details, message text, and record identifiers are excluded. Cancellation does not produce a warning. Performance tests record query and batch
costs. Dedicated production metric counters are deferred; query text, snippets,
message/task IDs, and workspace IDs must never enter generic logs or metric labels.

No external search service or new public configuration switch is required.
Database capability setup and restart recovery are documented with delivery.
If benchmark or dialect compatibility fails, revise this draft before shipping
rather than enabling a transcript-scan fallback.

## Alternatives and references

A workspace-wide LIKE scan grows with history and defeats the requested cost
bound. Browser-side transcript search transfers private history unnecessarily.
Native word indexes change substring and multilingual matching. A separate
search server adds deployment and access-control boundaries. Native trigram
indexes keep this feature with its existing database, with PostgreSQL extension
availability and index storage as explicit costs. This feature-local choice
is recorded here rather than establishing a new repository-wide search ADR.

- [SQLite FTS5 trigram and external-content rules](https://www.sqlite.org/fts5.html#the_trigram_tokenizer)
- [PostgreSQL trigram index support](https://www.postgresql.org/docs/current/pgtrgm.html#PGTRGM-INDEX)
- [Requirements](../requirements/conversation-search.md)
- [Plan and work orders](../../../plans/task-conversation-search/plan.md)
- [Workspace metadata search](workspace-search.md)
- [Archived result presentation](command-panel-archived-task-results.md)
- [Replayable migrations](../../../decisions/0027-replayable-schema-migrations.md)
