---
name: fix
description: Diagnose a bug, update its behavioral spec, create reviewable fix plan and task files, then hand off for explicit implementation with TDD.
---

# Fix

Use the same durable, reviewable workflow as feature work. Diagnose first, then
produce the spec amendment, fix plan, and task files before changing production
code. The user reviews those artifacts, switches the main session if desired,
and sends a later explicit implementation request. Native subagents still
require explicit authorization.

## Core Flow

```text
Evidence -> Root cause -> Spec amendment -> Fix plan + tasks -> Design-package handoff -> Explicit implementation request -> TDD implementation -> PR AI review
```

Do not patch production code before the planning checkpoint unless the user
explicitly opts out of the workflow.

## Phase 0: Evidence And Root Cause

When a bug originates from an issue tracker, read the canonical issue and every
image attachment before hypothesizing. Reproduce with existing tests, a
read-only trace, or a minimal throwaway repro. Do not add production code during
this phase.

State the root cause before planning:

- what is actually wrong, not only the symptom;
- why it happens and under which conditions;
- the smallest reliable reproduction; and
- the intended regression-test level and path.

If the problem cannot be reproduced or the cause remains unclear, stop and ask
the user rather than guessing at a fix.

## Phase 1: Fix Specification

Before implementation, find the affected durable spec under
`docs/specs/<slug>/spec.md` and amend its behavior or GIVEN/WHEN/THEN scenario
to cover the regression. If no relevant spec exists, create a concise repair
spec at `docs/specs/<fix-slug>/spec.md` that states the broken behavior, desired
behavior, regression scenario, out-of-scope work, and any relevant contract or
persistence constraint.

This spec is the reviewable expected behavior; it is not an incident report.

## Phase 2: Fix Plan And Task Files

Create `docs/plans/<fix-slug>/plan.md` and sibling
`docs/plans/<fix-slug>/task-<NN>-<short-slug>.md` files before implementation.
Follow the `/plan` structure, with these fix-specific requirements:

- Link the amended repair spec and state the confirmed root cause.
- Include the regression test that must fail before the code change and pass
  afterward.
- Name exact files, dependencies, acceptance criteria, and targeted commands.
- Use dependency waves and mark `parallel-safe` only for disjoint tasks with no
  shared schema, migration, generated contract, lockfile, or package config.
- Keep `parallelism: sequential` by default in each task; waves are a human
  decision aid, not authorization to delegate.

## Phase 3: Design-Package Handoff

Before changing production or permanent test code, present:

- root-cause summary and reproduction evidence;
- amended/new spec path;
- fix plan and task-file paths;
- task waves, parallel candidates, and exact validation commands; and
- risks and out-of-scope work.

End the turn after this handoff. Do not call `ask_user_question_kandev` (or an
equivalent approval prompt) to ask the user to approve the package or switch
models. The user reviews the files, switches the main session if desired, and
sends a later explicit implementation request. The files may remain
`draft`/`pending`; do not wait for a separate approval marker. Do not infer
subagent authorization from the plan.

## Phase 4: Implement With TDD After Handoff

After the user explicitly asks to implement, execute the tasks sequentially by
default:

1. Mark the task `in_progress`.
2. Write and run the regression test; confirm it fails for the expected reason.
3. Implement the minimal fix.
4. Run the task's exact targeted unit, integration, or E2E command.
5. Mark the task `done` and update `plan.md` status in the primary session.

When the user explicitly authorizes subagents, follow
`/planner-orchestration`: native delegation only, current user-selected model,
`fork_turns: "none"`, compact task-file handoffs, no recursive spawning, and
runtime usage-metadata confirmation. Only launch tasks marked parallel-safe.

## Phase 5: PR Review

After all task checks pass, commit, push, and open the PR. Do not run
automatic local simplify, QA, code/security review, or broad verification. The
two configured PR AI reviewers are the semantic-review gate. Use `/pr-fixup`
only for CI failures or actionable reviewer findings, rerunning only the
affected task check after remediation.

## Stop Conditions

Stop and ask the user when the root cause is uncertain, the repair changes an
architecture/public-contract/persistence/security boundary, the spec and code
disagree, or the same targeted check fails three times.

## Final Report

Report the root cause, spec/plan/task paths, design-package handoff,
subagents explicitly authorized (if any), changed files, tests run, and current
PR-review status.
