---
status: active
system: tasks
created: 2026-09-02
owners:
  - kandev
---

# Task plan content size limit Requirements

## Overview

`task_plans.content` has no write-side size ceiling. Every write path replaces the
whole document, and none of them bounds what it accepts. The only backstops are
the WebSocket gateway's 32 MiB per-message read limit and SQLite's ~1 GB string
limit. The existing shrinkage guard flags a write that *drops* content; nothing
flags a write that *grows* it, and nothing caps it outright.

Stored plan content is not inert. It is read back and prepended verbatim to the
prompt on every session handover for that task. An agent, possibly steered by
external text it was asked to summarize, can grow a plan without limit through
`update_task_plan_kandev`, and every subsequent launch on that task pays the
resulting cost again against a shared backend process.

This capability admits new plan content only below a fixed byte ceiling, at the
one seam every write path shares. The `tasks` system owns it because the durable
artifact is the plan, not the MCP or browser surface that submits it.

## Terminology

- **Plan content:** the value stored in `task_plans.content` and copied into each
  `task_plan_revisions.content` row.
- **Plan write:** an operation that admits caller-supplied content and upserts the
  plan HEAD row. Today: create and update, on both the agent and browser paths.
- **Plan revert:** the operation that restores a stored revision's content to HEAD.
  It admits no caller-supplied content and is not a plan write for this document.
- **Agent write path:** the MCP tools `create_task_plan_kandev` and
  `update_task_plan_kandev`.
- **Browser write path:** the WebSocket actions `task.plan.create` and
  `task.plan.update`, driven by the plan panel's editor and its autosave.
- **Content ceiling:** the maximum byte length of plan content a plan write admits.
- **Shared write seam:** the single component both write paths call to perform a
  plan write, below the per-surface request handling.

## Prior art

**Wiki leg — DID NOT RUN, tool unreachable.** The wiki retrieval tools were not
available in this environment. No wiki content was consulted.

**Vendor leg — DID NOT RUN, tool unreachable.** The `saas-kb` MCP server and its
`search_fsm_docs` tool are not registered in this session, so the `ai_sdlc`
category was not queried. No vendor claims informed this document.

**In-repo prior art — searched `docs/decisions/` and `docs/specs/` for byte
ceilings and `internal/` for `MaxBytesReader` call sites.** Kandev already has a
settled convention for this exact decision, and it is followed here rather than
re-derived. `maxUserStateBodyBytes` (256 KiB) bounds per-user plugin storage on
the stated reasoning that "without a cap, an authenticated-but-arbitrary browser
write could exhaust backend memory", sized to "comfortably cover a rich-text
scratchpad document while bounding worst-case memory per request". A plan is the
same shape of artifact under the same threat, so this capability reuses the same
ceiling. `REQ-TASKS-TITLE-LENGTH-LIMIT-001` supplies the other half of the
pattern: one limit enforced at a shared seam that no surface can skip, existing
oversized rows left readable and unrewritten, and the value deliberately not
configurable.

**What is different here.** Those precedents cap an HTTP request body at the
transport edge. The plan write paths are WebSocket actions with no per-action body
limit, and there are two of them, so the ceiling is a service-level rule about
admitted content rather than a transport rule about request size. The rejection
also has an agent audience, so it carries an instruction not to retry unchanged or
reconstruct from memory, following the reasoning already recorded on
`planTruncationWarning`.

**Measured input.** Against the local instance database (194 plan HEAD rows, 2602
revision rows) `length(content)` in bytes is: p50 14,468; p90 72,967; p99 119,852;
maximum 122,215 for a HEAD row and 158,393 for a revision. Three HEAD rows exceed
100,000 bytes; none exceeds 200,000. The ceiling below therefore rejects nothing
that has ever legitimately been stored on that instance.

## Requirements

### REQ-TASKS-PLAN-CONTENT-SIZE-LIMIT-001: New plan content is admitted only below a fixed byte ceiling

**Intent:** Bound the plan a single write can create, so that no caller can grow a
task's stored content without limit and impose a repeated cost on every later
session launch for that task.

**User story:** As an operator of a shared Kandev backend, I want a hard ceiling on
what any plan write can store, so that one task's plan cannot grow until it
degrades every session launched against it.

#### Acceptance criteria

- **AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-001.1:** The system shall reject a plan write
  whose submitted plan content exceeds 262,144 bytes, measured as the UTF-8 byte
  length of the content, and shall persist no part of that content.
- **AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-001.2:** When submitted plan content is
  exactly 262,144 bytes, the system shall admit the write. When it is 262,145
  bytes, the system shall reject it.
- **AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-001.3:** The system shall apply the ceiling at
  the shared write seam that both the agent write path and the browser write path
  call, so that a write path cannot reach storage without the ceiling being
  evaluated, and shall apply the identical ceiling to every write path.
- **AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-001.4:** When the system rejects a plan write
  for exceeding the ceiling, it shall leave the plan HEAD row unchanged, append no
  revision, coalesce into no revision, and publish no plan-created, plan-updated,
  or revision-created event.
- **AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-001.5:** The system shall check that a task
  identifier was supplied and authorize the caller before returning a size-limit
  response. For an oversized write, it shall establish that the target task
  exists before returning the size-limit response, so a missing or inaccessible
  task returns `not_found` without storage-constraint details. After these checks,
  the ceiling shall be evaluated before reading plan storage or acquiring that
  task's write lock. A rejected write shall not read the plan or wait behind
  another write for the same task.

  This preserves `AC-TASKS-DOCUMENTS-001.2`: a plan write for a missing task
  returns `not_found`, creates no plan data, and exposes no storage-constraint
  details. The existence check above is not a plan-row read and does not change
  the normal foreign-key race handling in the write transaction.
- **AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-001.6:** The system shall decide the ceiling
  from the submitted content alone. When two plan writes for the same task are in
  flight, the outcome of the ceiling check for each shall be independent of their
  interleaving and of the stored content either one replaces. The ceiling shall
  apply to each write individually and shall not accumulate across writes or
  across a task's revision history.
- **AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-001.7:** When a caller resubmits content that
  was rejected for exceeding the ceiling, the backend shall evaluate it as a new
  write and reject it identically. The backend shall retain no per-caller or
  per-task admission state from the earlier rejection. A client may keep local
  UI state to avoid retrying unchanged content.
- **AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-001.8:** When submitted plan content is at or
  below the ceiling, including empty content where the write path already accepts
  it, the system shall handle the write exactly as it does today, with unchanged
  coalescing, revision-append, truncation-warning, event, and response behavior.
- **AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-001.9:** The system shall not apply the
  ceiling to a plan revert. A revert whose target revision exceeds the ceiling
  shall succeed and restore that revision's content to HEAD.
- **AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-001.10:** The system shall leave plan content
  stored before this ceiling existed unchanged. Content above the ceiling shall
  remain readable, listable in revision history, revertable, and available to
  session handover. The system shall not rewrite, truncate, or migrate it, and
  shall not fail a read on its size.
- **AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-001.11:** The ceiling shall be a fixed value
  compiled into the backend, identical across every deployment and write path, and
  shall not be readable or writable from configuration, environment, or a runtime
  feature toggle.

### REQ-TASKS-PLAN-CONTENT-SIZE-LIMIT-002: An agent can act on the rejection without losing the plan

**Intent:** An agent that is told only "rejected" has two bad moves available:
resubmit the same document until it gives up, or rebuild a shorter plan from
memory and destroy the stored one on the next write. The rejection has to close
both, the way the existing truncation warning closes the equivalent move.

**User story:** As an agent whose plan write was refused for size, I want to be
told the limit, my size, and that nothing was stored, so that I compact the
document I am holding instead of retrying it or rewriting it from memory.

#### Acceptance criteria

- **AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-002.1:** When `create_task_plan_kandev` or
  `update_task_plan_kandev` submits content above the ceiling, the tool call shall
  return an error result rather than a success acknowledgement, and that result
  shall state the byte ceiling and the submitted byte size.
- **AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-002.2:** The error result shall state that no
  content was stored and that the task's existing plan is unchanged.
- **AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-002.3:** The error result shall direct the
  caller to shorten the document it is holding before writing again. It shall not
  instruct the caller to retry the same content, and it shall not suggest
  reconstructing the plan from memory.
- **AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-002.4:** The system shall report the rejection
  as a validation failure rather than an internal or transient failure, so that a
  caller distinguishing retryable from non-retryable errors does not retry it.
- **AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-002.5:** The registered descriptions of
  `create_task_plan_kandev` and `update_task_plan_kandev` shall state the content
  byte ceiling, so that a caller can size its document before its first write.

### REQ-TASKS-PLAN-CONTENT-SIZE-LIMIT-003: A user sees the rejection and keeps their draft

**Intent:** The plan panel autosaves on a timer and today discards the save error
without rendering it. A ceiling added without a visible failure state would let a
user keep typing into an editor that silently stopped persisting, and lose the
work on reload. That is a worse outcome than the growth this capability prevents.
The inverse is a real hazard too: the panel must report a rejected write only when
a write was actually rejected, because a failure signal attached to the wrong
operation would tell a user to shorten a document that is stored perfectly well.

**User story:** As a user editing a plan in the browser, I want a rejected save to
be visible and my text to stay in the editor, so that I can shorten it rather than
discover the loss after a reload.

#### Acceptance criteria

- **AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-003.1:** When a browser plan write is rejected
  for exceeding the ceiling, the plan panel shall display an error naming the byte
  ceiling and the byte size of the content that rejected write submitted. Both
  numbers shall be taken from the rejection itself rather than re-derived from the
  editor, so that editing the draft while a write is in flight can neither change
  either number nor suppress the error.
- **AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-003.2:** After such a rejection, the editor
  shall still contain the user's draft content. The system shall not clear it,
  revert it to the stored plan, or replace it with the stored plan.
- **AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-003.3:** After such a rejection, the panel
  shall not present the plan as saved and shall continue to report unsaved changes.
- **AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-003.4:** After such a rejection, the system
  shall not resubmit the same draft content. It shall attempt another write only
  when the draft changes or the user explicitly requests a save.
- **AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-003.5:** The system shall clear a displayed
  size rejection when the next save attempt begins, rather than when that attempt
  completes. When the draft submitted by that attempt is at or below the ceiling,
  the system shall not display a size rejection for it; if that attempt fails for
  an unrelated reason, the system shall display that failure instead of the size
  rejection, whatever the draft's size at the moment that failure arrives. Between
  one attempt failing and the next attempt beginning, editing the draft shall not
  change which failure is displayed. When the draft is at or below the ceiling and
  the write is otherwise valid and reaches the backend, the write shall be admitted.
- **AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-003.6:** The displayed error shall be
  localized copy in every locale Kandev ships, not a raw backend message.
- **AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-003.7:** The panel shall present a plan-write
  rejection only when a plan write was actually attempted and failed. The failure
  of an operation that is not a plan write, including loading the plan, loading
  revision history, reverting to a revision, and deleting the plan, shall not
  render the plan-write rejection, whatever the current draft's size. In
  particular, a task whose stored plan already exceeds the ceiling under
  `AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-001.10` shall not report a rejected write on
  opening the panel.
- **AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-003.8:** A displayed plan-write rejection shall
  belong to the task it was produced for and to the most recently started write
  attempt. When the panel changes to a different task, a rejection displayed for the
  previous task shall stop being displayed, and a write for the previous task that
  fails after the change shall not be displayed at all. When two writes for the same
  task are in flight, the outcome of the one that started earlier shall not replace
  what is displayed for the one that started later, whichever of the two completes
  first.

## Out of scope

Each exclusion below is a decision, not an omission.

- **A read-side or injection-time budget.** Bounding what session handover reads
  from a plan is a separate capability. A plan stored above this ceiling before it
  existed is still read and injected in full; this document deliberately does not
  change that, and closing it requires the read-side capability, not this one.
- **A ceiling on the plan title.** `task_plans.title` is also unbounded. It is not
  injected into prompts and is not scanned per launch, so it is not the same
  resource class, and capping it here would widen a write-boundary change into an
  unrelated field. It remains a plausible follow-up.
- **Total revision-history size, and revision pruning or retention.** The ceiling
  is per write. A task can accumulate many at-ceiling revisions; history is stored
  but not injected, so it is not the amplification this capability addresses.
- **Making the ceiling configurable.** A per-install ceiling would make the agent
  guidance in `REQ-TASKS-PLAN-CONTENT-SIZE-LIMIT-002` install-dependent and give
  an operator a way to disable the bound. It stays a compiled constant, matching
  the precedent in `REQ-TASKS-TITLE-LENGTH-LIMIT-001`.
- **Pre-emptive blocking or a live size indicator in the editor.** Stopping the
  user at the ceiling as they type, or showing a running byte count, is a
  usability improvement on top of the failure state required by
  `REQ-TASKS-PLAN-CONTENT-SIZE-LIMIT-003` and is not required for correctness.
- **Transport-level limits.** The WebSocket gateway's 32 MiB per-message read
  limit is unchanged. This ceiling is about content admitted for storage, not
  about the size of a frame.
- **Migrating existing oversized plans.** No backfill, rewrite, or reporting job.
  `AC-TASKS-PLAN-CONTENT-SIZE-LIMIT-001.10` keeps them intact and usable.
