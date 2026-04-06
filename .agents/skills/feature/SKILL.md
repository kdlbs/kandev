---
name: feature
description: Guided feature development — brainstorm, explore codebase, design architecture, implement with TDD, and review. Use for new features or significant changes.
---

# Feature Development

Systematic feature development: understand the problem, explore the codebase, design the solution, implement with TDD, verify it works, and review.

## Available skills and subagents

The following skills and subagents are available in this repo to delegate work to. Use them instead of doing everything inline — they have specialized instructions and keep your context clean:

- **`/tdd`** — Implement changes using Test-Driven Development (Red-Green-Refactor). Delegate implementation tasks to this.
- **`/e2e`** — Write and run Playwright E2E tests using TDD. Use for the final wave when the full feature needs end-to-end coverage.
- **`qa` subagent** — Verify a feature works and review code quality. Traces wiring, tests edge cases, checks for security and architecture issues.
- **`verify` subagent** — Run fmt, typecheck, test, and lint across the monorepo, then fix any issues found.
- **`simplify` subagent** — Simplify recently changed code — inline one-off abstractions, remove speculative code, reduce nesting.
- **`/record`** — Record architectural decisions or save implementation plans for future reference.

---

## Before anything else: create the pipeline

The first thing you do — before reading code, before asking questions, before any exploration — is create a task list for the full pipeline. This is non-negotiable because it keeps you accountable to the process and lets the user see where you are.

Create these tasks immediately (use your task/todo tracking tool if available):

1. **Understand the problem** — Clarify requirements, identify constraints, confirm understanding with user
2. **Explore the codebase** — Find similar patterns, relevant architecture, integration points
3. **Design the solution** — Propose approaches with trade-offs, get user approval before implementing
4. **Implement with TDD** — Break into waves, implement test-first, delegate where possible
5. **QA and review** — Verify the feature works, review code quality, simplify
6. **Record** — Save any architectural decisions or insights for future sessions

Then start with task 1. Mark each task in_progress when you begin it and completed when you finish it. Do not skip ahead — each phase produces context that the next phase needs. Designing without exploring leads to solutions that fight the codebase. Implementing without design approval wastes time on the wrong approach.

---

## Phase 1: Understand the problem

Mark task 1 as in_progress.

1. If the request is vague, ask clarifying questions — one at a time, prefer multiple choice
2. **Challenge the premise** — before accepting the feature at face value, ask whether this is the right solution to the underlying problem. For example: "Have you considered X instead?" or "Is the real need Y rather than Z?" This is especially important for vague requests where the user may not have thought through alternatives. Even for clear requests, briefly consider whether a simpler approach exists.
3. Identify: what problem is being solved, who benefits, what are the constraints
4. Summarize understanding and confirm with the user

Mark task 1 as completed.

---

## Phase 2: Explore the codebase

Mark task 2 as in_progress.

1. Search the codebase in parallel targeting different aspects:
   - Similar features and their implementation patterns
   - Architecture and abstractions in the relevant area
   - Integration points, data flow, and extension points
2. Read all key files identified
3. Check `docs/decisions/INDEX.md` for relevant architectural decisions in this area
4. Present a summary of existing patterns and conventions to reuse

Mark task 2 as completed.

---

## Phase 3: Design

Mark task 3 as in_progress.

1. **Scope check:** If the feature spans multiple independent subsystems, break it into sub-features first. Each should produce working, testable software on its own.
2. Identify remaining ambiguities: edge cases, error handling, scope boundaries, backward compatibility
3. Ask the user to resolve them — do not assume
4. Propose 2-3 approaches with trade-offs and your recommendation:
   - Minimal change (smallest diff, maximum reuse)
   - Clean architecture (maintainability, elegant abstractions)
   - Pragmatic balance (speed + quality)
5. **Map the file structure:** List which files will be created or modified and what each is responsible for. This locks in decomposition decisions.
6. **Stop and wait for explicit user approval on the approach before proceeding.** Do not start Phase 4 until the user confirms the design.

Mark task 3 as completed only after the user approves.

---

## Phase 4: Implement with TDD

Mark task 4 as in_progress.

**You are the coordinator.** Your job is to hold the big picture (requirements, design, file structure, wave progress) and delegate implementation. Protect your context:

- **Delegate over inline work.** Every task you implement inline fills your context with code details and tool outputs. Delegate to sub-tasks whenever possible — they get fresh context and return only a summary.
- **Keep coupled tasks small.** If you must implement inline (coupled tasks), keep each task focused and short. Don't read entire files unnecessarily — read only what you need to verify work or wire things together.
- **Don't debug in the coordinator.** If a task fails quality gates, dispatch a new sub-task to fix it rather than debugging inline. Pass the failure details and let the fresh context investigate.
- **Track progress, not details.** For each completed task, note: what was done, which files changed, commit hash. Don't carry the implementation details forward.

### 4a. Decompose into implementation tasks

Using the file structure map from Phase 3, create sub-tasks for each discrete piece of work. Each task should:
- Touch a specific set of files
- Have a clear done condition (test passes, API works, component renders)
- Be classifiable as independent or coupled

### 4b. Group into waves

Parallelism is constrained by build boundaries:
- **Backend (Go)**: packages compile independently — different packages can run in parallel
- **Frontend (Next.js)**: single build — only ONE task can work on frontend at a time
- **E2E tests**: need full build — only after both backend and frontend changes are done
- **Coupled tasks** (shared types, sequential data flow): must run sequentially regardless

Group tasks into waves. Example:
```text
Wave 1 (parallel): [Backend API handler, Frontend component + hook]
Wave 2 (sequential): [Wire frontend to API, integration test]
Wave 3: [E2E test for the full flow]
```

For small features with 1-3 tasks, skip wave grouping and implement sequentially.

### 4c. Execute wave by wave

For each wave, follow TDD strictly (RED-GREEN-REFACTOR):

**Independent tasks in the wave**: delegate each task via `/tdd`. Each gets:
- Task description and acceptance criteria
- Relevant file paths from Phase 3
- Codebase conventions from Phase 2

**Coupled tasks** (e.g., wiring frontend to backend, integration glue): implement inline but keep it minimal — only the glue code, not full feature implementation.

Wait for all tasks in the wave to complete before moving to the next wave.

### 4d. Quality gate after each wave

After each wave completes:
- Backend: `cd apps/backend && go test ./internal/path/...` (changed packages only)
- Frontend: `cd apps && pnpm --filter @kandev/web typecheck && pnpm --filter @kandev/web test`
- If tests fail, fix before proceeding to the next wave
- E2E tests only in the final wave (via `/e2e`) or Phase 5 (QA)

Mark each implementation sub-task as completed as it passes its quality gate.

### 4e. Stop conditions

- **Bugs or missing validation discovered during a task:** fix inline. If the bug surfaces after a wave completes (quality gate failure), dispatch a new task to fix it — don't debug in the coordinator.
- **Blocker (missing dependency, unclear requirement, test fails repeatedly):** stop and ask the user
- **Fix requires architectural change (new DB table, new service layer, switching libraries):** stop and ask — don't make structural decisions silently
- **3 failed fix attempts on the same issue:** stop, question the approach, ask the user

Mark task 4 as completed when all implementation sub-tasks pass their quality gates.

---

## Phase 5: QA and review

Mark task 5 as in_progress.

1. Delegate to the `qa` subagent to verify the feature works and review code quality. It will:
   - Trace the wiring (exports used, APIs called, data flows)
   - Test the happy path and try to break it (boundary values, error paths, concurrency)
   - Review changed code for quality, security, and architecture compliance
   - Write tests for any gaps found
2. Delegate to the `simplify` subagent to clean up the implementation
3. Delegate to the `verify` subagent to run fmt, typecheck, test, and lint
4. Fix any blockers, present suggestions to the user
5. Summarize: what was built, key decisions, files modified, suggested next steps

Mark task 5 as completed.

---

## Phase 6: Record

Mark task 6 as in_progress.

Check if any decisions or insights from this feature should be recorded for future sessions:

1. If significant architectural decisions were made, run `/record decision` to create an ADR
2. If the implementation plan has reusable patterns or non-obvious context, run `/record plan` to save it
3. If nothing worth recording, skip this phase

Mark task 6 as completed.
