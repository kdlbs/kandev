---
status: active
system: executors
created: 2026-11-08
owners:
  - kandev
---

# Prepared launch actor requirements

## Overview

[Issue #5181](https://github.com/kdlbs/kandev/issues/5181) reports an
admission gap in the following launch:

- the session is CREATED;
- its environment was prepared earlier;
- the start runs with no caller identity, and the session has no durable
  executor actor.

That launch reaches the agent runtime through `attemptColdResume` →
`startAgentOnPreparedWorkspace` with an unscoped context. Unscoped secret
resolution can then reveal values that belong to any user. This requirement
specializes
[REQ-EXECUTORS-OWNERSHIP-004](per-user-ownership.md#req-executors-ownership-004-authorized-compute-admission)
for that prepared-workspace start.

## Terms

- **Prepared-workspace start:** An internal start that launches the agent on a
  session whose environment already exists, on behalf of the session's
  recorded executor actor.
- **Durable executor actor:** The user ID recorded on a session as accountable
  for its executor launches.

## Requirements

### REQ-EXECUTORS-PREPARED-LAUNCH-ACTOR-001: Prepared-workspace start requires an actor

**Intent:** A prepared-workspace start runs only as an authorized identity,
never unscoped.

#### Acceptance criteria

- **AC-EXECUTORS-PREPARED-LAUNCH-ACTOR-001.1:** When executor admission
  establishes no launch identity for a prepared-workspace start, the system
  shall refuse the start. The refusal shall happen before agent process start,
  secret resolution, or prompt delivery, and shall return an actionable error.
  This covers a missing caller with no durable actor, and a durable actor that
  is deleted, disabled, or otherwise inaccessible.
- **AC-EXECUTORS-PREPARED-LAUNCH-ACTOR-001.2:** When an authorized caller
  starts the session, the system shall launch as that caller, taking precedence
  over the durable actor. When the caller is unauthorized, the system shall
  deny the start and shall not fall back to the durable actor.
- **AC-EXECUTORS-PREPARED-LAUNCH-ACTOR-001.3:** When the durable actor is a
  valid owner or collaborator, the system shall launch as that actor. That
  actor's private profile secrets shall never be resolved for any other
  identity.
- **AC-EXECUTORS-PREPARED-LAUNCH-ACTOR-001.4:** When authentication is
  disabled, the system shall launch with the explicit synthetic identity as
  before.
- **AC-EXECUTORS-PREPARED-LAUNCH-ACTOR-001.5:** A cancelled start shall launch
  nothing. Concurrent starts of the same session shall launch at most once,
  and concurrent unattributed starts shall all be refused.
- **AC-EXECUTORS-PREPARED-LAUNCH-ACTOR-001.6:** The prepared-workspace start
  shall resolve identity through the same executor admission as other launch
  paths: the same caller precedence, the same durable-actor lookup, and the
  same account-state denials. It shall add no separate actor policy that the
  cold `ResumeSession` path could disagree with.

## Exclusions

- **Other launch paths.** Unattributed fresh starts and passive reattachment
  keep their existing contracts. This requirement does not change how actors
  are recorded.
- **Cold resume.** The separate cold `ResumeSession` admission gap is
  addressed by draft PR
  [#5176](https://github.com/kdlbs/kandev/pull/5176), not here. Requiring a
  launch actor on that branch is owned by that change.

## Delivery

[Implementation plan](../../../plans/shared-profile-account-boundaries/plan.md).
