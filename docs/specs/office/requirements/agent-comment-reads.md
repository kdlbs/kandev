---
status: draft
system: office
created: 2026-08-25
owners:
  - Kandev
---

# Office: Agent Comment Reads Requirements

## Overview

Every Office role instruction file tells its agent to post task comments. No
Office agent can read one. The Office MCP surface registers exactly twelve tools
and none reads `task_comments`, so a producer stage writes its deliverable to a
channel no consumer stage can open, and multi-stage delegation cannot hand off.

This document adds one read tool to the Office MCP surface, making the existing
comment channel legible to the agents already instructed to write to it. Office
owns this contract because Office owns agent coordination; the comment store and
the cross-task access rule are consumed here, not redefined.

## Design decision

Two options were considered.

1. Expose comments to the Office MCP surface as a read tool.
2. Redirect the role instructions at `write_task_document_kandev`, treating
   comments as human-facing discussion only.

**Option 1 is chosen.** Six role instruction files (`ceo`, `worker`,
`reviewer`, `qa`, `security`, `devops`) already direct their agents to post
comments, so option 2 is not an instruction edit but a rewrite of every role,
after which that text becomes load-bearing for every future role author. The
write path already exists, is signed, and is task-scoped; only the read is
missing, and the access rule it needs already grants parent-reads-child. A
comment and a document are also not interchangeable: a document is a named
artifact replaced wholesale by key, so two stages publishing to a shared parent
collide on key choice, while a comment is an append-only attributed event, the
shape a progress note, a review finding, and a stage deliverable actually have.

## Prior art

**Our own prior reasoning.** Attempted, not skipped: the vault at
`/Users/henry/Documents/henry/wiki` returned `Operation not permitted` and the
`obsidian-wiki` CLI is not installed, so this leg returned nothing.

**What other products shipped.** Queried through the `saas-kb` `ai_sdlc` slice.

- *Warp orchestration.* Parent and child agents each own an inbox addressed by
  agent ID on a durable message bus; Warp is explicit that "agents don't read
  each other's transcripts or live working trees" and exchange messages instead.
  Its review swarm has each child post findings as a comment and exit while the
  parent fans results in, the same shape as the Office stage handoff. Warp also
  gives messages and run-state transitions a shared global sequence number "so
  the parent never observes a child's `SUCCEEDED` state before the message that
  produced the result".
- *Devin managed Devins.* A coordinator session scopes work and compiles
  results, and the Devin MCP server lets it "inspect any session's full event
  timeline" — a tool, not instruction text.
- *OpenHands.* Queried; only SDK visualizer and architecture hits, nothing on
  cross-agent deliverable handoff.

**What we are doing differently.** Not a message bus or mailbox: Office already
has a durable, per-task, append-only, attributed channel agents are instructed
to write to, so the gap is a missing reader, not a missing transport. We
deliberately do **not** adopt Warp's global sequence number: it solves
payload-visible-before-state ordering across a bus, and this is a read gap, not
a race. That is what makes the stable-arbitrary tiebreak in
REQ-OFFICE-AGENT-COMMENT-READS-003 acceptable, named again under "Out of scope".
Unlike Devin, the read is scoped by task relation, not blanket access to any
session a coordinator can name.

## Terminology

- **Caller task:** The task whose session invoked the tool, taken from the MCP
  server's bound task identity, never from tool arguments.
- **Target task:** The task whose comments are requested.
- **Read relation:** The existing cross-task read rule shared with the task
  document tools: target is the caller itself, an ancestor, descendant, sibling
  with a shared non-empty parent, or blocker of the caller, both in the same
  workspace.
- **Window:** The `total` / `returned` / `has_more` triple describing how much
  of a task's comment history a response covers.
- **Coordinator:** The `ceo` Office role, at
  `apps/backend/internal/office/configloader/instructions/ceo/AGENTS.md`, which
  creates subtasks and fans their results back in. No role directory is named
  `coordinator`; the six are `ceo`, `devops`, `qa`, `reviewer`, `security`,
  `worker`. The scheduled routine also called "coordinator" is a different
  concept.

## Requirements

### REQ-OFFICE-AGENT-COMMENT-READS-001: Read a related task's comments

**Intent:** Let a coordinating agent read the deliverable a child stage wrote,
so a fan-in completes without a human relaying the payload.

**User story:** As an Office coordinator agent, I want to read a child task's
comments, so I can act on a completed stage's written output.

#### Acceptance criteria

- **AC-OFFICE-AGENT-COMMENT-READS-001.1:** The Office MCP surface shall
  register a `list_task_comments_kandev` tool, and the kanban, configuration,
  external, and automation surfaces shall not register it.
- **AC-OFFICE-AGENT-COMMENT-READS-001.2:** When the caller task and the target
  task satisfy the read relation, the tool shall return the target's comments.
- **AC-OFFICE-AGENT-COMMENT-READS-001.3:** When the target task is a descendant
  of the caller task, the read relation shall be satisfied, so a parent can
  read a child's comments.
- **AC-OFFICE-AGENT-COMMENT-READS-001.4:** When the caller and target do not
  satisfy the read relation, the tool shall return an access-denied error and
  no comment content.
- **AC-OFFICE-AGENT-COMMENT-READS-001.5:** When the target task does not exist,
  the tool shall return the same access-denied error and the same message as an
  unrelated existing task, so a caller cannot distinguish the two.
- **AC-OFFICE-AGENT-COMMENT-READS-001.6:** When the caller and target are in
  different workspaces, the tool shall return an access-denied error even if a
  parent, child, sibling, or blocker edge exists between them.
- **AC-OFFICE-AGENT-COMMENT-READS-001.7:** The read relation applied by this
  tool shall be the same rule the task document read tools apply, evaluated by
  the same shared implementation, so the two surfaces cannot diverge.
- **AC-OFFICE-AGENT-COMMENT-READS-001.8:** The tool shall be read-only and
  shall provide no way to create, edit, or delete a comment.
- **AC-OFFICE-AGENT-COMMENT-READS-001.9:** The shared read-relation guard shall
  resolve a caller task or target task that does not exist to a plain denial,
  emitting no error that names the task or otherwise distinguishes a
  nonexistent task from an existing unrelated one, so that
  AC-OFFICE-AGENT-COMMENT-READS-001.5 holds at the guard rather than at any
  single caller.
- **AC-OFFICE-AGENT-COMMENT-READS-001.10:** The normalisation required by
  AC-OFFICE-AGENT-COMMENT-READS-001.9 shall take effect for every caller of the
  shared guard, including the existing task document read and write tools, so
  the two surfaces cannot answer the does-this-task-exist question
  differently.
- **AC-OFFICE-AGENT-COMMENT-READS-001.11:** Any in-memory task lookup used to
  test the shared guard shall return the same not-found result shape the task
  repository returns for an absent identifier, so a guard test cannot report
  success on a branch production never reaches.
- **AC-OFFICE-AGENT-COMMENT-READS-001.12:** The access-denied outcome shall be
  reported through the existing shared access-denied sentinel, whose message is
  the literal string `document access denied` and which maps to the existing
  forbidden error code. The tool shall not introduce a second access-denied
  sentinel or a comment-specific message, so the denial a caller sees is
  identical across the comment and document surfaces.

### REQ-OFFICE-AGENT-COMMENT-READS-002: Return every author's comments

**Intent:** A stage's context includes what its peers wrote and what a human
told it. Dropping either silently hides a decision.

#### Acceptance criteria

- **AC-OFFICE-AGENT-COMMENT-READS-002.1:** The tool shall return comments
  regardless of author, including agent-authored and human-authored comments.
- **AC-OFFICE-AGENT-COMMENT-READS-002.2:** Each returned comment shall carry
  its `author_type`, `author_id`, and `source` values as recorded.
- **AC-OFFICE-AGENT-COMMENT-READS-002.3:** The tool shall not accept an author,
  author-type, or source filter parameter, so no caller can receive a silently
  narrowed view of a task's history.
- **AC-OFFICE-AGENT-COMMENT-READS-002.4:** Each returned comment shall omit
  `reply_channel_id` and shall omit per-comment run lifecycle fields.

### REQ-OFFICE-AGENT-COMMENT-READS-003: Deterministic ordering and windowing

**Intent:** A coordinator re-reading a thread must see the same thread. An
under-determined sort makes a fan-in unrepeatable and a truncated read
indistinguishable from an empty one.

#### Acceptance criteria

- **AC-OFFICE-AGENT-COMMENT-READS-003.1:** The tool shall select the most
  recent `limit` comments for the target task, ordered by the `created_at`
  column descending and, where `created_at` values are equal, by the `id`
  column descending.
- **AC-OFFICE-AGENT-COMMENT-READS-003.2:** The tool shall return that selected
  window ordered by the `created_at` column ascending and, where `created_at`
  values are equal, by the `id` column ascending, so the caller reads the
  thread oldest-first.
- **AC-OFFICE-AGENT-COMMENT-READS-003.3:** The tiebreak column named in
  AC-OFFICE-AGENT-COMMENT-READS-003.1 and AC-OFFICE-AGENT-COMMENT-READS-003.2
  shall be the same column in both directions, so the window boundary selects
  the same comment set on every call.
- **AC-OFFICE-AGENT-COMMENT-READS-003.4:** When `limit` is omitted, null, zero,
  or negative, the tool shall apply a default limit of 20.
- **AC-OFFICE-AGENT-COMMENT-READS-003.5:** When `limit` exceeds 100, the tool
  shall clamp it to 100 and shall not return an error.
- **AC-OFFICE-AGENT-COMMENT-READS-003.6:** Every response shall carry a window
  reporting `total`, the target's comment count; `returned`, the number of
  comments in this response; and `has_more`, true exactly when `returned` is
  less than `total`.
- **AC-OFFICE-AGENT-COMMENT-READS-003.7:** The total count and the returned
  comments shall be read within a single read transaction, so the returned
  count never exceeds the reported total.
- **AC-OFFICE-AGENT-COMMENT-READS-003.8:** Two calls with identical arguments
  and no intervening comment write shall return byte-identical comment
  identifiers in identical order.
- **AC-OFFICE-AGENT-COMMENT-READS-003.9:** When `limit` is present but is not
  an integer, the tool shall apply the default limit of 20 rather than return
  an error.
- **AC-OFFICE-AGENT-COMMENT-READS-003.10:** The window field named in
  AC-OFFICE-AGENT-COMMENT-READS-003.6 shall describe only how many comments the
  response omits, and shall never report whether any body was shortened.
- **AC-OFFICE-AGENT-COMMENT-READS-003.11:** The `limit` argument shall be
  declared optional with no JSON Schema type constraint, so that argument
  validation cannot reject a value AC-OFFICE-AGENT-COMMENT-READS-003.4 or
  AC-OFFICE-AGENT-COMMENT-READS-003.9 requires to be defaulted. Every value
  shall reach the handler and be defaulted per those criteria, and the tool
  shall return no error for any `limit` value, from any layer.

### REQ-OFFICE-AGENT-COMMENT-READS-004: Bounded bodies

**Intent:** A comment body is unbounded text, and an unbounded read can exhaust
a consuming agent's context and lose the deliverable it was fetched for.

#### Acceptance criteria

- **AC-OFFICE-AGENT-COMMENT-READS-004.1:** Each returned comment body shall be
  truncated to at most 8192 bytes.
- **AC-OFFICE-AGENT-COMMENT-READS-004.2:** A truncated body shall be cut at a
  rune boundary and shall remain valid UTF-8.
- **AC-OFFICE-AGENT-COMMENT-READS-004.3:** When a body is shortened, the
  returned comment shall carry a `body_truncated` marker and `body_bytes`, the
  original body length in bytes.
- **AC-OFFICE-AGENT-COMMENT-READS-004.4:** When a body is not shortened, the
  returned comment shall carry neither a `body_truncated` marker nor a
  `body_bytes` value.
- **AC-OFFICE-AGENT-COMMENT-READS-004.5:** When a body is exactly 8192 bytes,
  the tool shall return it unchanged and shall carry no `body_truncated`
  marker.
- **AC-OFFICE-AGENT-COMMENT-READS-004.6:** The tool shall return at most 65536
  bytes of comment body across a single response.
- **AC-OFFICE-AGENT-COMMENT-READS-004.7:** When the window selected by
  REQ-OFFICE-AGENT-COMMENT-READS-003 would exceed the budget in
  AC-OFFICE-AGENT-COMMENT-READS-004.6, the tool shall drop whole comments from
  the oldest end of the window until the response fits, so the newest comments
  are always the ones retained.
- **AC-OFFICE-AGENT-COMMENT-READS-004.8:** Comments dropped under
  AC-OFFICE-AGENT-COMMENT-READS-004.7 shall be excluded from `returned` and
  shall not change `total`, so `has_more` reports them.
- **AC-OFFICE-AGENT-COMMENT-READS-004.9:** When the window selected by
  REQ-OFFICE-AGENT-COMMENT-READS-003 is not empty, the response shall contain at
  least one comment. The budget in AC-OFFICE-AGENT-COMMENT-READS-004.6 shall
  never reduce a non-empty window to an empty list, because an empty list is
  read by a coordinator as "the child produced nothing" and ends the delegation.
- **AC-OFFICE-AGENT-COMMENT-READS-004.10:** AC-OFFICE-AGENT-COMMENT-READS-004.9
  holds by construction while the per-body cap in
  AC-OFFICE-AGENT-COMMENT-READS-004.1 is at or below the budget in
  AC-OFFICE-AGENT-COMMENT-READS-004.6, since the newest comment's shortened body
  then fits alone. Should either constant change so that one shortened body
  could exceed the budget, that comment shall still be returned alone.

### REQ-OFFICE-AGENT-COMMENT-READS-005: Empty, absent, and failed reads are distinguishable

**Intent:** "The child posted nothing" and "the read did not work" are different
facts leading to opposite coordinator decisions. The reported defect is a
coordinator concluding the former from evidence of neither.

#### Acceptance criteria

- **AC-OFFICE-AGENT-COMMENT-READS-005.1:** When the target task is accessible
  and has no comments, the tool shall succeed and return an empty comment list,
  never a null list, with a total of zero.
- **AC-OFFICE-AGENT-COMMENT-READS-005.2:** When a backing dependency is
  unconfigured or a storage read fails, the tool shall return an error and
  shall not return an empty comment list.
- **AC-OFFICE-AGENT-COMMENT-READS-005.3:** The access-denied error, the
  dependency error, and the empty-but-successful result shall be mutually
  distinguishable from the tool's response alone.
- **AC-OFFICE-AGENT-COMMENT-READS-005.4:** When `task_id` is omitted, empty, or
  the literal value `self`, the tool shall target the caller task.
- **AC-OFFICE-AGENT-COMMENT-READS-005.5:** When `task_id` resolves to empty
  because there is no caller task identity, the tool shall return a validation
  error naming `task_id` as required.
- **AC-OFFICE-AGENT-COMMENT-READS-005.6:** A string `task_id` shall have leading
  and trailing whitespace removed before every comparison and lookup, so a
  whitespace-only value is treated as omitted and `" self "` resolves to the
  literal `self`. The comparison against `self` shall be case-sensitive, so
  `Self` and `SELF` are task identifiers to be looked up, not the caller.
- **AC-OFFICE-AGENT-COMMENT-READS-005.7:** When the target task is archived,
  its comments shall remain readable on the same terms as an unarchived task,
  so a coordinator can read a child that was archived on completion.
- **AC-OFFICE-AGENT-COMMENT-READS-005.8:** When `task_id` is present with a null
  value, it shall be treated as omitted and shall target the caller task, on the
  same terms as AC-OFFICE-AGENT-COMMENT-READS-005.4.
- **AC-OFFICE-AGENT-COMMENT-READS-005.9:** When `task_id` is present with a
  value that is neither a string nor null, the tool shall return a validation
  error naming `task_id`, and shall not fall back to the caller task. The null
  case is governed by AC-OFFICE-AGENT-COMMENT-READS-005.8 and is not an error.

- **AC-OFFICE-AGENT-COMMENT-READS-005.10:** The `task_id` argument shall be
  declared optional with no JSON Schema type constraint, so a null or
  wrong-typed value reaches the handler instead of being rejected by argument
  validation. The validation error required by
  AC-OFFICE-AGENT-COMMENT-READS-005.9 shall be produced by the handler.

### REQ-OFFICE-AGENT-COMMENT-READS-006: Concurrent reads and writes

**Intent:** Comments arrive while a coordinator reads them.

#### Acceptance criteria

- **AC-OFFICE-AGENT-COMMENT-READS-006.1:** When a comment is written to the
  target task while a read is in flight, that comment shall either appear
  whole in the response or be absent from it, and shall never appear partially.
- **AC-OFFICE-AGENT-COMMENT-READS-006.2:** Two concurrent reads of the same
  target task with identical arguments shall each return a self-consistent
  window satisfying AC-OFFICE-AGENT-COMMENT-READS-003.7.
- **AC-OFFICE-AGENT-COMMENT-READS-006.3:** The tool shall acquire no lock that
  blocks a concurrent comment write.

### REQ-OFFICE-AGENT-COMMENT-READS-007: The surface is discoverable and advertised

**Intent:** A tool an agent is never told about is not a fix. The reported
defect is partly an instruction defect: the injected Office comment reference
documents only the write.

#### Acceptance criteria

- **AC-OFFICE-AGENT-COMMENT-READS-007.1:** The Office first-turn context shall
  advertise `list_task_comments_kandev` with its required and optional
  arguments.
- **AC-OFFICE-AGENT-COMMENT-READS-007.2:** The set of tool names advertised in
  the Office first-turn context shall remain exactly equal to the set of tools
  registered for the Office MCP surface.
- **AC-OFFICE-AGENT-COMMENT-READS-007.3:** The injected Office comment
  reference shall document reading a related task's comments alongside the
  existing write command.
- **AC-OFFICE-AGENT-COMMENT-READS-007.4:** The Coordinator role instructions,
  at `apps/backend/internal/office/configloader/instructions/ceo/AGENTS.md`,
  shall direct the agent to read a delegated child task's comments before
  concluding that the child produced no output. No other role instruction file
  is required to change.

### REQ-OFFICE-AGENT-COMMENT-READS-008: Acceptance path

**Intent:** State the end-to-end outcome as one observable behavior.

#### Acceptance criteria

- **AC-OFFICE-AGENT-COMMENT-READS-008.1:** When a child Office task's agent
  posts its stage deliverable as a task comment and its parent task's agent
  subsequently runs, the parent shall be able to obtain that comment body
  through `list_task_comments_kandev` with no human action between the two
  runs.

## Out of scope

- **Comment writes through MCP.** Agents keep writing through the signed Office
  runtime path; this document adds no write tool.
- **A monotonic comment sequence column.** Ordering is deterministic but its
  tiebreak is stable-arbitrary, not insertion order: `id` is random, so two
  comments sharing a `created_at` are ordered repeatably and not necessarily as
  written. A monotonic sequence on `task_comments` is a schema change with no
  consumer needing true insertion order today.
- **Cursor pagination.** `has_more` tells a caller older comments exist and
  offers no cursor to reach them. History older than the clamped maximum or the
  response body budget is out of scope.
- **Closing the unscoped CLI read path.** `agentctl kandev tasks conversation`
  reaches the dashboard comments endpoint with no cross-task relation check, so
  a CLI-capable agent can already read an unrelated task's comments. That gap
  pre-exists, and is neither widened nor fixed here.
- **Comment authorship correctness.** Reviewer runs execute inside the author's
  session, so `author_id` is unreliable. This document returns the recorded
  value and does not repair attribution.
- **Human-facing comment UI.** No dashboard, board, or task-detail change.
- **Notification or wake behavior.** Reading raises no wake and changes no run
  scheduling. The wake payload's five-comment window is unchanged.
