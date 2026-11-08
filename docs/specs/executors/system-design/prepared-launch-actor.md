---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-PREPARED-LAUNCH-ACTOR-001
---

# Prepared launch actor system design

## Purpose and boundaries

This design closes the unscoped prepared-workspace start. It adds one
post-admission check to the existing executor launch and changes no identity
resolution. Caller precedence, durable-actor lookup, and account-state denials
remain in the executor admission that `backendapp.wireExecutorAdmission`
installs, as described in
[per-user executor ownership](per-user-ownership.md). The delivery record is
the [implementation plan](../../../plans/shared-profile-account-boundaries/plan.md).

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-EXECUTORS-PREPARED-LAUNCH-ACTOR-001 | Control flow; Launch option; Failure behavior |

## Control flow

```text
StartCreatedSession / attemptColdResume
  -> Service.startAgentOnPreparedWorkspace      (RequireExecutorActor: true)
  -> launchPreparedSessionWithDynamicFallback   (options carried unchanged)
  -> Executor.launchPreparedSession
       session lock acquired
       session re-read
       ctx = e.executorAdmission(ctx, session, executor, executor profile)
       requirePreparedLaunchActor(ctx, opts)    <- new
       worktree recovery, agent profile, runtime StartAgentProcess / LaunchAgent
```

The check runs under the per-session launch lock, after admission and before:

- worktree recovery;
- agent-profile resolution;
- any runtime call, and so any secret reveal or prompt delivery.

Concurrent and duplicate starts therefore keep the existing single-launch
serialization, and each one is refused independently when no actor exists.

## Launch option

`executor.LaunchOptions.RequireExecutorActor` is set only by
`startAgentOnPreparedWorkspace`. That is the internal start that acts on behalf
of the session's durable executor actor. Other launch paths keep their existing
contracts.

`requirePreparedLaunchActor` (`orchestrator/executor/prepared_actor.go`) reads
the admitted context:

| Admitted identity | Result |
| --- | --- |
| none | refuse |
| non-synthetic with empty user ID | refuse |
| non-synthetic user | continue as that user |
| synthetic (auth disabled) | continue as before |

Admission keeps these properties:

- An authorized caller wins over the durable actor.
- An unauthorized caller is denied inside admission, with no fallback to the
  durable actor.
- A deleted or disabled durable actor does not resolve, so the context stays
  identity-free and this check refuses the start.

## Failure behavior

The refusal is `executor.ErrPreparedLaunchActorUnavailable`. Its message tells
the operator to start the session as a user with access to the task, or to
restore the account that prepared it. The existing launch-failure handling
records the refusal on the session and contains no secret material. Context
cancellation before or during admission returns without any runtime call.

## Relationship to cold resume

The cold `ResumeSession` branch is a different entry point, and the separate
draft PR [#5176](https://github.com/kdlbs/kandev/pull/5176) addresses it. This
design depends only on the shared admission function, so both paths apply the
same caller and actor policy once both changes land.
