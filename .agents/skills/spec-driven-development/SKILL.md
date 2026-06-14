---
name: spec-driven-development
description: Single entrypoint for Kandev spec-driven implementation. Use when developing a feature or behavior-changing fix through the full flow: clarify intent, create/update specs, create a plan, split into independent verifiable tasks, execute with subagents/worktrees when possible, use TDD, verify, and report progress.
---

# Spec-Driven Development

Use this as the default development workflow for non-trivial features and behavior-changing fixes. It is an orchestration skill: load referenced skills as phase guides when needed, but the user only needs to invoke this one.

## Subagents

Use these helpers when the runtime supports delegated subagents. The parent agent remains the orchestrator; subagents do not spawn other subagents.

- **`architect`** - creates/updates the spec, plan, independent task graph, waves, and acceptance/verification criteria.
- **`implementer`** - implements one assigned task with TDD in the current worktree or an assigned git worktree.
- **`test-engineer`** - designs or adds focused tests when coverage is unclear, a bug needs a Prove-It regression, or a task is test-heavy.
- **`qa`** - validates integrated waves against the spec/plan, checks wiring, tests edge cases, and reports readiness.
- **`security-auditor`** - reviews security-sensitive changes such as auth, workspace isolation, filesystem/process execution, integrations, webhooks, secrets, or agent/tool permissions.

If subagents are unavailable, execute the same phases directly in the parent session.

## Core Flow

```text
Intent -> Architect -> Parent approval -> Implementers by wave -> QA -> Verify -> Report
```

Do not skip from intent to code unless the user explicitly asks to bypass the process.

## Phase 0: Pipeline

Create a visible task list for this workflow:

1. Clarify intent
2. Create or update spec
3. Create implementation plan
4. Decompose into independent tasks and waves
5. Execute tasks with TDD
6. Integrate and verify
7. Review, record, and summarize

Mark each phase in progress/completed as you go.

## Phase 1: Clarify Intent

Use the `architect` subagent for phases 1-4 when available. Otherwise use `/interview-me` only if the request is underspecified. Prefer `ask_user_question_kandev` when available so the user can answer 2-4 focused questions at once.

Exit criteria:
- Outcome, user, success criteria, constraints, and out-of-scope are clear.
- Ambiguities that affect behavior, data, permissions, or API contracts are resolved or explicitly accepted as open questions.

## Phase 2: Spec

Use the `architect` subagent when available; otherwise use `/spec` to create or update the product spec under `docs/specs/`.

For bug fixes:
- If the fix only restores intended behavior, use `/fix` and regression tests instead of a feature spec.
- If the fix changes observable behavior, public contracts, permissions, persistence, or documented scenarios, update the relevant spec or create one if the product surface has none.

Spec exit criteria:
- `Why`, `What`, `Scenarios`, and `Out of scope` are complete.
- Data model, API surface, state machine, permissions, failure modes, and persistence guarantees are included when relevant.
- Success criteria are measurable or observable.
- User has approved the spec, or explicitly told you to continue with named open questions.

## Phase 3: Plan

Use the `architect` subagent when available; otherwise use `/plan` to create `docs/specs/<slug>/plan.md`.

The plan must include:
- Exact files likely touched.
- Backend, frontend, tests, and E2E sections when applicable.
- Dependency order.
- Verification commands for each area.
- Risks and open questions.

Prefer vertical slices that leave the product working after each wave. Avoid broad horizontal plans where no behavior can be verified until the end.

## Phase 4: Independent Tasks

Convert the plan into implementation tasks. Each task must be independently executable by a different agent when possible.

Each task needs:
- **Title:** one behavior or layer, no "and" unless inseparable.
- **Acceptance:** 1-3 concrete, testable conditions.
- **Verification:** exact targeted command(s).
- **Files:** specific paths, not broad directories.
- **Inputs:** spec section, plan section, relevant patterns, and dependencies.
- **Output contract:** summary of changes, tests run, files touched, blockers, and follow-up risks.
- **Dependencies:** task IDs that must complete first, or `None`.

Independence test:
- Can an agent start with only this task, the spec/plan excerpts, and named files?
- Can it verify its own work without another task's unmerged changes?
- Does it avoid touching files another parallel task needs to edit?

If any answer is no, split the task or put it in a later sequential wave.

The parent agent must review and approve the architect's task graph before implementation starts. Do not fan out implementers from an unreviewed plan.

## Phase 5: Waves And Parallelism

Group tasks into waves by dependency and file ownership:

```text
Wave 1: independent backend foundations in separate packages
Wave 2: API/client contracts and shared wiring
Wave 3: frontend UI/state work
Wave 4: E2E, integration, QA, docs
```

Parallelize only when safe:
- Backend packages can often run in parallel if they do not edit the same files or migrations.
- Frontend tasks are usually sequential because Next.js build/types/state surfaces overlap.
- Database migrations, generated API types, shared DTOs, and package-wide config are sequential.
- E2E runs happen after backend/frontend integration is coherent.

### Worktrees

If multi-agent tools and git worktrees are available, prefer one worktree per independent task:

```bash
git worktree add ../kandev-task-<short-name> -b task/<short-name> HEAD
```

Rules:
- Do not create a worktree from a dirty parent state unless the task explicitly depends on those local changes.
- Give each subagent its worktree path, branch name, task acceptance criteria, and verification command.
- Merge or cherry-pick completed task branches back in dependency order.
- If worktrees are unavailable or risky, run tasks sequentially in the current worktree.

## Phase 6: Implementation

For each task:
- Use `/tdd` for code changes.
- Use `/e2e` for browser/user-flow coverage.
- Use `/mobile-parity` for frontend UI changes.
- Use `/debug` when diagnosis or instrumentation is needed; remove temporary logs before PR.

When available, assign each independent task to an `implementer` subagent. Launch implementers in parallel only for tasks in the same wave that do not share mutable files. Use this prompt shape:

```text
Task: <title>
Spec: <file + relevant scenarios>
Plan: <plan section>
Acceptance:
- ...
Verification:
- ...
Files/patterns:
- ...
Constraints:
- Follow scoped AGENTS.md.
- Use TDD. Do not broaden scope.
Output:
- Summary, files changed, tests run, blockers, risks.
```

The parent agent coordinates waves, resolves conflicts, merges/cherry-picks worktree branches, and keeps progress state. It should not debug every subtask inline unless delegation is unavailable.

## Phase 7: Integrate And Verify

After each wave:
- Run targeted tests for changed backend packages or frontend modules.
- Resolve conflicts and re-run affected tests.
- Update the plan if the task graph changed.

At the end:
- Run the `test-engineer` subagent when coverage is disputed, missing, or hard to place at the right test level.
- Run the `qa` subagent, or `/qa` directly, for integration and behavior verification.
- Run the `security-auditor` subagent for security-sensitive changes before declaring readiness.
- Run `/simplify` if implementation grew speculative abstractions.
- Run `/verify` for full format, typecheck, tests, and lint.
- Use `/record` for ADR/spec updates if implementation discovered a durable decision or behavior change.

## Stop Conditions

Stop and ask the user when:
- Spec and codebase disagree on behavior or ownership.
- A task cannot be made independent without changing scope.
- A fix requires a new architecture, dependency, data model, permission rule, or public contract not covered by the spec.
- The same verification failure repeats after three focused attempts.

## Final Report

Report:
- Spec path and plan path.
- Task waves completed and any tasks left.
- Subagent/worktree branches used, if any.
- Tests and verification commands run.
- Known pending checks or risks.
