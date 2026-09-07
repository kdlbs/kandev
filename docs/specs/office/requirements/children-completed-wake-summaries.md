---
status: draft
system: office
created: 2026-09-08
owners:
  - kandev
---

# Children-Completed Wake Summaries Requirements

## Overview

When every direct child of a parent task reaches a terminal state, Kandev wakes
the parent's agent with a `task_children_completed` run. The wake prompt tells
the parent that its children finished. It does not tell the parent *which*
children finished, what state each ended in, what each concluded, or what each
produced.

The prompt has always had a section for exactly that. It renders only when the
assembled prompt context carries child summaries, and nothing has ever put them
there: the only code that populates them reads a `children` key out of the run
payload, and no producer of this run writes that key. Three of the four
producers do assemble child summaries — two of them paying a database read and a
pull-request lookup to do it — and then discard them at a boundary that has no
field to carry them. The result is a wake that costs a full agent turn and
begins with the parent knowing nothing about the work it delegated.

This capability makes the children-completed wake name its children. It defines
what a child summary line reports, which children appear, in what order, and how
the prompt behaves when the underlying reads are unavailable.

The Office system owns this contract because the outcome is the content of an
Office wake prompt: what an autonomous agent is told when the Office scheduler
launches it. Adjacent contracts this capability reads but does not own are the
task system's parent/child relationship, task state, comments, and archival, and
the workflow engine's `on_children_completed` trigger and its payload type.

This capability is constrained by
[parent wake wave identity](parent-wake-wave-identity.md), which collapses the
four producers onto a single run per completion wave. That capability's
AC-OFFICE-WAKE-WAVE-IDENTITY-002.10 requires the surviving run to deliver a wake
equivalent to the one any other producer would have delivered. Producer-specific
prompt content would make prompt quality a race outcome, so REQ-OFFICE-WAKE-
CHILD-SUMMARIES-002 below is not a convenience — it is the condition under which
this capability can ship at all.

## Prior art

**Wiki leg — receipt: not run, tool absent.** The step's `wiki-query` leg could
not be executed in this session. `find ~/.claude /Users/neo/Projects/gstack
-maxdepth 3 -name 'wiki-query*'` returned nothing, `~/.obsidian-wiki` does not
exist (`ls: No such file or directory`), and `OBSIDIAN_VAULT_PATH` is unset, so
there is no vault to pin with `@henry` and no QMD collection to name. No
degraded grep fallback was substituted, because a keyword sweep over a vault
that is not present would produce an empty result indistinguishable from a
healthy miss.

**saas-kb leg — receipt: not run, server absent.** The `saas-kb` MCP server is
not registered in this session; the only MCP server available is `kandev`
(`~/.claude.json` `mcpServers` is empty and the session tool registry exposes no
`search_fsm_docs`). No `ai_sdlc` query was issued and no vendor comparison is
claimed.

**In-repo prior art, which was available and was read.** The immediately
preceding capability,
[parent wake wave identity](parent-wake-wave-identity.md), took a position on
this exact defect while specifying something else. Its system design records, in
its "Wake equivalence between producers" section, that all four producers queue a
payload with no child summaries, that this is why collapsing a racing pair is
currently safe, and that "a change that starts populating `children` from one
producer must populate it from all four." This capability adopts that position
rather than re-deriving it, and satisfies it structurally — by removing the
per-producer payload as the carrier — instead of by editing four producers to
agree.

## Terminology

- **Children-completed wake:** the run the Office scheduler dispatches with
  reason `task_children_completed`, or the legacy reason `children_completed`,
  to wake a parent whose children have all reached a terminal state.
- **Producer:** any code path that can cause a children-completed run to be
  queued. Four exist, as enumerated by
  [parent wake wave identity](parent-wake-wave-identity.md).
- **Prompt assembly time:** the moment the scheduler renders the wake prompt for
  a claimed run, immediately before launching the agent session. This is later
  than, and can be much later than, the moment the run was queued.
- **Live direct child:** a task whose parent is the woken parent task and whose
  `archived_at` is unset.
- **Child summary line:** one rendered line describing one live direct child.
- **Child list section:** the block of child summary lines, with its heading and
  any truncation notice, inside the wake prompt.

## Requirements

### REQ-OFFICE-WAKE-CHILD-SUMMARIES-001: The wake names the children it is about

**Intent:** A parent woken to review delegated work must be told what that work
was and how it ended, in the wake itself, without spending a tool call to
discover the identity of its own children.

**User story:** As an autonomous parent agent, I want the wake that tells me my
children finished to also tell me which children and what they concluded, so
that my first action can be judgement rather than discovery.

#### Acceptance criteria

- **AC-OFFICE-WAKE-CHILD-SUMMARIES-001.1:** When a children-completed wake
  prompt is assembled for a parent that has at least one live direct child, the
  prompt shall contain a child list section holding one child summary line for
  each live direct child, subject to the display cap in
  AC-OFFICE-WAKE-CHILD-SUMMARIES-003.4.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-001.2:** Each child summary line shall report
  the child's task identifier, the child's title, and the child's task state.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-001.3:** When a child has at least one
  comment, its line shall report the body of that child's most recent comment.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-001.4:** When a child has no comments, its
  line shall report no comment text and shall still report identifier, title,
  and state.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-001.5:** When a reported comment body is
  longer than the per-comment display limit, the line shall report the leading
  portion of that body followed by an explicit truncation marker. A body at or
  below the limit shall be reported without a marker.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-001.6:** A reported comment body shall occupy
  a single line. Line breaks within the body shall not split a child summary
  line into two.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-001.7:** When a child has one or more linked
  pull requests, its line shall report each of those pull request URLs.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-001.8:** When a child has no linked pull
  request, its line shall report no pull-request text, and its remaining fields
  shall be unaffected.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-001.9:** When a child has no task identifier,
  its line shall render a fixed placeholder in the identifier position and shall
  still report title and state.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-001.10:** When the parent has no live direct
  children at prompt assembly time, the prompt shall contain no child list
  section, no heading for it, and no truncation notice.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-001.11:** The wake prompt's existing lead-in
  sentence and its existing closing instruction shall be present and unchanged in
  every case, including the empty case in
  AC-OFFICE-WAKE-CHILD-SUMMARIES-001.10 and the failure cases in
  REQ-OFFICE-WAKE-CHILD-SUMMARIES-004.

### REQ-OFFICE-WAKE-CHILD-SUMMARIES-002: One rendering, whichever producer won

**Intent:** Four producers can queue this run, and
[wave identity](parent-wake-wave-identity.md) makes exactly one of them win a
race that none of them can observe. If the prompt's content depended on which
one won, the parent's briefing would be decided by a race. The only way to
guarantee equivalence across four producers is to stop asking the producers for
the content.

#### Acceptance criteria

- **AC-OFFICE-WAKE-CHILD-SUMMARIES-002.1:** For a given parent and a given
  prompt assembly moment, the rendered child list section shall be byte-identical
  regardless of which producer queued the run.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-002.2:** The rendered child list section shall
  be derived from the parent's children as they are at prompt assembly time, and
  shall not be derived from any snapshot captured when the run was queued.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-002.3:** A `children` key present in a run
  payload shall not contribute to, suppress, or alter the rendered child list
  section. The same shall hold for a `truncated` key.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-002.4:** No producer shall be required to
  place child summary data, pull-request data, or a truncation flag into a run
  payload for the child list section to render.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-002.5:** No producer shall perform a database
  read whose only consumer is child summary data discarded before the run row is
  written.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-002.6:** The child list section shall render
  for both the current children-completed run reason and the legacy one.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-002.7:** Runs of every other reason shall
  render exactly the prompt they render today. No other wake shall gain or lose
  content, and no other wake shall gain a database read.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-002.8:** A child summary line shall report the
  child's task state as the task system holds it. A producer-specific
  reinterpretation of that state, such as one producer's substitution of a
  completed state for a child occupying a terminal workflow step, shall not be
  reproduced at prompt assembly time, because it is not available to the other
  three producers and would violate
  AC-OFFICE-WAKE-CHILD-SUMMARIES-002.1.

### REQ-OFFICE-WAKE-CHILD-SUMMARIES-003: Deterministic membership, order, and cap

**Intent:** The same parent in the same state must produce the same briefing
twice. Every axis that could decide otherwise — which children count, what order
they appear in, which comment is "the" comment, and what happens past the cap —
is named here rather than left to whichever row the database returns first.

#### Acceptance criteria

- **AC-OFFICE-WAKE-CHILD-SUMMARIES-003.1:** The child list section shall include
  live direct children only. An archived child shall not appear in it.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-003.2:** Child summary lines shall be ordered
  ascending by the child's creation timestamp, tiebroken ascending by `tasks.id`.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-003.3:** A child's reported comment shall be
  its most recent comment by descending comment creation timestamp, tiebroken by
  descending comment id, regardless of the comment's author or author type.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-003.4:** The child list section shall contain
  at most 20 child summary lines. When the parent has more than 20 live direct
  children, the lines present shall be the first 20 under the ordering in
  AC-OFFICE-WAKE-CHILD-SUMMARIES-003.2.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-003.5:** When the parent's live direct child
  count exceeds 20, the child list section shall carry a truncation notice
  stating that the list is partial. When it does not exceed 20, the section shall
  carry no truncation notice.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-003.6:** The count that decides
  AC-OFFICE-WAKE-CHILD-SUMMARIES-003.5 shall count live direct children only.
  Archived children shall not be able to raise that count, so a truncation notice
  shall never appear while every live direct child is listed.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-003.7:** When a child has more than one linked
  pull request, its reported URLs shall be ordered ascending by URL string.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-003.8:** Two prompt assemblies for the same
  parent, with no intervening change to that parent's children, their titles,
  states, archival, comments, or pull-request links, shall produce byte-identical
  child list sections.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-003.9:** Membership, ordering, the cap, and
  the truncation decision shall behave identically on SQLite and on PostgreSQL.

### REQ-OFFICE-WAKE-CHILD-SUMMARIES-004: The briefing never costs the wake

**Intent:** The child list is context, not the point. A parent whose children
finished must still be woken when the context is unavailable, and must be woken
with a prompt that is honestly short rather than one that is wrong.

#### Acceptance criteria

- **AC-OFFICE-WAKE-CHILD-SUMMARIES-004.1:** When the read of the parent's
  children fails, the prompt shall render with no child list section, the run
  shall proceed to launch, and the failure shall not be reported to the agent as
  prompt content.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-004.2:** When the pull-request lookup fails or
  is not configured, every child summary line shall still render its identifier,
  title, state, and comment, without pull-request text, and the run shall proceed
  to launch.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-004.3:** When the parent task does not exist
  at prompt assembly time, the prompt shall render with no child list section and
  the run shall proceed to launch.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-004.4:** No failure of a read performed for
  the child list section shall fail the run, change the run's status, or schedule
  a retry.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-004.5:** When a child's state, title,
  archival, comments, or pull-request links change between the moment the run is
  queued and prompt assembly time, the rendered section shall reflect the values
  at prompt assembly time.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-004.6:** When a run is retried or reassembled,
  the child list section shall be re-derived at each assembly rather than reused
  from the previous assembly.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-004.7:** When a child is written concurrently
  with prompt assembly, the section shall report either that child's pre-write or
  its post-write values, and assembly shall not fail. The section is a
  point-in-time reading and carries no cross-child atomicity guarantee.
- **AC-OFFICE-WAKE-CHILD-SUMMARIES-004.8:** Prompt assembly for one
  children-completed run shall perform a bounded number of reads that does not
  grow with the number of live direct children.

## Out of scope

- **When a children-completed wake fires, and which children make a parent
  ready.** Readiness is the task system's
  [subtask completion trigger](../../tasks/requirements/subtask-completion-trigger.md)
  contract and the Office scheduler's existing readiness checks. This capability
  changes only what the resulting wake says.

- **The existing divergence in archived-child handling between readiness and
  membership.** Office readiness counts archived children as blocking; this
  capability's child list excludes them
  (AC-OFFICE-WAKE-CHILD-SUMMARIES-003.1). The two therefore answer different
  questions, deliberately: readiness asks whether anything is still running, the
  list describes the wave the parent is being asked to review.
  [Parent wake wave identity](parent-wake-wave-identity.md)
  AC-OFFICE-WAKE-WAVE-IDENTITY-004.7 leaves that divergence in place, and this
  capability does not close it either. The visible consequence is bounded and
  named: a parent all of whose children are archived and terminal can be woken
  with no child list section, which
  AC-OFFICE-WAKE-CHILD-SUMMARIES-001.10 defines as a valid rendering.

- **Wave identity, run deduplication, and backstop admission.** Owned by
  [parent wake wave identity](parent-wake-wave-identity.md). This capability
  depends on that one's producer-equivalence requirement and adds no identity of
  its own. In particular, the rendered content of the wake shall not be an input
  to any deduplication decision.

- **Removing `ChildSummaries` from the workflow engine's
  `on_children_completed` trigger payload type.** That field is part of a
  workflow-engine type shared across systems, and no consumer reads it today.
  AC-OFFICE-WAKE-CHILD-SUMMARIES-002.5 stops Office producers paying for data
  that is discarded, but the field itself stays. Deleting a field from a shared
  trigger payload is a change to the workflow engine's contract, with a different
  owner and a different blast radius, and belongs in its own capability. A future
  capability that removes it needs to know that the field is written by three
  producers, read by none, and unreachable from workflow conditions, which reach
  the trigger payload only through a typed assertion for comment payloads.

- **Merging the two children-completed run reasons.** The legacy reason is
  rendered by AC-OFFICE-WAKE-CHILD-SUMMARIES-002.6 and is otherwise untouched.

- **The cross-parent coalescing behaviour of children-completed runs.** Two
  children-completed runs for different parents addressed to the same agent
  within the coalescing window can merge, because this reason is not
  task-scoped for coalescing. That is pre-existing, is not caused or worsened by
  this capability — deriving content at assembly time from the surviving run's
  own parent makes the merged run's prompt self-consistent rather than less so —
  and its correctness is a separate question about which parent should have been
  woken, not about what the wake says.

- **User interface.** This capability adds no control, view, or setting. Its
  output is visible in two existing surfaces without change to either: the agent
  session's first message, and the assembled-prompt panel on the Office run
  detail page.

- **Changing which comment represents a child's conclusion.** The most recent
  comment by any author is what this capability reports
  (AC-OFFICE-WAKE-CHILD-SUMMARIES-003.3). Selecting a designated summary
  comment, or preferring an agent-authored comment over a user's, is a different
  contract about what a child's conclusion *is*, and is not decided here.
