---
name: plan
description: Create a committed implementation plan from a feature spec. Explores the codebase, designs the approach, and produces docs/plans/<feature>/plan.md plus individual task files. Use after writing a spec and before implementing.
---

# Create Implementation Plan

This is a primary-session artifact skill. The user-started conversation creates
the plan and task files, then returns control with a handoff. The user reviews
the files, switches that same conversation to an implementation model if
desired, and sends the explicit request to execute them.

Translate a feature spec into a concrete, phased implementation plan saved under
`docs/plans/<feature>/`. Plans and task files are committed implementation
records for the current buildout; specs remain the durable requirements under
`docs/specs/`.

## Input

- The feature spec (`docs/specs/<slug>/spec.md`) — read it first
- The codebase — explore relevant areas before designing

## Output

- `docs/plans/<slug>/plan.md` — a structured plan that links back to the spec
  and references every task file
- `docs/plans/<slug>/task-<NN>-<short-slug>.md` — one independently executable
  implementation task per file

---

## Steps

### 1. Read the spec

Read `docs/specs/<slug>/spec.md` in full. Identify:
- The observable behaviors (What section)
- The scenarios — each is a potential test case
- Any out-of-scope items (don't plan for these)

### 2. Explore the codebase

Search in parallel for all integration points the spec touches:
- Relevant models, repos, services, handlers
- Similar existing features to reuse as patterns
- Frontend state slices, hooks, and components in the area
- Existing tests in the area (to understand the testing patterns)

Use `docs/decisions/INDEX.md` to check for relevant architectural decisions.

Map dependencies before writing tasks. Implementation order follows the dependency chain: persistence/contracts first, service behavior next, API/client wiring after that, then UI and E2E. Prefer vertical slices that leave the product working over broad horizontal layers that cannot be verified until the end.

### 3. Ask before designing (if needed)

If the spec leaves implementation choices open, ask — one question at a time. Do not assume. Examples of things to ask:
- Which table/model owns new data?
- Is a new API endpoint needed or does an existing one extend?
- Should this be behind a feature flag?

Stop asking when you have enough to write the plan.

### 4. Write plan.md

Save to `docs/plans/<slug>/plan.md`. Use this structure:

```markdown
---
spec: docs/specs/<slug>/spec.md
created: YYYY-MM-DD
status: draft
---

# Implementation Plan: <Feature Name>

## Overview
2-4 sentences. What changes, in what order, and why that order.

---

## Backend

### <Area 1 — e.g., Schema Changes>
For each change: file path, exact struct/function/SQL, reason.

### <Area 2 — e.g., Service Layer>
...

### <Area N>
...

---

## Frontend

> Skip this section if the spec has no user-facing changes.

### <Component / Page>
File path, what changes, why.

### API client
What new calls are needed and where they go.

### State
Store slice / hook changes.

---

## Tests

Every plan MUST include this section. For each testable behavior in the spec,
list the exact pre-PR validation:
- **What:** the behavior under test (maps to a spec scenario)
- **File:** where the test goes (`*_test.go` or `*.test.ts`)
- **How:** table-driven unit test / integration test with real DB / mock service

At minimum, include:
- One unit test per new function with non-trivial logic
- One integration test that exercises the full path (handler → service → repo)
- One test per edge case called out in the spec scenarios

Do not add a generic local QA, review, security, simplify, or full-verification
step to the plan. The listed task checks are the pre-PR evidence; the two PR AI
reviewers perform semantic review after the PR opens.

---

## E2E Tests

> Skip this section only if the spec has zero user-visible UI changes.

For each user-facing scenario in the spec:
- **Scenario:** restate the GIVEN/WHEN/THEN from the spec
- **File:** `apps/web/e2e/<area>/<name>.spec.ts`
- **What to verify:** the observable outcome (URL change, element visible, toast shown)

---

## Verification Results
Pending. On completion, synchronize this section with each task's `## Results`:
record exact commands and outcomes/counts, generated artifact paths, and
cleanup/teardown evidence.

---

## Implementation Waves And Parallel Candidates

Group task files by dependency order. Use waves to expose possible parallelism,
but label a task as parallel-safe only when its files are disjoint and it does
not touch shared schemas, migrations, generated contracts, lockfiles, or
package-wide configuration. E2E follows the backend and frontend changes it
covers.

The default is sequential execution in the primary conversation. Waves do not
authorize subagents: only the user may explicitly ask to use them after
selecting the implementation model.

```
Wave 1 (parallel candidates — user authorization required):
- [ ] [task-01-backend-contracts](task-01-backend-contracts.md)
- [ ] [task-02-backend-repository](task-02-backend-repository.md)

Wave 2:
- [ ] [task-03-frontend-ui](task-03-frontend-ui.md)

Wave 3:
- [ ] [task-04-e2e](task-04-e2e.md)
```

For small features (≤3 tasks total), waves are optional — list sequentially.

The plan links to task files; it does not contain full task bodies. Update the
checkbox/status link when a task is completed.

---

## Open Questions
(Delete when empty.)
```

### 5. Write task files

Create one task file beside `plan.md` per task, named
`docs/plans/<slug>/task-<NN>-<short-slug>.md`. Use this structure:

```markdown
---
id: "01-backend-contracts"
title: "Backend contracts"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/<slug>/spec.md"
---

# Task 01: Backend contracts

Each task should be small enough for one focused implementation pass:
- **Acceptance:** 1-3 concrete conditions.
- **Verification:** exact command(s), e.g. `cd apps/backend && go test -run TestName ./internal/path/...` or `cd apps && pnpm --filter @kandev/web test -- path/to/file.test.ts`. Frontend/E2E tasks must include the fresh-worktree bootstrap (`cd apps && pnpm install --frozen-lockfile`) when dependencies may be absent; direct web typechecking uses `cd apps/web && pnpm run typecheck`, while other workspace package commands use the documented `pnpm --filter` form. Backend commands should use the applicable repository `make` target when one exists.
- **Files likely touched:** specific paths, not broad directories.
- **Dependencies:** task numbers that must land first, or `None`.
- **Parallelism:** `sequential` by default; set `parallel-safe` only with named
  disjoint files and no shared-state blocker.
- **Inputs:** relevant spec sections, plan sections, patterns, and dependencies.
- **Output contract:** summary, files changed, tests run, blockers, risks, and
  task/plan status update in the same conversation.

## Results
Pending. Before marking the task done, replace this with every exact command
actually run and its outcome/count, generated artifact paths, and cleanup or
teardown evidence (including temporary capture-spec removal and
`git diff --check` when used). Record security/trust and external side-effect
boundaries when applicable, or explicitly state `None`.

Break a task down further if it touches unrelated subsystems, needs more than one focused session, or the title contains "and".

When an implementation agent starts the task, it must change `status` to
`in_progress`. Before it finishes, reconcile **Files likely touched** with the
actual diff, including modified existing tests used as E2E evidence. It may then
change `status` to `done`, update its `## Results`, and synchronize the
corresponding checkbox/status and `## Verification Results` in `plan.md`.
```

### 6. End the design turn

After `plan.md` and every task file are written and validated, report their
paths, dependency order, exact checks, and open risks as a compact handoff,
then end the turn. Do not call `ask_user_question_kandev` (or an equivalent
approval prompt) to ask the user to approve the plan or switch models. The user
reviews the artifacts and controls the next implementation request and model
choice.

### Style rules

- **Be specific.** Name exact file paths, function signatures, SQL column names. The implementing agent should not need to re-explore the codebase.
- **No speculation.** Only plan what the spec requires. Do not add "nice to have" items.
- **Tests are not optional.** Every plan must have a Tests section. E2E is required whenever there are UI changes.
- **Frontend is not optional.** If the spec has any user-visible behavior, the plan must have a Frontend section.
- **Keep it proportional.** A small spec gets a 1-page plan. A large spec may need 3-4 pages. Do not pad.
- **Keep task bodies out of the plan.** Put implementation details in individual
  task files and link to them from `plan.md`.
