---
status: draft
system: office
created: 2026-09-06
owners:
  - kandev
---

# Office Run Causation Chain Requirements

## Overview

Office starts agent processes from several triggers, and one launch can cause
another: an agent creates a task, the task wakes its assignee, that agent
comments, and a further wake follows. Nothing today records that one run caused
another, so a chain cannot be reconstructed after the fact.

A run records `actor_id` in its payload for some wake reasons, which names the
agent that acted but not the run it was acting inside. That is enough to
attribute a single hop and not enough to follow a chain, compute how deep it has
gone, or answer which originating trigger a given launch belongs to.

This document defines that identity once. Two consumers need it and neither
should define its own: the launch-safety limits, which bound how deep a chain may
go and which launches a budget applies to, and operability, which needs one
identifier traceable from a wake through its run and session to any run that
session spawns.

Office owns this contract because Office owns the run row and the enqueue path
where causation is decided.

## Terminology

- **Causation chain:** the set of runs reachable from one root, where each run was
  queued as a consequence of its parent. A run can have several children, so the
  shape is a tree rooted at the **root** run.
- **Causation depth:** the number of edges between a run and its root. A root has
  depth `0`.
- **Root cause:** a wake with no Office run behind it. Human action, a cron routine
  fire, an inbound webhook, and the recovery sweep are root causes.
- **Actor:** who performed the action that caused a wake, as one of exactly three
  kinds: a human user, an Office agent, or the system itself. The actor is a
  property of the causing action, not of the agent being woken.
- **Human-rooted:** a property of the whole chain, true when the root of that chain
  was caused by a human actor. Every run in a human-rooted chain is human-rooted,
  at every depth.
- **Routine attribution:** the routine a run is chargeable to, whether the run was
  queued directly by a routine fire or by a task that a routine created.

## Requirements

### REQ-OFFICE-RUN-CAUSATION-001: Causation chain identity

**Intent:** Make every launch attributable to what caused it, with one identifier
that survives every hop. Today a run records `actor_id` in its payload for some
reasons only, naming an agent but not a run, so a chain cannot be reconstructed
and depth cannot be computed. The same identifier satisfies the correlatability
requirement the Office beta review left unspecified, so it is defined once here.

**User story:** As an operator reviewing an unattended run, I want one identifier
leading from the originating wake to every run it caused, so that I can see the
chain without correlating log lines by timestamp.

#### Acceptance criteria

- **AC-OFFICE-RUN-CAUSATION-001.1:** Every run shall carry a causation identifier,
  an optional parent run identifier, and a non-negative integer causation depth,
  each persisted on the run and readable through the run API.
- **AC-OFFICE-RUN-CAUSATION-001.2:** When a run is queued from a root cause, the
  system shall set its causation identifier to its own run identifier, its parent
  run identifier to empty, and its causation depth to `0`.
- **AC-OFFICE-RUN-CAUSATION-001.3:** When a run is queued as a consequence of a
  causing run, the system shall set its causation identifier to the causing run's
  causation identifier, its parent run identifier to the causing run's identifier,
  and its causation depth to the causing run's depth plus `1`.
- **AC-OFFICE-RUN-CAUSATION-001.4:** The causation identifier and depth shall be
  carried into the agent process for the run and shall be returned unchanged on
  every runtime action that agent performs, so that a run queued by a runtime
  action is attributed to the run that performed it.
- **AC-OFFICE-RUN-CAUSATION-001.5:** When an Office trigger creates a task, the
  system shall persist the carrier set defined by AC-OFFICE-RUN-CAUSATION-001.18 on
  that task. An Office trigger is any of: an agent creating a task through a runtime
  action, and a routine fire creating a task. A run later queued because of that task
  shall inherit the carrier set as though the task creation were the causing run,
  taking the persisted run identifier as its parent run identifier so that
  AC-OFFICE-RUN-CAUSATION-001.3 is satisfiable across the task boundary. A routine
  fire has no creating run, so it persists an empty run identifier, an empty causation
  identifier, depth `0`, and its own routine identifier, which is what makes a heavy
  routine's launches chargeable under AC-OFFICE-RUN-CAUSATION-001.14.
- **AC-OFFICE-RUN-CAUSATION-001.6:** When a run row predates this capability and
  carries no causation identifier, a reader shall treat that run as its own root at
  depth `0` rather than failing. No backfill of historical rows is performed.
- **AC-OFFICE-RUN-CAUSATION-001.7:** When two wake requests coalesce into one run,
  the surviving run shall retain the causation identifier, parent run identifier,
  and depth it was created with, and shall not adopt those of the request merged
  into it.
- **AC-OFFICE-RUN-CAUSATION-001.8:** When the causing run carries the legacy empty
  causation identifier, the run it causes shall adopt the causing run's own run
  identifier as its causation identifier, so a chain rooted before this capability
  existed still produces one identifier for every later hop.
- **AC-OFFICE-RUN-CAUSATION-001.9:** When a wake's actor is a human user, the
  resulting run shall be a root at depth `0`, discarding any causation the
  originating task or comment carried. Human action ends a chain and starts a new
  one.
- **AC-OFFICE-RUN-CAUSATION-001.10:** When a value of the carrier set is absent,
  malformed, non-numeric, or negative on the task it was carried on, the system shall
  increment a counter naming the value and the reason, and shall resolve that value to
  its most restrictive reading rather than failing the enqueue or guessing: causation
  identifier, parent run identifier and depth resolve to a root at depth `0`; the
  human-rooted flag resolves to false; the routine attribution resolves to empty. Each
  value is resolved independently, so one malformed value shall not discard the
  others.
- **AC-OFFICE-RUN-CAUSATION-001.11:** The causation identifier, parent run
  identifier, causation depth, actor, human-rooted flag, and routine attribution
  shall not contribute to a run's idempotency key, so that two enqueues that are
  duplicates today remain duplicates.
- **AC-OFFICE-RUN-CAUSATION-001.12:** One run can cause more than one run. Causation
  forms a tree rooted at the root run, not a single line: two runs caused by the
  same run shall each carry that run as parent, shall share its causation
  identifier, and shall each be at the same depth. Depth counts edges from the
  root, never the number of runs sharing a causation identifier.
- **AC-OFFICE-RUN-CAUSATION-001.13:** Every run shall carry a persisted boolean
  recording whether its chain is human-rooted. A root run shall set it from its own
  actor; a caused run shall inherit the causing run's value unchanged, taking it from
  the carrier set of AC-OFFICE-RUN-CAUSATION-001.18 when the inheritance crosses a
  task boundary, so that no read of the creating run is required. When the value is
  nonetheless unavailable, the resulting run shall not be human-rooted, so that a
  missing ancestor cannot manufacture a budget exemption. A consumer shall read this
  flag rather than walking the chain to its root, so that the answer does not change
  when the root run is pruned. A direct enqueue naming an unreadable causing run is
  governed by AC-OFFICE-RUN-CAUSATION-001.21 and is refused, not defaulted.
- **AC-OFFICE-RUN-CAUSATION-001.14:** Every run shall carry a persisted routine
  attribution, empty when the run is attributable to no routine. A run queued
  directly by a routine fire shall take that routine; a run queued because of a task
  a routine created shall take the routine persisted on that task. The attribution
  shall be a first-class field on the run, not a value a consumer extracts from a
  payload document.
- **AC-OFFICE-RUN-CAUSATION-001.15:** Every enqueue shall carry a typed actor,
  taking exactly one of the values `user`, `agent`, or `system`, as a declared field
  of the enqueue request rather than a convention inside a payload document. The
  value `agent` shall be accompanied by the acting agent profile. The field shall be
  **required**: every enqueue path shall state its actor explicitly, and no path shall
  reach the queue without one. A caller whose action genuinely has neither a human nor
  an agent behind it states `system` deliberately, rather than reaching it by omission.
- **AC-OFFICE-RUN-CAUSATION-001.16:** When an enqueue supplies no actor, or an actor
  the system does not recognize, the system shall record the actor as `system` and
  increment a counter naming the enqueue reason. An actor declared `agent` with no
  acting agent profile shall be treated as unrecognized by this rule. An absent or
  unrecognized actor shall never be read as `user`, so that a rule keyed on a human
  actor fails toward the restrictive answer. This rule is a residual defence, not a
  supported way to omit the actor: AC-OFFICE-RUN-CAUSATION-001.15 makes the field
  required, so a path that relies on this rule to supply `system` is a defect, and the
  counter it increments is how that defect is found.
- **AC-OFFICE-RUN-CAUSATION-001.17:** An agent shall not be able to set, reset, or
  lower its own causation identifier, parent run identifier, depth, human-rooted
  flag, or actor. The system shall derive every one of them from the server-side run
  record rather than from the request body, and shall write the carrier set of
  AC-OFFICE-RUN-CAUSATION-001.18 itself at task creation rather than accepting it from
  the creating agent.
- **AC-OFFICE-RUN-CAUSATION-001.18:** The **carrier set** is the closed set of values
  that must survive the task boundary, and it shall be exactly those persisted run
  values that a later enqueue would otherwise have to obtain by reading the creating
  run row. It comprises the causation identifier, the causation depth, the creating
  run identifier, the human-rooted flag, the routine attribution, and the actor kind
  and actor identifier. A value may be recorded as **re-derivable**, and so kept out of
  the carrier, only when AC-OFFICE-RUN-CAUSATION-001.23 declares a named source for it
  at **every** enqueue path; a re-derivability claim that holds at some paths and not
  others is not a disposition but an omission. The system shall have a test that fails
  when a value in the set of persisted run values named by
  AC-OFFICE-RUN-CAUSATION-001.1, `001.13`, `001.14`, `001.19` or `001.20` neither joins
  the carrier set nor has a named source at every enqueue path, and recording a
  disposition shall not by itself satisfy it. The actor is in the carrier because it
  failed in exactly that way: it was recorded as re-derivable from the new wake's own
  actor while the task-boundary path had no actor to re-derive from, and a test that
  asked only whether a disposition existed could not see it.
- **AC-OFFICE-RUN-CAUSATION-001.19:** Every run shall carry a persisted actor,
  recording both the actor kind of AC-OFFICE-RUN-CAUSATION-001.15 and the acting user
  or agent profile, empty when the kind is `system`. A consumer shall read the
  persisted actor rather than a payload document or a parent-run join, so that a rule
  counting past wakes by actor has a source that does not depend on any other run row
  surviving.
- **AC-OFFICE-RUN-CAUSATION-001.20:** Every run shall carry a persisted workspace.
  For a profile with a workspace, enqueue shall use that profile workspace. A global
  Kanban profile may have no workspace of its own; when its enqueue is task-bound,
  enqueue shall use the owning task's persisted workspace as the trusted scope. A
  taskless global profile, an unknown task, or an otherwise absent or empty workspace
  shall refuse the enqueue with a distinguishable error and increment a counter,
  rather than queueing a run that no workspace-scoped ceiling or budget can be
  counted against. An empty workspace shall never be used as a countable scope value,
  and callers shall not supply the workspace as an override.
- **AC-OFFICE-RUN-CAUSATION-001.21:** When an enqueue supplies a causing run
  identifier that is well-formed but names a run the system cannot read, the system
  shall refuse the enqueue with a distinguishable error and increment a counter naming
  the reason. It shall not treat the run as a root. Rooting an unreadable ancestor at
  depth `0` would let any chain reset its own depth by outliving one row, which is the
  control REQ-OFFICE-LAUNCH-SAFETY-003 exists to provide. This differs deliberately
  from AC-OFFICE-RUN-CAUSATION-001.10, where the causation crossed a task boundary and
  the creating run was never named.
- **AC-OFFICE-RUN-CAUSATION-001.22:** When a wake request is merged into an existing
  run by coalescing, the surviving run shall retain its own actor, routine
  attribution, and priority class unchanged, on the same reasoning as
  AC-OFFICE-RUN-CAUSATION-001.7. The merged request shall contribute none of them.
- **AC-OFFICE-RUN-CAUSATION-001.23:** Every enqueue path shall have a named actor
  source, and those sources shall be declared in exactly one place. A path with no
  named source is a defect. The declared sources are: a runtime action takes the
  authenticated run's agent profile, with kind `agent`; a user-initiated request takes
  the authenticated user, with kind `user`; a routine fire takes kind `system`; and a
  wake queued because of a task takes the actor carried on that task under
  AC-OFFICE-RUN-CAUSATION-001.18. The system shall have a test that fails when an
  enqueue path can reach the queue without a source in that declaration. Enumerating
  the paths that happen to pass an actor today is not sufficient, for the reason
  AC-OFFICE-BACKPRESSURE-001.8 gives: a list maintained by hand is what fell behind.
  Retry, the recovery sweep and routing re-dispatch are not enqueue paths: they re-queue
  an existing row rather than creating one, so they need no actor source and shall leave
  the persisted actor unchanged, on the same reasoning that exempts them from causation
  depth in AC-OFFICE-LAUNCH-SAFETY-003.7.
- **AC-OFFICE-RUN-CAUSATION-001.24:** When the carrier's creating run identifier is
  empty, the wake it produces shall be a root as defined by
  AC-OFFICE-RUN-CAUSATION-001.2: its causation identifier is its own run identifier,
  its parent run identifier is empty, and its causation depth is `0`. The creating run
  identifier alone decides this: when it is empty the wake is a root whatever the
  carried causation identifier and carried depth say, so the wake shall neither be
  queued at the carried depth plus `1` nor adopt a carried causation identifier. A
  carrier holding an empty creating run identifier beside a non-empty causation
  identifier is resolved by this criterion and not by
  AC-OFFICE-RUN-CAUSATION-001.10, because each value is individually well formed and
  nothing is malformed to report. The routine attribution, the human-rooted flag and the actor on
  the carrier still apply, so a routine that works through a task stays chargeable
  under AC-OFFICE-RUN-CAUSATION-001.14. This is the routine-fire case of
  AC-OFFICE-RUN-CAUSATION-001.5, where a root cause created the task and there is no
  creating run to be a parent. It differs from AC-OFFICE-RUN-CAUSATION-001.8, where a
  causing run exists but predates this capability, and from
  AC-OFFICE-RUN-CAUSATION-001.10, where a value is malformed rather than deliberately
  empty.

## Out of scope

- **Numeric limits on chain depth or launch rate.** This document defines the
  identity and how it propagates. What value of depth is too deep, and what happens
  when that value is exceeded, belong to
  [Office Unattended Launch Safety](unattended-launch-safety.md).
- **Which priority class an actor implies.** This document says what the actor is;
  [Office Launch Backpressure](launch-backpressure.md) decides what it is worth when
  capacity is scarce.
- **Backfill of historical rows.** Runs created before this capability carry the
  legacy empty identifier and are read as roots.
  AC-OFFICE-RUN-CAUSATION-001.6 is the reader contract; no migration pass rewrites
  them.
- **Retention of causation records.** How long run rows survive is a separate
  contract; chain reconstruction is only possible for runs still present.
  AC-OFFICE-RUN-CAUSATION-001.13 exists so that the one consumer whose correctness
  would otherwise depend on the root row surviving does not depend on it.
- **Cross-workspace chains.** Every enqueue path in scope is workspace-scoped
  already, so a chain cannot span workspaces. Nothing here creates that ability.
