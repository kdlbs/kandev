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

Follow `/tdd` strictly. Decide how to execute based on task independence:

**Independent tasks** (separate files/packages, no shared state): dispatch a subagent per task. Each gets a fresh context with the task description, relevant file paths from Phase 3, and codebase conventions from Phase 2. This prevents context pollution and keeps each task focused.

**Coupled tasks** (shared types, sequential data flow, integration work): implement inline in the current session to maintain shared context.

For each behavior or component:
1. **RED** — write a failing test (unit, or E2E via `/e2e` for user-facing flows)
2. **GREEN** — write the minimum code to pass
3. **REFACTOR** — clean up while staying green
4. Follow codebase conventions from Phase 2
5. Build incrementally — one behavior at a time, tests passing at every step

**When things go wrong during implementation:**
- **Bugs or missing validation discovered:** fix inline, add a test, continue
- **Blocker (missing dependency, unclear requirement, test fails repeatedly):** stop and ask the user
- **Fix requires architectural change (new DB table, new service layer, switching libraries):** stop and ask — don't make structural decisions silently
- **3 failed fix attempts on the same issue:** stop, question the approach, ask the user

## Phase 5: QA

Run `/qa` as a subagent (keeps the main context clean) to verify the feature works end-to-end:
- Trace the wiring (exports used, APIs called, data flows)
- Test the happy path
- Try to break it (boundary values, error paths, concurrency)
- Write tests for any gaps found

## Phase 6: Review

1. Run `/simplify` to clean up the implementation
2. Run `/verify` to ensure fmt, typecheck, test, and lint all pass
3. Run `/code-review` as a subagent on the changes
4. Fix any blockers, present suggestions to the user
5. Summarize: what was built, key decisions, files modified, suggested next steps
6. If significant architectural decisions were made, record them via `/record decision`
7. Save the feature design to `docs/plans/` via `/record plan` for permanent reference
