---
name: spec-driven-development
description: "Single planner entrypoint for Kandev spec-driven implementation. Use when developing a feature or behavior-changing fix through the full flow: clarify intent, create/update specs, plan substantial delegated implementation, verify, and report progress."
---

# Spec-Driven Development

Use this as the default development workflow for non-trivial features and behavior-changing fixes. It is an orchestration skill: load referenced skills as phase guides when needed, but the user only needs to invoke this one.

## Planner And Workers

The user-started primary session owns architecture, planning, integration
judgment, delegation, and user
communication. Large feature-plan implementation stays worker-driven; small
scoped work may be direct under `/planner-orchestration`. Workers do not spawn.

- **`architect`** - optional frontier-model design reviewer for unusually risky architecture; it is not the default planner.
- **`implementer`** - implements one assigned task with TDD in the current worktree or an assigned git worktree.
- **`test-engineer`** - designs or adds focused tests when coverage is unclear, a bug needs a Prove-It regression, or a task is test-heavy.
- **`qa`** - exceptional independent validation for unusually complex multi-component behavior or an important boundary lacking faithful tests.
- **`code-review`** - exceptional local frontier review; qualifying current-head PR AI evidence is the default semantic path for PR delivery.
- **`security-auditor`** - exceptional audit for high-impact new/changed security boundaries or concrete security concerns.
- **`spark-explorer`** - Codex-only opt-in read-only specialist for bounded call-path tracing and evidence gathering; never use it for architecture, security, final review, implementation, or edits.
- **`spark-implementer`** - Codex-only opt-in specialist for explicit localized low-risk UI or code edits; use the normal implementer for work outside its stop conditions.

If a required worker cannot be launched, stop and report the unavailable role.
Never execute that worker's phase in the planner session.

## Core Flow

```text
Intent -> Planner artifacts -> User approval -> Workers by wave -> Risk-routed evidence -> Commit -> Verify -> Report
```

Do not skip from intent to code unless the user explicitly asks to bypass the process.

When this skill is invoked by name, treat every phase as mandatory even if the
requested change looks small. Do not replace the spec, plan, or task files with
inline notes in chat. If the correct durable artifact is unclear, stop and ask
where to record it instead of coding.

Before implementation begins, pause at the user-approval checkpoint with:
- Spec path and a short summary of the intended behavior.
- Plan path and the planned file/test touch points.
- Task files grouped by waves, including the worker role and model tier for each.
- Exact verification commands.

Only proceed past that checkpoint after the user approves, or after the user
explicitly says to skip SDD artifacts / approval for this task.

## Phase 0: Pipeline

Create a visible task list for this workflow:

1. Clarify intent
2. Create or update spec
3. Create implementation plan
4. Decompose into independent task files and waves
5. Execute tasks with TDD
6. Integrate and verify
7. Review, record, and summarize

Mark each phase in progress/completed as you go.

## Phase 1: Clarify Intent

The planner owns phases 1-4. Use `/interview-me` only if the request is
underspecified. Prefer the active harness's native user-question UI when it can
ask 2-4 focused questions together. Use the `architect` only for a bounded
second opinion when the design has unusually high architectural risk.

Exit criteria:
- Outcome, user, success criteria, constraints, and out-of-scope are clear.
- Ambiguities that affect behavior, data, permissions, or API contracts are resolved or explicitly accepted as open questions.

## Phase 2: Spec

The planner uses `/spec` to create or update the product spec under `docs/specs/`.

For bug fixes:
- If the fix only restores intended behavior, use `/fix` and regression tests instead of a feature spec.
- If the fix changes observable behavior, public contracts, permissions, persistence, or documented scenarios, update the relevant spec or create one if the product surface has none.

Spec exit criteria:
- `Why`, `What`, `Scenarios`, and `Out of scope` are complete.
- Data model, API surface, state machine, permissions, failure modes, and persistence guarantees are included when relevant.
- Success criteria are measurable or observable.
- User has approved the spec, or explicitly told you to continue with named open questions.
- The spec is recorded in `docs/specs/<slug>/spec.md` unless the user explicitly
  chooses another durable location. Chat-only specs do not satisfy SDD.

## Phase 3: Plan

The planner uses `/plan` to create `docs/plans/<slug>/plan.md`.

The plan must include:
- Exact files likely touched.
- Backend, frontend, tests, and E2E sections when applicable.
- Dependency order.
- Verification commands for each area.
- Risks and open questions.
- A saved `docs/plans/<slug>/plan.md` path. Chat-only plans do not satisfy SDD.
- Links to each sibling task file under `docs/plans/<slug>/`.

Prefer vertical slices that leave the product working after each wave. Avoid broad horizontal plans where no behavior can be verified until the end.

## Phase 4: Independent Tasks

Convert the plan into individual implementation task files next to `plan.md`,
under `docs/plans/<slug>/`. Each task must be independently executable by a
different agent when possible. Do not put full task bodies only inside
`plan.md`: the plan should link to sibling task files so implementers can load
just the task they need.

Task file naming:
- Use `docs/plans/<slug>/task-<NN>-<short-slug>.md`, e.g.
  `docs/plans/<slug>/task-01-backend-contracts.md`.
- Start each file with frontmatter: `id`, `title`, `status`, `wave`,
  `depends_on`, and `plan`.
- Initial `status` is `pending`; the implementing worker updates only its
  owned task file to `in_progress` when starting and `done` when finished.
- `plan.md` must reference every task file and show its current status. The
  planner updates those statuses after accepting worker results, or delegates
  a serialized update to a worker with explicit shared-plan ownership.

Each task needs:
- **Title:** one behavior or layer, no "and" unless inseparable.
- **Acceptance:** 1-3 concrete, testable conditions.
- **Verification:** exact targeted command(s).
- **Files:** specific paths, not broad directories.
- **Inputs:** spec section, plan section, relevant patterns, and dependencies.
- **Output contract:** compact handoff capsule: intent/acceptance; base/head SHA when applicable; changed files and entry points; named spec/ADR sections; risk tags; exact targeted commands/results; uncertainties.
- **Dependencies:** task IDs that must complete first, or `None`.

Independence test:
- Can an agent start with only this task, the spec/plan excerpts, and named files?
- Can it verify its own work without another task's unmerged changes?
- Does it avoid touching files another parallel task needs to edit?

If any answer is no, split the task or put it in a later sequential wave.

The planner must review its task files and wave graph before implementation starts. Do not fan out implementers from an unreviewed plan.

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
- Frontend tasks are usually sequential because Vite/React SPA build, type, and state surfaces overlap.
- Database migrations, generated API types, shared DTOs, and package-wide config are sequential.
- Parallel workers update only their owned task files. Never let them update
  `plan.md` concurrently; serialize plan-status updates even when shared-plan
  ownership is explicitly assigned.
- E2E runs happen after backend/frontend integration is coherent.

### Worktrees

If the active harness's native subagent tools and git worktrees are available,
the planner may request one worktree per independent task. Worktree creation,
branch operations, and integration are delegated when their isolation benefit
is material; the planner may handle a small routine Git step directly.

The assigned setup worker may use:

```bash
git worktree add ../kandev-task-<short-name> -b task/<short-name> HEAD
```

Rules:
- Do not create a worktree from a dirty parent state unless the task explicitly depends on those local changes.
- The planner gives each subagent its worktree path, branch name, task acceptance criteria, and verification command.
- Assign merging or cherry-picking completed task branches to an implementer in dependency order.
- If worktrees are unavailable or risky, run tasks sequentially in the current worktree.

## Phase 6: Implementation

For each task:
- Update only the owned task file frontmatter to `status: in_progress` before
  coding.
- Use `/tdd` for code changes.
- Use `/e2e` for browser/user-flow coverage.
- Use `/mobile-parity` for frontend UI changes.
- Use `/debug` when diagnosis or instrumentation is needed; remove temporary logs before PR.
- Update the task file frontmatter to `status: done` after the task's
  acceptance criteria and targeted verification pass. Do not update
  `plan.md` unless the work packet explicitly owns that shared file and the
  update is serialized outside parallel execution.

Assign every independent task to the normal `implementer` worker by default.
Codex may use `spark-implementer` only for an explicit, localized low-risk UI
or code edit when none of its documented stop conditions apply; if there is
doubt, use the normal `implementer`. Keep the same file-ownership, wave, TDD,
and output rules for either role. Launch workers in parallel only for tasks in
the same wave that do not share mutable files. Use this prompt shape:

```text
Task: <title>
Spec: <file + relevant scenarios>
Plan: <plan path + linked task file>
Acceptance:
- ...
Verification:
- ...
Files/patterns:
- ...
Constraints:
- Follow scoped AGENTS.md.
- Use TDD. Do not broaden scope.
- Update only the assigned task file; do not edit `plan.md` without explicit,
  serialized shared-plan ownership.
Output:
- Compact handoff capsule (intent/acceptance; base/head SHA when applicable;
  changed files/entry points; named spec/ADR sections; risk tags; exact
  targeted commands/results; uncertainties) and task file status update.
```

The planner coordinates waves and keeps progress state. Delegate substantial
conflict resolution, integration, and follow-up fixes; handle only small scoped
ones directly under `/planner-orchestration`.

## Phase 7: Integrate And Verify

Per-task versus per-wave checks are distinct:

- Each assigned `implementer` or `spark-implementer` owns and runs its task's
  targeted verification. The planner checks the reported evidence; it does not
  duplicate those tests after every wave.
- Derive risk tags from worker reports plus planner inspection and apply the
  `/planner-orchestration` evidence contract. Ordinary integration with
  faithful tests does not automatically require `qa`.

After each wave:
- Delegate conflict resolution and rerun affected tests through an implementer.
- Update the plan if the task files or wave graph changed.

At the end:
- Run the `test-engineer` worker when coverage is disputed, missing, or hard to place at the right test level.
- Run the `simplify` worker if implementation grew speculative abstractions.
- For PR delivery, defer routine semantic review until one qualifying
  current-head PR AI review; use local `code-review` only for the central
  exceptional routes.
- Run `qa` and `security-auditor` only for their central exceptional routes.
- Commit through active hooks, then run mandatory final change-aware `verify`
  before push; pass the hook receipt and use full mode when triggered.
- Use `/record` for ADR/spec updates if implementation discovered a durable decision or behavior change.

When PR delivery is in scope, PR AI review may be deferred, but do not claim
readiness until qualifying current-head semantic-review evidence exists.

Route every finding to a new bounded implementer packet. For a small,
scope-preserving PR remediation, use the finding, focused regression, final
Spark `verify`, and fresh qualifying exact-head PR AI review; do not relaunch
local gates unless the fix meets a central exceptional route. Reuse the same
native thread when role, change, and file scope remain materially the same;
otherwise launch a fresh worker for independent judgment or stale/noisy context.

## Stop Conditions

Stop and ask the user when:
- Spec and codebase disagree on behavior or ownership.
- A task cannot be made independent without changing scope.
- A fix requires a new architecture, dependency, data model, permission rule, or public contract not covered by the spec.
- The same verification failure repeats after three focused attempts.
- A required worker role cannot be launched or cannot access its assigned worktree.

## Final Report

Report:
- Spec path and plan path.
- Task waves completed and any tasks left.
- Subagent/worktree branches used, if any.
- Tests and verification commands run.
- Semantic-review evidence, risk tags, and QA decision/reason.
- Known pending checks or risks.
