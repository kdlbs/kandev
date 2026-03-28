---
name: feature
description: Guided feature development — brainstorm, explore codebase, design architecture, implement with TDD, and review. Use for new features or significant changes.
---

# Feature Development

Systematic feature development: understand the problem, explore the codebase, design the solution, implement with TDD, and review.

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
3. Present a summary of existing patterns and conventions to reuse

## Phase 3: Design

1. Identify remaining ambiguities: edge cases, error handling, scope boundaries, backward compatibility
2. Ask the user to resolve them — do not assume
3. Propose 2-3 approaches with trade-offs and your recommendation:
   - Minimal change (smallest diff, maximum reuse)
   - Clean architecture (maintainability, elegant abstractions)
   - Pragmatic balance (speed + quality)
4. Get explicit user approval on the approach before proceeding

## Phase 4: Implement with TDD

Follow `/tdd` strictly:

1. For each behavior or component:
   - **RED** — write a failing test (unit, or E2E via `/e2e` for user-facing flows)
   - **GREEN** — write the minimum code to pass
   - **REFACTOR** — clean up while staying green
2. Follow codebase conventions from Phase 2
3. Build incrementally — one behavior at a time, tests passing at every step

## Phase 5: Review

1. Run `/verify` to ensure fmt, typecheck, test, and lint all pass
2. Run `/code-review` on the changes
3. Fix any blockers, present suggestions to the user
4. Summarize: what was built, key decisions, files modified, suggested next steps
