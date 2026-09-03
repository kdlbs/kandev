---
id: "05-bounded-history-storage"
title: "Bound history hydration and operational payload storage"
status: done
wave: 2
depends_on: ["01-canonical-watch-migration"]
plan: "plan.md"
spec: "../../specs/platform/pr-watch-and-storage-bounds.md"
---

# Task 05: Bound history hydration and operational payload storage

## Intent

Keep normal history responsive while retaining conversations and making large operational data deduplicable and retention-eligible.

## Acceptance

- Every normal session-history hydration path uses cursor-bounded reads and does not eagerly materialize large tool metadata.
- Large tool payloads use digest-backed compressed/external storage; explicit authorized detail loading verifies integrity and ordinary user/agent messages remain retained.
- Equivalent Git snapshots deduplicate, while superseded payload/snapshot/plan-revision selection respects configurable retention and is non-destructive until maintenance executes.

## Files likely touched

- apps/backend/internal/task/models/message.go
- apps/backend/internal/task/repository/sqlite/message.go
- apps/backend/internal/task/repository/sqlite/git_snapshots.go
- apps/backend/internal/task/repository/sqlite/plan.go
- apps/backend/internal/task/repository/sqlite/base_schema.go
- apps/backend/internal/task/service/service_messages.go
- Relevant task controller/DTO hydration callers and focused repository/service tests

## Dependencies

Task 01 for shared migration/snapshot conventions.

## Parallelism

Sequential. It establishes the retention data contract consumed by the maintenance command.

## Verification

Build a multi-gigabyte-equivalent fixture through bounded blobs rather than committing a giant test database:

~~~bash
cd apps/backend && go test ./internal/task/repository/sqlite ./internal/task/service -run 'Test.*(Message|History|Payload|Snapshot|Retention|PlanRevision).*' -count=1 -v
cd apps/backend && go test ./internal/task/repository/sqlite ./internal/task/service -count=1
git diff --check
~~~

## Output contract

Report page limits, query/payload assertions, digest collision/dedup behavior, retention candidates, message-preservation evidence, and test outcomes. Update task and plan status.

## Results

**Bounded default page (cursor reads).** `service.DefaultMessagesPageSize` moved
10 → 50 (matching AC10's "bounded first page of 50"). `parseListMessageParams`
(HTTP) and `wsListMessages` (WS) now default to that page size whenever a
caller supplies *zero* pagination params (no `limit`/`before`/`after`/`sort`);
any explicit param (including bare `sort`) still runs the existing paginated
path unchanged. The shared service-layer `ListMessagesPaginated` default-limit
guard was deliberately left untouched — `apps/web/hooks/use-summarize-session.ts`
relies on `sort=asc` with no limit to fetch a session's *entire* history for
summarization, so bounding at the service layer would have silently broken
that caller for sessions over 50 messages. `fetchMessages` no longer has an
unbounded branch at all. Normal list reads (`ListMessages`,
`ListMessagesPaginated`, etc.) were not changed to add payload columns, so
paging never selects/decompresses large blobs.

**Digest-backed externalized payloads.** `task_session_messages` gained
`payload_digest`/`payload_size` (migration `migrateMessagePayloadStorage`,
`base_migrations.go`), and a new content-addressed `task_message_payloads`
table (`digest TEXT PRIMARY KEY, compressed_content <BlobType>,
uncompressed_size, compressed_size, created_at`; `dialect.BlobType` keeps the
column type Postgres-portable per the `dynamic_installation_keys` precedent).
`externalizeMessagePayload` (new file, `message_payload.go`) is the sole write
path for `metadata`: only a shell command's combined stdout+stderr over 4096
bytes (`largeMessagePayloadThresholdBytes`) is gzip-compressed, SHA-256
digested, and upserted (`ON CONFLICT (digest) DO NOTHING`, so byte-identical
output from different messages or retries is naturally deduplicated and never
errors) before the persisted `metadata` is replaced with the same small
`has_output`/`stdout_bytes`/`stderr_bytes`/`truncated`/`exit_code` summary
`ProjectMessageMetadata` already computes for client responses. Every other
metadata key (`tool_call_id`, `pending_id`, `status`, `agent_disconnected`,
...), read directly via SQL `json_extract` elsewhere, is left untouched — the
externalization is scoped to exactly the
`metadata.normalized.shell_exec.output` nested field. `CreateMessage`,
`insertMessageRow` (shared by the user-message boundary path), and
`UpdateMessage` all route through it; `GetMessage` — the only read path used
by the lazy shell-output detail route — now also selects
`payload_digest`/`payload_size`. `RehydrateMessagePayload` is the explicit,
authorized detail-load inverse: it loads the compressed row, decompresses,
**recomputes and verifies the SHA-256 digest before trusting the content**,
then merges the restored stdout/stderr back into `message.Metadata` via the
new `models.RehydrateShellOutput`. `Service.RehydrateMessagePayload` is a thin
passthrough (no extra authorization — callers already went through
`GetMessage`'s session-scoped check). `httpGetShellOutput` now rehydrates
whenever `message.PayloadDigest != ""` before extracting shell output, so a
large output that was externalized at write time is no longer invisible to
that endpoint. Ordinary user/agent message content (never routed through
`externalizeMessagePayload`'s shell-output branch) and small tool metadata
(the common case) are stored inline exactly as before — nothing is dropped or
truncated for either.

**Git snapshot content dedup (read-only retention candidates).**
`task_session_git_snapshots` gained `content_digest` (migration
`migrateGitSnapshotContentDigest` + a Go-side backfill for historical rows,
since neither SQLite nor Postgres has a portable SHA-256 SQL function).
`computeGitSnapshotDigest` hashes only the repository-state fields (branch,
remote_branch, head_commit, base_commit, ahead, behind, files) — deliberately
excluding `session_id`/`snapshot_type`/`triggered_by`/`created_at`, which
describe *why* and *for whom* a snapshot was captured, not what it contains.
`CreateGitSnapshot` and `UpsertLatestLiveGitSnapshot` both compute and persist
it on every write. Per the plan's "destructive maintenance is explicit,
dry-run-first" constraint, this wave does **not** skip a write when the
computed digest matches the immediately preceding row (an initial write-time
skip design was tried and reverted after it silently broke two pre-existing
ordering tests — `TestGetLatestGitSnapshotFallsBackToNewestWithoutAgentCompleted`
and `TestGetGitSnapshotsBySessionOrdersDescendingAndHonoursLimit` — that
depend on every historical poll being recorded). Instead,
`ListDuplicateGitSnapshotCandidates` (new, read-only) reports every row in a
`(session_id, content_digest)` group except the newest (matching
`snapshotRankExpr`'s tie-break), for a later, explicit maintenance pass
(Task 06) to prune. A dangling-session-reference "orphan" query was
considered and dropped: `task_session_git_snapshots.session_id` has
`FOREIGN KEY ... REFERENCES task_sessions(id) ON DELETE CASCADE`, so that
condition is schema-guaranteed impossible to reach (confirmed by a test that
tried to insert one and hit the FK constraint).

**Plan-revision retention candidates (read-only).**
`ListObsoletePlanRevisionCandidates(ctx, taskID, keepLastN)` (new, in
`plan.go`) reports superseded revisions: it always excludes the task's
current HEAD (`MAX(revision_number)`) and any revision some other revision's
`revert_of_revision_id` points to (so the "restore an earlier plan" ancestry
chain is never broken), and additionally excludes the most recent `keepLastN`
non-HEAD revisions when `keepLastN > 0`. The revision *doing* the revert is
not itself protected by ancestry (only the revision it points *to* is) — its
content already duplicates the protected target, so losing it doesn't lose
information. Purely a `SELECT`; a dedicated non-destructiveness test asserts
row count is unchanged after calling it.

**Message/conversation preservation.** No conversational message row, and no
message's human/agent-authored `content`, was touched by any change in this
wave — only the `metadata` JSON of shell-tool-call messages over the
externalization threshold is ever rewritten (to the summary shape), and the
original bytes remain fully recoverable via `RehydrateMessagePayload`'s
verified round trip.

**Tests** (`internal/task/repository/sqlite`, `internal/task/service`,
`internal/task/handlers`; all new tests use `newRepoForSessionTests`/
`newRepoForEntityTests` against the real migrated SQLite schema, not mocks):
- `message_payload_test.go`: small metadata stays inline
  (`TestCreateMessageLeavesSmallMetadataInline`); large output is
  externalized then correctly rehydrated end-to-end
  (`TestCreateMessageExternalizesLargeShellOutputAndRehydrates`); identical
  large payloads across two messages dedupe to one
  `task_message_payloads` row
  (`TestExternalizeMessagePayloadDedupesIdenticalContentAcrossMessages`);
  tampering with stored bytes is rejected by the integrity check
  (`TestRehydrateMessagePayloadRejectsTamperedContent`).
- `git_snapshot_digest_test.go`: digest computed and always recorded per
  poll (`TestCreateGitSnapshotSetsContentDigestAndAlwaysRecords`), upsert
  path sets the digest too (`TestUpsertLatestLiveGitSnapshotSetsContentDigest`),
  duplicate-candidate selection keeps only the newest of a group
  (`TestListDuplicateGitSnapshotCandidatesKeepsNewestPerGroup`) and is
  read-only (`TestListDuplicateGitSnapshotCandidatesIsNonDestructive`).
- `plan_retention_test.go`: HEAD + revert-ancestry protection
  (`TestListObsoletePlanRevisionCandidatesProtectsHeadAndAncestry`),
  `keepLastN` recency window
  (`TestListObsoletePlanRevisionCandidatesRespectsRecencyWindow`), and
  non-destructiveness
  (`TestListObsoletePlanRevisionCandidatesIsNonDestructive`).
- `message_list_handlers_test.go`: `TestHTTPListMessagesDefaultsToABoundedFirstPage`,
  `TestWSListMessagesDefaultsToABoundedFirstPage` (both assert the paginated
  path runs with `limit == service.DefaultMessagesPageSize` when no params
  are given); pre-existing `TestHTTPShellOutputSnapshot` /
  `TestHTTPGetShellOutputDeniesForeignMessageWith404` continue to pass
  unmodified, confirming no regression to the lazy shell-output route.

**Verification run:**
~~~text
$ go test ./internal/task/repository/sqlite ./internal/task/service \
    -run 'Test.*(Message|History|Payload|Snapshot|Retention|PlanRevision).*' -count=1 -v
ok  	github.com/kandev/kandev/internal/task/repository/sqlite
ok  	github.com/kandev/kandev/internal/task/service

$ go test ./internal/task/repository/sqlite ./internal/task/service -count=1
ok  	github.com/kandev/kandev/internal/task/repository/sqlite
ok  	github.com/kandev/kandev/internal/task/service

$ go test ./internal/task/repository/sqlite ./internal/task/service \
    -run 'Test.*(Message|History|Payload|Snapshot|Retention|PlanRevision).*' -count=1 -race
ok  	github.com/kandev/kandev/internal/task/repository/sqlite
ok  	github.com/kandev/kandev/internal/task/service

$ go build ./...   # apps/backend, clean
$ go vet ./...     # apps/backend, clean
$ golangci-lint run ./internal/task/repository/sqlite/... ./internal/task/service/... \
    ./internal/task/handlers/... ./internal/task/models/... ./internal/task/repository/...
0 issues.
$ gofmt -l <touched .go files>   # empty
$ git diff --cached --check      # empty
~~~

Two pre-existing failures observed in `internal/task/service`/
`internal/task/handlers` full-package runs
(`TestServiceInitializeLocalRepositoryCreatesMainRepository` and siblings,
"parent directory cannot be accessed") are environmental — reproduced
identically with this wave's changes fully stashed, so unrelated to this
task.

**Deferred to Task 06** (the maintenance command, explicitly out of scope
here): actually acting on `ListDuplicateGitSnapshotCandidates` /
`ListObsoletePlanRevisionCandidates` / stale `task_message_payloads` rows
(dry-run-first, backup-gated, offline/exclusive per the plan's constraints),
and the multi-gigabyte-equivalent end-to-end pagination-cost fixture (AC11)
belongs with the HTTP/WS bounded-read surface once the maintenance command
exists to generate large fixtures safely.
