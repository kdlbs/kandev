---
name: plan
description: Create an implementation plan (plan.md) from a feature spec. Explores the codebase, designs the approach, and produces a structured plan with backend, frontend, tests, and E2E sections. Use after writing a spec and before implementing.
---

# Create Implementation Plan

Translate a feature spec into a concrete, phased implementation plan saved as `plan.md` alongside the spec.

## Input

- The feature spec (`docs/specs/<slug>/spec.md`) — read it first
- The codebase — explore relevant areas before designing

## Output

`docs/specs/<slug>/plan.md` — a structured plan the implementing agent can follow directly

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

### 3. Ask before designing (if needed)

If the spec leaves implementation choices open, ask — one question at a time. Do not assume. Examples of things to ask:
- Which table/model owns new data?
- Is a new API endpoint needed or does an existing one extend?
- Should this be behind a feature flag?

Stop asking when you have enough to write the plan.

### 4. Write plan.md

Save to `docs/specs/<slug>/plan.md`. Use this structure:

```markdown
---
spec: <slug>
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

Every plan MUST include this section. For each testable behavior in the spec, list:
- **What:** the behavior under test (maps to a spec scenario)
- **File:** where the test goes (`*_test.go` or `*.test.ts`)
- **How:** table-driven unit test / integration test with real DB / mock service

At minimum, include:
- One unit test per new function with non-trivial logic
- One integration test that exercises the full path (handler → service → repo)
- One test per edge case called out in the spec scenarios

---

## E2E Tests

> Skip this section only if the spec has zero user-visible UI changes.

For each user-facing scenario in the spec:
- **Scenario:** restate the GIVEN/WHEN/THEN from the spec
- **File:** `apps/web/e2e/<area>/<name>.spec.ts`
- **What to verify:** the observable outcome (URL change, element visible, toast shown)

---

## Implementation Waves

Group all work above into waves. Parallelism rules:
- Backend packages: can run in parallel
- Frontend (Next.js): single build — only one task at a time
- E2E: after all backend + frontend changes are done

```
Wave 1 (parallel): [backend task A, backend task B]
Wave 2: [frontend changes]
Wave 3: [E2E tests]
```

For small features (≤3 tasks total), waves are optional — list sequentially.

---

## Open Questions
(Delete when empty.)
```

### Style rules

- **Be specific.** Name exact file paths, function signatures, SQL column names. The implementing agent should not need to re-explore the codebase.
- **No speculation.** Only plan what the spec requires. Do not add "nice to have" items.
- **Tests are not optional.** Every plan must have a Tests section. E2E is required whenever there are UI changes.
- **Frontend is not optional.** If the spec has any user-visible behavior, the plan must have a Frontend section.
- **Keep it proportional.** A small spec gets a 1-page plan. A large spec may need 3-4 pages. Do not pad.
