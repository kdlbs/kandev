---
created: 2026-09-04
status: completed
requirements:
  - REQ-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001
  - REQ-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001
system_design:
  - ../../specs/ui/system-design/message-queue-automation-controls.md
legacy_specs:
  - ../../specs/ui/requirements/message-queue-run.md
  - ../../specs/ui/requirements/message-queue-auto-merge.md
---

# Implementation Plan: Message Queue Session Automation

## Overview

Add the per-session Auto-merge policy before exposing it in the queue panel, so
every rendered switch is backed by authoritative persistence and event
projection. Then recompose Auto-run and Auto-merge as compact header pills,
prove the same workflow on desktop and mobile, and reconcile public queue and
WebSocket documentation.

## Scope

### In scope

- Preserve current install-wide automatic-merge configuration as the fallback.
- Persist an explicit automatic-merge override only after a session switch is
  changed.
- Resolve untouched sessions against later global changes and keep overridden
  sessions independent.
- Project status epoch/generation and policy availability plus the effective
  value, source, and revision through queue reads and every session-scoped live
  status producer.
- Move Auto-run into a compact pill beside the queue summary and add the
  matching Auto-merge pill in the same responsive header region.
- Preserve queue actions, touch sizing, one scroll owner, and mobile capability.
- Define future eligible direct folding at or above the limit and
  staged-attachment rejection in the draft replacement contract. Keep active
  capacity requirements unchanged until verified implementation and lifecycle
  cutover.

### Out of scope

- A third Follow global state or reset-override action.
- Per-task or per-workspace automatic-merge policy.
- Retroactive compaction of pending rows.
- Changes to compatibility, content/entity combination, manual merge, Send Now,
  Auto-run delivery, capacity configuration, or workflow-coalescing semantics.
  Coalescing only gains the common session-identity guard. Automatic fold
  queued time remains the shipped later target/source value.
- Copying an override to a different session during workflow session transfer.

## Technical approach

### Backend policy and protocol

- Extend `queue_session_state` in
  `apps/backend/internal/orchestrator/messagequeue/repository_sqlite.go` with
  nullable `auto_merge_override` and monotonic `auto_merge_revision` columns in
  fresh schema and replay-safe migration paths.
- Extend `Repository` plus memory and SQL implementations with nullable
  override reads and atomic explicit writes that increment and return the
  session revision.
- Require `QueueSessionIdentity` on every service and repository operation that
  can add, append, replace, restore, reserve, acknowledge, consume, update,
  remove, merge, reorder, clear, transfer, or change session policy. Browser,
  plugin, MCP, clarification, workflow, retry, pending-move, drain, and Send Now
  paths migrate without textual-ID mutating overloads.
- Define required SQL and memory session-authority seams in `messagequeue`.
  Besides task locks and session validation, their guarded boundary claims
  typed staged attachments. Production injects the task repository; SQL
  fixtures seed task/session/attachment rows and memory fixtures seed a locked
  authority. Remove task-table probing, nil authority, and fail-open identity
  validation.
- Use one repository transaction helper: lock sorted unique task rows through
  the authority, then sorted repository-local and durable session locks, then
  validate exact identities. Admission resolves policy, inserts, claims staged
  attachments, and folds inside that boundary; claim failure rolls everything
  back. Override, pending-move, and destructive lifecycle mutations use the same
  order. Remove the post-commit after-insert callback and compensation delete.
- Transfer requires exact source and destination identities in one transaction,
  using sorted task rows then sorted session locks before validation. It removes
  source state without replacing destination policy. Deterministic
  two-connection tests cover both race orders with session/task/workspace
  delete/recreate and opposite-direction transfer.
- Add queue-owned session-state cleanup to destructive transactions. Preserve
  state on archive. A separate deletion-only post-commit callback carries
  authoritative identities; memory buckets CAS-purge only matching deleted
  incarnations, while the existing archive/delete purger and recount notifier
  stay separate.
- Add `pending_moves.session_incarnation_id`. Deferred apply CASes identity,
  move/source/target, increments task transition generation, and returns an
  immutable effect token. Post-commit task effects CAS generation/target;
  session/queue effects validate token identity/generation. Recreation or a
  successor move makes every stale effect fail closed.
- Add an immutable server-generated `queue_incarnation_id` to each task-session
  row and expose it with `TaskSession`. Register its replay-safe distinct UUID
  backfill after every existing task-session table rebuild, require future
  rebuilds to preserve it, and carry it as `session_incarnation_id` on every
  session-scoped queue status.
- Persist an internal global automatic-merge revision with the existing system
  message-queue settings value. Increment it only when the global boolean
  changes, commit both together, and keep the revision out of editable public
  settings responses.
- Replace the queue service's independent automatic boolean with one immutable
  value/revision snapshot published through an atomic pointer after settings
  commit. Hydrate the same pair from persisted queue settings during backend
  startup before admissions become available.
- Update `messagequeue.Service` admission and status paths to resolve the
  override against one loaded global snapshot. Use the repository-linearized
  effective snapshot for ordinary and eligible direct folds; fail closed on an
  override read error, preserve the shipped newest queued time, and reject
  staged-attachment admissions at or above the row limit without claiming an
  attachment.
- Add status ordering, availability, and effective policy to `QueueStatus`, plus
  `message.queue.auto_merge.set`. Every browser queue request, including get
  and all mutations, requires the task/session/incarnation triplet. Gateway and
  handler authorize the supplied pair and pass the triplet unchanged.
- Replace separate queue/session reads with
  `Snapshot(expected QueueSessionIdentity)`. Browser requests/mutations supply
  expected identity; internal work retains it. Persistence revalidates and
  snapshots under task-then-session locks. Responses/events use that identity
  and publish nothing on mismatch, including mutation-commit/recreation races.
- Tag reservations, `SendNowClaim`, queued dispatch/cancellation, deferred
  transition dispatch, and in-flight ownership with identity plus operation
  generation. Revalidate before cancel, prompt, restore, ack, and CAS cleanup
  without locks over agent I/O. Post-claim races cover manual drain, Auto-run,
  and both Send Now scopes.
- Include the identity, ordering, and available tuple or unavailable marker in
  get/mutation responses and every session-scoped producer.
- After session deletion, rely on repository-transaction cleanup and its
  deletion-only memory callback rather than post-delete `CancelAll`, then
  publish only an empty-session explicit task-scoped recount. Gateway routing
  retains that signal for internal status-summary projection but drops it
  before browser broadcast; invalid empty-session queue events are also
  dropped.
- Preserve current snapshot/restore semantics and Auto-run transfer behavior.

### Frontend state and controls

- Preserve backend JSON naming in frontend wire-derived types:
  `TaskSession.task_id`, `TaskSession.queue_incarnation_id`, and queue payload
  `session_incarnation_id`. Reject session-scoped queue payloads whose
  incarnation differs from the current `TaskSession`, and reset policy ordering
  when a matching new incarnation replaces stored metadata.
- Replace textual-session loading booleans with active operation tokens carrying
  session incarnation and a monotonically allocated client operation
  generation. All terminal paths compare-and-clear only the exact token while
  its incarnation is still current, so delayed completion cannot clear a
  replacement session's mutation.
- Clear queue metadata/loading on every session and task removal. For
  `workspace.deleted`, snapshot affected session IDs by joining normalized
  current `TaskSession.task_id` values to locally cached workspace tasks before
  removing indexes, then invoke the same session cleanup.
- Add status epoch/generation, policy availability, and the known Auto-merge
  value/source/revision to `QueueMeta` and queue read/event contracts, with
  preservation logic in every queue metadata replacement.
- Apply newest-status availability, process-epoch retirement, source precedence,
  and same-source policy revision comparison within one incarnation so failed
  resolution or a delayed pre-mutation event cannot disable, invent, or
  overwrite newer authoritative state.
- Require current task/session/incarnation in every queue request in
  `queue-api.ts`; `useQueue` supplies `TaskSession` identity, refetches after
  stale outcomes, and never retries. Raw E2E WebSocket helpers accept and emit
  the same complete triplet for get, add, and cancel.
- Update `QueueAffordance`, `QueuePanel`, and `QueuePanelHeader` wiring for the
  effective value and mutation.
- Replace the dedicated Auto-run row in
  `queued-ghost-panel-header.tsx` with one responsive automation region. Wide
  layouts keep the pills beside the summary on the first visual line; narrow
  layouts may wrap logical groups. Each compact bordered pill contains a
  localized visible label and shared `Switch`; Auto-run retains state-specific
  accessible help.
- Update the global Message Queue settings description and component test to
  explain inheritance until a per-session change.
- Add complete English, Portuguese, Simplified Chinese, and generated
  Traditional Chinese locale entries for the pills, mutation error, and global
  inheritance description.

### Browser coverage

- Extend `apps/web/e2e/tests/chat/message-queue.spec.ts` with a desktop flow that
  verifies both pills share the summary header, an untouched session follows a
  changed global value, a user change persists across reload, and a later
  global change no longer affects that session.
- Extend
  `apps/web/e2e/tests/chat/mobile-message-queue-management.spec.ts` with the
  same user value on `mobile-chrome`: both controls are reachable by touch,
  mutation persists, the wrapping header remains contained, and the document
  has no horizontal overflow.
- Use API setup only for global-setting preconditions; all session-override
  changes and assertions go through the rendered queue controls.
- Update `apps/web/e2e/helpers/api-client.ts` and every queue helper caller.
  Focused sidebar and workflow scenarios cover raw get/add/cancel triplets;
  typecheck enforces the clean helper-signature cutover across chat, task,
  workflow, and SSH suites.

### Public documentation

- Update `docs/public/coordination.md`, `docs/public/operations.md`,
  `docs/public/sessions-and-review.md`, `docs/public/configuration.md`, and
  `docs/public/websocket-api.md` where they describe queue Auto-run,
  install-wide automatic merging, full-capacity behavior, or queue protocol
  fields.
- After production and behavioral checks pass, activate the replacement
  requirements, deprecate and forward-link the unchanged superseded contracts,
  amend the active management capacity text to the delivered exceptions, mark
  the system design current, and update the UI specification indexes.

## Tests

- `AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.2`, `.3`, `.7`, `.8`, `.9`,
  `.11`, `.12`, `.13`, `.15`, `.16`, `.17`, `.18`, `.19`, `.21`, `.22`, `.23`,
  `.24`, and `.25` map to settings, startup, migrations, authority, claims,
  deferred transactions/effects, every request/mutator, async dispatch
  ownership, lifecycle races, expected-identity snapshots, and producer tests.
- `AC-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001.2`, `.7`, and `.8` plus
  `AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.4`, `.12`, `.13`, `.15`, `.16`,
  `.17`, `.18`, `.19`, `.20`, and `.24` map to queue API, hook,
  incarnation/generation operation tokens, slice, removal, connection epoch,
  WebSocket-handler, settings, and header tests under `apps/web`.
- Existing Auto-run, automatic-merge compatibility, manual merge,
  full-capacity, and Send Now tests remain regression coverage for unchanged
  behavior.

## E2E tests

- `apps/web/e2e/tests/chat/message-queue.spec.ts` covers
  `AC-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001.1`, `.2`, `.7` and
  `AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.2`, `.3`, `.4`, `.11`, `.12`,
  `.13` on Chromium.
- `apps/web/e2e/tests/chat/mobile-message-queue-management.spec.ts` covers
  `AC-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001.9` and
  `AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.14` on `mobile-chrome`.
- `apps/web/e2e/tests/task/sidebar-queued-count.spec.ts` covers complete raw
  queue get/add/cancel identities for
  `AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.19` and `.24`.

## Work orders

- [ ] [Task 01: Persist Session Auto-merge Overrides](task-01-persist-session-auto-merge-overrides.md)
- [ ] [Task 02: Deliver Queue Automation Pills](task-02-deliver-queue-automation-pills.md)
- [ ] [Task 03: Update Queue Automation Documentation](task-03-update-queue-automation-documentation.md)

## Verification results

Pending.

After all work-order checks pass, run the user-requested repository gates from
the repository root in this order:

```bash
make fmt
make typecheck test lint
```

## Risks

- Collapsing nullable inheritance into a boolean during persistence would freeze
  untouched sessions and violate later global changes.
- Missing one synthesized `QueueMeta` path could make Auto-merge visually reset
  after clear, reorder, reconnect, an unavailable status, or an older event.
- A compact desktop header can become an unusable compressed toolbar on mobile;
  the control group must wrap while preserving action visibility and one scroll
  owner.
- Queue Auto-merge terminology can be confused with pull-request Auto-merge;
  copy and documentation must keep the message-queue context explicit.
- Every browser read/mutator and internal operation carries expected identity.
  A textual-ID path lets delayed work target a replacement; compile-time
  signatures plus typed and raw-client stale-token tests enforce inventory.
- Queue repositories require injected SQL/memory authority. Typed arguments
  alone cannot validate current task-session rows inside the lock boundary.
- Transfer locks both task rows before sorted session locks and revalidates both
  endpoints; another order can deadlock deletion or move stale rows.
- Attachment claims execute through authority inside admission; a post-commit
  callback leaves a deletion/recreation gap.
- Deferred apply returns an immutable transition effect token. Task effects CAS
  generation/target and session effects validate identity/generation; tests
  race both recreation and successor moves after commit.
- Reusing archive's task purger for memory policy cleanup either resets archive
  state or leaks deleted overrides. A separate deletion-only callback carries
  authoritative incarnation pairs and CAS-purges matching memory buckets.
- Queue metadata keyed only by textual session ID can accept delayed policy
  from a deleted incarnation. Status must match the current immutable
  task-session incarnation, and every removal path must clear queue state.
- A status publisher that re-resolves after authorization or mutation commit
  can expose replacement state. Producers use
  `Snapshot(expected QueueSessionIdentity)` and publish nothing on mismatch.
- Untagged async claims let delayed drain, Auto-run, deferred dispatch, or Send
  Now work target or clear replacement ownership. Identity/generation guards
  survive claim through prompt, restore, acknowledgement, and cleanup.
- A raw helper signature change without migrating every caller leaves hidden
  WebSocket payloads stale. The work order inventories chat, task, workflow, and
  SSH callers; focused add/cancel/get E2E plus typecheck enforces the cutover.
- An incarnation migration placed before an existing `task_sessions` rebuild
  is silently dropped; it must run after all current rebuilds and future
  projections must retain it.
- Workspace deletion loses empty-queue metadata unless it derives affected
  sessions before removing task/workspace indexes.
- A delayed status result can regress or disable the new switch unless every
  session-scoped producer emits epoch/generation and
  availability/source/revision metadata and both refetch and event paths use
  the same comparisons.
- Independent runtime atomics for the global boolean and revision could expose
  a policy generation that never existed; all reads use one immutable snapshot
  hydrated from the persisted revision at startup.
- The global settings description becomes part of the precedence contract; all
  locales and its component test must change together.
- The replacement requirements must remain draft until Tasks 01 and 02 pass,
  then cut over atomically with deprecation links from the prior contracts.
- Task-only status-summary signals cannot carry session policy or reach browser
  routing; explicit scope, an empty session ID, early gateway consumption, and
  producer/gateway tests enforce the boundary.
- PostgreSQL verification must include both changed DSN-gated repository
  packages; running only message-queue tests misses task-session migration and
  lifecycle behavior.
