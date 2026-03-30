---
name: feature
description: Guided feature development — brainstorm, explore codebase, design architecture, implement with TDD, and review. Use for new features or significant changes.
---

# Feature Development

Systematic feature development: understand the problem, explore the codebase, design the solution, implement with TDD, verify it works, and review.

**No implementation before design approval.**

## Phase 1: Understand the problem

1. If the request is vague, ask clarifying questions — one at a time, prefer multiple choice
2. Challenge the premise: is this the right feature, or is the real need something different?
3. Identify: what problem is being solved, who benefits, what are the constraints
4. Summarize understanding and confirm with the user

## Phase 2: Explore the codebase

1. Launch 2-3 Explore agents in parallel targeting different aspects:
   - Similar features and their implementation patterns
   - Architecture and abstractions in the relevant area
   - Integration points, data flow, and extension points
2. Read all key files the agents identify
3. Check `docs/decisions/INDEX.md` for relevant architectural decisions in this area
4. Present a summary of existing patterns and conventions to reuse

## Phase 3: Design

1. **Scope check:** If the feature spans multiple independent subsystems, break it into sub-features first. Each should produce working, testable software on its own.
2. Identify remaining ambiguities: edge cases, error handling, scope boundaries, backward compatibility
3. Ask the user to resolve them — do not assume
4. Propose 2-3 approaches with trade-offs and your recommendation:
   - Minimal change (smallest diff, maximum reuse)
   - Clean architecture (maintainability, elegant abstractions)
   - Pragmatic balance (speed + quality)
5. **Map the file structure:** Before getting approval, list which files will be created or modified and what each is responsible for. This locks in decomposition decisions.
6. Get explicit user approval on the approach before proceeding

## Phase 4: Implement with TDD

**You are the coordinator.** Your job is to hold the big picture (requirements, design, file structure, wave progress) and delegate implementation to subagents. Protect your context:

- **Prefer subagents over inline work.** Every task you implement inline fills your context with code details and tool outputs. Delegate to subagents whenever possible — they get fresh context and return only a summary.
- **Keep coupled tasks small.** If you must implement inline (coupled tasks), keep each task focused and short. Don't read entire files unnecessarily — read only what you need to verify the subagent's work or wire things together.
- **Don't debug in the coordinator.** If a subagent's task fails quality gates, dispatch a new subagent to fix it rather than debugging inline. Pass the failure details and let the fresh subagent investigate.
- **Track progress, not details.** For each completed task, note: what was done, which files changed, commit hash. Don't carry the implementation details forward.

### 4a. Decompose into tasks

Using the file structure map from Phase 3, break the implementation into discrete tasks. Each task should:
- Touch a specific set of files
- Have a clear done condition (test passes, API works, component renders)
- Be classifiable as independent or coupled

### 4b. Group into waves

Subagents share the same worktree, so parallelism is constrained by build boundaries:
- **Backend (Go)**: packages compile independently — different packages can run in parallel subagents
- **Frontend (Next.js)**: single build — only ONE subagent can work on frontend at a time
- **E2E tests**: need full build — only after both backend and frontend changes are done
- **Coupled tasks** (shared types, sequential data flow): must run sequentially regardless

Group tasks into waves. Example:
```text
Wave 1 (parallel): [Backend API handler (subagent), Frontend component + hook (subagent)]
Wave 2 (sequential): [Wire frontend to API, integration test]
Wave 3: [E2E test for the full flow]
```

Max parallelism: 1 frontend subagent + 1-2 backend subagents (if working on independent packages). For small features with 1-3 tasks, skip wave grouping and implement sequentially.

### 4c. Execute wave by wave

For each wave, follow `/tdd` strictly (RED-GREEN-REFACTOR):

**Independent tasks in the wave**: dispatch one subagent per task. Each gets fresh context with:
- Task description and acceptance criteria
- Relevant file paths from Phase 3
- Codebase conventions from Phase 2
- Instruction to follow `/tdd`

**Coupled tasks** (e.g., wiring frontend to backend, integration glue): implement inline but keep it minimal — only the glue code, not full feature implementation.

Wait for all tasks in the wave to complete before moving to the next wave.

### 4d. Quality gate after each wave

After each wave completes:
- Backend: `cd apps/backend && go test ./internal/path/...` (changed packages only)
- Frontend: `cd apps && pnpm --filter @kandev/web typecheck && pnpm --filter @kandev/web test`
- If tests fail, fix before proceeding to the next wave
- E2E tests only in the final wave or Phase 5 (QA)

### 4e. Stop conditions

- **Bugs or missing validation discovered during a subagent's task:** the subagent fixes it inline. If the bug surfaces after a wave completes (quality gate failure), dispatch a new subagent to fix it — don't debug in the coordinator.
- **Blocker (missing dependency, unclear requirement, test fails repeatedly):** stop and ask the user
- **Fix requires architectural change (new DB table, new service layer, switching libraries):** stop and ask — don't make structural decisions silently
- **3 failed fix attempts on the same issue:** stop, question the approach, ask the user

## Phase 5: QA

Delegate to the **`qa` sub-agent** to verify the feature works end-to-end. It will:
- Trace the wiring (exports used, APIs called, data flows)
- Test the happy path
- Try to break it (boundary values, error paths, concurrency)
- Write tests for any gaps found

## Phase 6: Review

1. Delegate to the **`simplify` sub-agent** to clean up the implementation
2. Delegate to the **`verify` sub-agent** to ensure fmt, typecheck, test, and lint all pass
3. Delegate to the **`code-review` sub-agent** to review the changes
4. Fix any blockers, present suggestions to the user
5. Summarize: what was built, key decisions, files modified, suggested next steps
6. If significant architectural decisions were made, record them via `/record decision`
7. Save the feature design to `docs/plans/` via `/record plan` for permanent reference
