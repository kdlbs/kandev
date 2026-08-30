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
Office agent can usefully read one, so a producer stage writes its deliverable
to a channel no consumer stage can open on the terms it needs, and multi-stage
delegation cannot hand off.

The read endpoint already exists and the agent CLI already calls it, so the gap
is not a missing transport. It is a read that returns the wrong thing: it
applies no cross-task relation check, and it ignores the caller's requested
size, returning a task's whole comment history unbounded. This document makes
that endpoint safe and bounded for agent callers and leaves its human-facing
behaviour unchanged. Office owns this contract because it owns agent
coordination; the comment store and access rule are consumed, not redefined.

## Design decision

Three options were considered.

1. Expose comments to the Office MCP surface as a read tool.
2. Redirect role instructions at `write_task_document_kandev`, treating
   comments as human-facing discussion only.
3. Fix the existing REST read the agent CLI already calls, and teach the
   instructions that CLI command.

**Option 3 is chosen. It supersedes an earlier selection of option 1, which was
built and then withdrawn.** Office mode's MCP surface is deliberately minimal;
its own registry records that "Kanban tools are excluded because office agents
use CLI commands instead", and nine shipped skills under
`internal/office/configloader/skills/` are already built on that CLI. An MCP
tool contradicted that boundary and routed around a defective endpoint instead
of repairing it: `agentctl kandev comment list` and `agentctl kandev tasks
conversation` both already reach this endpoint, so the unguarded, unbounded read
would have survived beside a new guarded one. Option 2 stays rejected because
six role files already direct agents to post comments, making it a rewrite of
every role rather than an instruction edit, after which that text becomes
load-bearing for every future role author. A comment and a document are also not
interchangeable: a document is replaced wholesale by key, so two stages
publishing to a shared parent collide on key choice, while a comment is an
append-only attributed event.

The access rule, byte budget, ordering guarantee, and window reporting are
unchanged; only the surface carrying them moved.

## Prior art

**Our own prior reasoning.** Attempted, not skipped: the vault at
`/Users/henry/Documents/henry/wiki` returned `Operation not permitted` and the
`obsidian-wiki` CLI is not installed, so this leg returned nothing.

**What other products shipped.** Queried through the `saas-kb` `ai_sdlc` slice.
*Warp* gives parent and child agents inboxes on a durable bus, is explicit that
"agents don't read each other's transcripts or live working trees", and has each
review-swarm child post findings and exit while the parent fans them in. *Devin*
lets a coordinator "inspect any session's full event timeline". *OpenHands*
returned nothing on cross-agent handoff.

**What we are doing differently.** Not a bus or mailbox: Office already has a
durable, per-task, append-only, attributed channel agents are told to write to,
so the gap is a missing guarded reader. We deliberately do **not** adopt Warp's
global message sequence number: it orders a bus against payload-before-state
races, and this is a read gap, not a race, which is what makes the
stable-arbitrary tiebreak in REQ-OFFICE-AGENT-COMMENT-READS-003 acceptable.
Unlike Devin, the read is scoped by task relation, not blanket access to any
session a coordinator can name.

## Terminology

- **Comment read endpoint:** `GET /api/v1/office/tasks/{id}/comments`, the
  existing dashboard route reached by the browser UI and by both agent CLI
  commands `agentctl kandev comment list` and `kandev tasks conversation`.
- **Agent caller:** A request carrying a valid Office agent JWT, for which the
  office route group records an authenticated agent on the request context.
- **Browser caller:** Any other request on that route, including the human
  dashboard SPA.
- **Caller task:** The task an agent caller is bound to, taken from its
  validated JWT claims, never from the request path, query, or body.
- **Target task:** The task whose comments are requested, taken from the route's
  path parameter.
- **Read relation:** The existing cross-task read rule shared with the task
  document tools: target is the caller itself, an ancestor, descendant, sibling
  with a shared non-empty parent, or blocker of the caller, both in the same
  workspace.
- **Window:** The `total` / `returned` / `has_more` triple describing how much
  of a task's comment history a response covers.
- **Coordinator:** The `ceo` Office role, at
  `apps/backend/internal/office/configloader/instructions/ceo/AGENTS.md`. No
  role directory is named `coordinator`; the six are `ceo`, `devops`, `qa`,
  `reviewer`, `security`, `worker`.

## Requirements

### REQ-OFFICE-AGENT-COMMENT-READS-001: Read a related task's comments

**Intent:** Let a coordinating agent read the deliverable a child stage wrote,
so a fan-in completes without a human relaying the payload, and let no agent
read a task it has no relation to.

**User story:** As an Office coordinator agent, I want to read a child task's
comments, so I can act on a completed stage's written output.

#### Acceptance criteria

- **AC-OFFICE-AGENT-COMMENT-READS-001.1:** The comment read endpoint shall
  apply the read relation to every agent caller. No MCP surface, Office
  included, shall register a comment read tool.
- **AC-OFFICE-AGENT-COMMENT-READS-001.2:** When the caller task and the target
  task satisfy the read relation, the endpoint shall return the target's
  comments.
- **AC-OFFICE-AGENT-COMMENT-READS-001.3:** When the target task is a descendant
  of the caller task, the read relation shall be satisfied, so a parent can
  read a child's comments.
- **AC-OFFICE-AGENT-COMMENT-READS-001.4:** When the caller and target do not
  satisfy the read relation, the endpoint shall return a forbidden status and no
  comment content, including when both tasks are in the caller's own workspace.
- **AC-OFFICE-AGENT-COMMENT-READS-001.5:** When the target task does not exist,
  the endpoint shall return the same forbidden status and the same message as an
  unrelated existing task, so a caller cannot distinguish the two.
- **AC-OFFICE-AGENT-COMMENT-READS-001.6:** When the caller and target are in
  different workspaces, the endpoint shall return a forbidden status even if a
  parent, child, sibling, or blocker edge exists between them.
- **AC-OFFICE-AGENT-COMMENT-READS-001.7:** The read relation applied by this
  endpoint shall be the same rule the task document read tools apply, evaluated
  by the same shared implementation, so the two surfaces cannot diverge.
- **AC-OFFICE-AGENT-COMMENT-READS-001.8:** The endpoint's read method shall
  remain read-only, with no way to create, edit, or delete a comment.
- **AC-OFFICE-AGENT-COMMENT-READS-001.9:** The shared read-relation guard shall
  resolve a caller task or target task that does not exist to a plain denial,
  emitting no error that names the task or distinguishes a nonexistent task
  from an existing unrelated one, so that
  AC-OFFICE-AGENT-COMMENT-READS-001.5 holds at the guard, not at one caller.
- **AC-OFFICE-AGENT-COMMENT-READS-001.10:** The normalisation required by
  AC-OFFICE-AGENT-COMMENT-READS-001.9 shall take effect for every caller of the
  shared guard, including the existing task document read and write tools, so
  the surfaces cannot answer does-this-task-exist differently.
- **AC-OFFICE-AGENT-COMMENT-READS-001.11:** Any in-memory task lookup used to
  test the shared guard shall return the same not-found result shape the task
  repository returns for an absent identifier, so a guard test cannot report
  success on a branch production never reaches.
- **AC-OFFICE-AGENT-COMMENT-READS-001.12:** The denial shall be reported through
  the existing shared access-denied sentinel, whose message is the literal string
  `document access denied`. No second access-denied sentinel or comment-specific
  message shall be introduced, so the denial a caller sees is identical across
  the comment and document surfaces.
- **AC-OFFICE-AGENT-COMMENT-READS-001.13:** When an agent caller's JWT carries
  no caller task, the endpoint shall return the forbidden status of
  AC-OFFICE-AGENT-COMMENT-READS-001.12 for every target, including one that
  exists, and shall not fall back to an unguarded read.

### REQ-OFFICE-AGENT-COMMENT-READS-002: Return every author's comments

**Intent:** A stage's context includes what its peers wrote and what a human
told it. Dropping either silently hides a decision.

#### Acceptance criteria

- **AC-OFFICE-AGENT-COMMENT-READS-002.1:** The endpoint shall return comments
  regardless of author, including agent-authored and human-authored comments.
- **AC-OFFICE-AGENT-COMMENT-READS-002.2:** Each returned comment shall carry
  its `author_type`, `author_id`, and `source` values as recorded.
- **AC-OFFICE-AGENT-COMMENT-READS-002.3:** The endpoint shall accept no author,
  author-type, or source filter, so no caller receives a silently narrowed view
  of a task's history.
- **AC-OFFICE-AGENT-COMMENT-READS-002.4:** A comment returned to an agent caller
  shall omit `reply_channel_id` and shall omit per-comment run lifecycle fields.

### REQ-OFFICE-AGENT-COMMENT-READS-003: Deterministic ordering and windowing

**Intent:** A coordinator re-reading a thread must see the same thread. An
under-determined sort makes a fan-in unrepeatable and a truncated read
indistinguishable from an empty one.

#### Acceptance criteria

- **AC-OFFICE-AGENT-COMMENT-READS-003.1:** For an agent caller the endpoint
  shall select the most recent `limit` comments for the target task, ordered by
  the `created_at` column descending and, where `created_at` values are equal,
  by the `id` column descending.
- **AC-OFFICE-AGENT-COMMENT-READS-003.2:** The endpoint shall return that
  selected window ordered by the `created_at` column ascending and, where
  `created_at` values are equal, by the `id` column ascending, so the caller
  reads the thread oldest-first.
- **AC-OFFICE-AGENT-COMMENT-READS-003.3:** The tiebreak column named in
  AC-OFFICE-AGENT-COMMENT-READS-003.1 and AC-OFFICE-AGENT-COMMENT-READS-003.2
  shall be the same column in both directions, so the window boundary selects
  the same comment set on every call.
- **AC-OFFICE-AGENT-COMMENT-READS-003.4:** When an agent caller supplies no
  `limit` query parameter, or supplies one that is empty, unparseable as an
  integer, zero, or negative, the endpoint shall apply a default limit of 20 and
  shall not return an error.
- **AC-OFFICE-AGENT-COMMENT-READS-003.5:** When an agent caller's `limit`
  exceeds 100, the endpoint shall clamp it to 100 and shall not return an error.
- **AC-OFFICE-AGENT-COMMENT-READS-003.6:** Every response to an agent caller
  shall carry a window reporting `total`, the target's comment count;
  `returned`, the number of comments in this response; and `has_more`, true
  exactly when `returned` is less than `total`.
- **AC-OFFICE-AGENT-COMMENT-READS-003.7:** The total count and the returned
  comments shall be read within a single read transaction, so the returned
  count never exceeds the reported total.
- **AC-OFFICE-AGENT-COMMENT-READS-003.8:** Two calls with identical arguments
  and no intervening comment write shall return byte-identical comment
  identifiers in identical order.
- **AC-OFFICE-AGENT-COMMENT-READS-003.9:** No `limit` value from an agent caller
  shall produce an error from any layer; every value shall be defaulted or
  clamped per AC-OFFICE-AGENT-COMMENT-READS-003.4 and
  AC-OFFICE-AGENT-COMMENT-READS-003.5.
- **AC-OFFICE-AGENT-COMMENT-READS-003.10:** The window field named in
  AC-OFFICE-AGENT-COMMENT-READS-003.6 shall describe only how many comments the
  response omits, and shall never report whether any body was shortened.
- **AC-OFFICE-AGENT-COMMENT-READS-003.11:** For a browser caller the endpoint
  shall keep its current behaviour: no default limit, the full comment list, the
  existing response shape including per-comment run lifecycle fields, and no
  read-relation check. Its valid positive `limit` may be honoured; absent one,
  no limit shall be imposed.

### REQ-OFFICE-AGENT-COMMENT-READS-004: Bounded bodies

**Intent:** A comment body is unbounded text, and an unbounded read can exhaust
a consuming agent's context and lose the deliverable it was fetched for.

#### Acceptance criteria

- **AC-OFFICE-AGENT-COMMENT-READS-004.1:** Each comment body returned to an
  agent caller shall be truncated to at most 8192 bytes.
- **AC-OFFICE-AGENT-COMMENT-READS-004.2:** A truncated body shall be cut at a
  rune boundary and shall remain valid UTF-8.
- **AC-OFFICE-AGENT-COMMENT-READS-004.3:** When a body is shortened, the
  returned comment shall carry a `body_truncated` marker and `body_bytes`, the
  original body length in bytes.
- **AC-OFFICE-AGENT-COMMENT-READS-004.4:** When a body is not shortened, the
  returned comment shall carry neither a `body_truncated` marker nor a
  `body_bytes` value.
- **AC-OFFICE-AGENT-COMMENT-READS-004.5:** When a body is exactly 8192 bytes,
  the endpoint shall return it unchanged and shall carry no `body_truncated`
  marker.
- **AC-OFFICE-AGENT-COMMENT-READS-004.6:** The endpoint shall return at most
  65536 bytes of comment body across a single response to an agent caller.
- **AC-OFFICE-AGENT-COMMENT-READS-004.7:** When the window selected by
  REQ-OFFICE-AGENT-COMMENT-READS-003 would exceed the budget in
  AC-OFFICE-AGENT-COMMENT-READS-004.6, the endpoint shall drop whole comments
  from the oldest end of the window until the response fits, so the newest
  comments are always the ones retained.
- **AC-OFFICE-AGENT-COMMENT-READS-004.8:** Comments dropped under
  AC-OFFICE-AGENT-COMMENT-READS-004.7 shall be excluded from `returned` and
  shall not change `total`, so `has_more` reports them.
- **AC-OFFICE-AGENT-COMMENT-READS-004.9:** When the window selected by
  REQ-OFFICE-AGENT-COMMENT-READS-003 is not empty, the response shall contain at
  least one comment. The budget in AC-OFFICE-AGENT-COMMENT-READS-004.6 shall
  never reduce a non-empty window to an empty list, because an empty list is
  read by a coordinator as "the child produced nothing" and ends the delegation.
- **AC-OFFICE-AGENT-COMMENT-READS-004.10:** Should the constants in
  AC-OFFICE-AGENT-COMMENT-READS-004.1 and AC-OFFICE-AGENT-COMMENT-READS-004.6
  ever change so that one shortened body could exceed the budget, that comment
  shall still be returned alone, so
  AC-OFFICE-AGENT-COMMENT-READS-004.9 does not depend on their present values.
- **AC-OFFICE-AGENT-COMMENT-READS-004.11:** Neither cap shall apply to a browser
  caller, whose bodies are returned whole.

### REQ-OFFICE-AGENT-COMMENT-READS-005: Empty, denied, and failed reads are distinguishable

**Intent:** "The child posted nothing" and "the read did not work" are different
facts leading to opposite coordinator decisions. The reported defect is a
coordinator concluding the former from evidence of neither.

#### Acceptance criteria

- **AC-OFFICE-AGENT-COMMENT-READS-005.1:** When the target task is accessible
  and has no comments, the endpoint shall succeed and return an empty comment
  list, never a null list, with a total of zero.
- **AC-OFFICE-AGENT-COMMENT-READS-005.2:** When a backing dependency is
  unconfigured or a storage read fails, the endpoint shall return an error
  status and shall not return an empty comment list.
- **AC-OFFICE-AGENT-COMMENT-READS-005.3:** The denied outcome, the dependency
  error, and the empty-but-successful result shall be mutually distinguishable
  from the response status and body alone.
- **AC-OFFICE-AGENT-COMMENT-READS-005.4:** The target task shall be taken from
  the route's path parameter alone. No request field shall be able to redirect
  the read to another task, and no caller-supplied value shall be able to
  substitute the caller task for the requested one.
- **AC-OFFICE-AGENT-COMMENT-READS-005.5:** The caller task shall be taken from
  the agent caller's validated JWT claims alone, so a caller cannot claim to be
  a task it is not.
- **AC-OFFICE-AGENT-COMMENT-READS-005.6:** When the target task is archived,
  its comments shall remain readable on the same terms as an unarchived task,
  so a coordinator can read a child that was archived on completion.

### REQ-OFFICE-AGENT-COMMENT-READS-006: Concurrent reads and writes

**Intent:** Comments arrive while a coordinator reads them.

#### Acceptance criteria

- **AC-OFFICE-AGENT-COMMENT-READS-006.1:** When a comment is written to the
  target task while a read is in flight, that comment shall either appear
  whole in the response or be absent from it, and shall never appear partially.
- **AC-OFFICE-AGENT-COMMENT-READS-006.2:** Two concurrent reads of one target
  with identical arguments shall each return a self-consistent window
  satisfying AC-OFFICE-AGENT-COMMENT-READS-003.7.
- **AC-OFFICE-AGENT-COMMENT-READS-006.3:** The read shall take no lock that
  blocks a concurrent comment write.

### REQ-OFFICE-AGENT-COMMENT-READS-007: The surface is discoverable and advertised

**Intent:** A capability an agent is never told about is not a fix. The reported
defect is partly an instruction defect: the injected Office comment reference
documents only the write.

#### Acceptance criteria

- **AC-OFFICE-AGENT-COMMENT-READS-007.1:** The Office first-turn context shall
  advertise no comment read tool, since none is registered.
- **AC-OFFICE-AGENT-COMMENT-READS-007.2:** The set of tool names advertised in
  the Office first-turn context shall remain exactly equal to the set of tools
  registered for the Office MCP surface.
- **AC-OFFICE-AGENT-COMMENT-READS-007.3:** The injected Office comment
  reference shall document reading a related task's comments with the CLI
  command, its `--task` and `--limit` flags, and the default and maximum limit,
  alongside the existing write command.
- **AC-OFFICE-AGENT-COMMENT-READS-007.4:** The Coordinator role instructions,
  at `apps/backend/internal/office/configloader/instructions/ceo/AGENTS.md`,
  shall direct the agent to read a delegated child task's comments with that CLI
  command before concluding that the child produced no output. No other role
  instruction file is required to change.

### REQ-OFFICE-AGENT-COMMENT-READS-008: Acceptance path

**Intent:** State the end-to-end outcome as one observable behavior.

#### Acceptance criteria

- **AC-OFFICE-AGENT-COMMENT-READS-008.1:** When a child Office task's agent
  posts its stage deliverable as a task comment and its parent task's agent
  subsequently runs, the parent shall be able to obtain that comment body by
  running `agentctl kandev comment list` against the child task, with no human
  action between the two runs.

## Out of scope

- **Comment writes.** Agents keep writing through the signed Office runtime
  path; this document changes no write.
- **A monotonic comment sequence column.** Ordering is deterministic but its
  tiebreak is stable-arbitrary, not insertion order: `id` is random, so two
  comments sharing a `created_at` are ordered repeatably and not necessarily as
  written. Adding a sequence column is a schema change no consumer needs today.
- **Cursor pagination.** `has_more` reports that older comments exist and
  offers no cursor to reach them. History beyond the clamped maximum or the
  response budget is out of scope.
- **The unguarded task document endpoints.** The dashboard document routes apply
  no relation check either. That gap pre-exists, is the same shape as the one
  closed here, and is neither widened nor fixed here.
- **Comment authorship correctness.** Reviewer runs execute inside the author's
  session, so `author_id` is unreliable. This document returns the recorded
  value and does not repair attribution.
- **Human-facing comment UI.** No dashboard, board, or task-detail change.
  Preserving that surface's behaviour is a requirement here, not a change to it.
- **Notification or wake behavior.** Reading raises no wake and changes no run
  scheduling; the wake payload's five-comment window is unchanged.
