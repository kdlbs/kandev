---
name: fix
description: Fix bugs and issues — reproduce, find root cause, minimal fix with regression test. Use when something is broken.
---

# Fix

Systematic bug fixing: reproduce the problem, find the root cause, apply a minimal fix with a regression test.

## Available skills and subagents

- **`/tdd`** — Use for implementing the fix with a regression test (Red-Green-Refactor).
- **`/e2e`** — Use when the bug is in a user-facing flow and needs a Playwright regression test.
- **`/verify`** — Run after fixing to ensure nothing else broke.
- **`/record`** — Record architectural decisions or insights discovered during the fix.

## What a fix produces (and what it doesn't)

The artifacts for a bug fix are:
- A regression test that fails before the fix and passes after.
- The minimal code change.
- A clear commit message capturing the root cause.
- **An ADR (via `/record decision`) IF the fix encoded a new project-wide convention** (e.g., "GC code must fail-closed", "all repo deletes must be transactional"). Most fixes don't need one.
- **An update to the related feature spec IF the bug exposed a requirement gap** (see Phase 5).

A bug fix does **not** produce a spec. Specs describe product features; bugs are corrections to existing features and are tracked in tests + commits.

---

## Before anything else: create the pipeline

Create these tasks immediately (use your task/todo tracking tool if available):

1. **Reproduce the bug** — Write a test or find a reliable reproduction case
2. **Find the root cause** — Trace the code path, narrow the scope, state the cause clearly
3. **Fix with TDD** — Minimal fix with regression test, no surrounding refactors
4. **Verify** — Run full verification, check for similar patterns elsewhere
5. **Record** — Save any architectural decisions or insights, AND update the related feature spec if the bug exposed a requirement gap

Then start with task 1. Mark each task in_progress when you begin it and completed when you finish it. Do not skip ahead — fixing without reproducing leads to patches that don't address the real problem. Fixing without understanding the root cause leads to whack-a-mole.

---

## Phase 1: Reproduce

Mark task 1 as in_progress.

Before anything else, reproduce the bug reliably. Pick the right method based on where the bug lives:

- **Backend** (API, logic, data): write a Go test that calls the function/endpoint and asserts the wrong behavior. Run with `go test -v -run TestName ./path/...`
- **Frontend** (UI, state, interaction): write a Playwright E2E test using `/e2e` that navigates to the page and triggers the bug. Run with `make test-e2e` to verify.
- **Full-stack** (user flow breaks end-to-end): Playwright E2E test that exercises the full path from UI through API to DB and back.
- **Unclear where it lives**: start by reading the code path from the reported symptom (a page, an error message, a wrong value) back to its source. Then write the test at the appropriate level.

If it can't be reproduced, add logging/assertions to gather more info — don't guess at a fix.
Find the minimal reproduction case: strip away everything that isn't needed to trigger the bug.

Mark task 1 as completed.

---

## Phase 2: Find the root cause

Mark task 2 as in_progress.

Don't guess and patch — systematically narrow the scope:

**Trace the code path:** Follow the data from input to the failure point. Add assertions or logging at the midpoint of the call chain. Is the data correct there? If yes, the bug is downstream. If no, upstream. Repeat until you find the exact line where things go wrong.

**Narrow the input:** What's the smallest input that triggers the bug? What's the largest input that succeeds? Strip away everything that isn't needed to trigger it.

**Check history (only if it used to work):** If a feature regressed, use `git bisect` to find the commit that broke it. Skip this for bugs that were always present.

**Before proceeding, state the root cause clearly:**
- What is the actual cause (not the symptom)?
- Why does it happen? (e.g., "empty string bypasses validation and reaches the DB layer")
- Under what conditions? (e.g., "only when the input is whitespace-only")

If you can't state this clearly, you haven't found the root cause yet — keep investigating. Present your root cause analysis to the user before fixing.

Mark task 2 as completed.

---

## Phase 3: Fix with TDD

Mark task 3 as in_progress.

Follow `/tdd`:
1. Write a test that reproduces the exact bug — confirm it fails
2. Write the minimal fix — change only what's necessary, don't refactor surrounding code
3. Confirm the test passes and no other tests regress

Mark task 3 as completed.

---

## Phase 4: Verify

Mark task 4 as in_progress.

1. Run `/verify` to ensure nothing else broke
2. Check that the fix addresses the root cause, not just the symptom
3. If the same category of bug could occur elsewhere, grep for similar patterns and flag them

Mark task 4 as completed.

---

## Phase 5: Record

Mark task 5 as in_progress.

Two questions, in order:

**1. Did the fix expose a requirement gap in an existing spec?**

A bug can mean the spec was wrong, ambiguous, or silent about the scenario that broke. Ask: "If someone re-implemented this feature from the spec alone, would they reproduce this bug?" If yes, the spec is incomplete.

- Find the related spec under `docs/specs/<slug>/spec.md` (check `docs/specs/INDEX.md`).
- Update it to cover the missing requirement — usually a new line under **What** and/or a new **GIVEN/WHEN/THEN** scenario. Keep it observable and behavior-focused; don't paste the root cause or the fix.
- If no spec exists yet but should (this category of behavior is feature-shaped and load-bearing), flag it to the user — don't create one unilaterally during a fix.
- If the bug was in infra, tooling, or behavior not covered by any feature spec, skip — there is nothing to update.

**Do NOT create a new spec for the bug fix itself.** Bugs aren't features.

**2. Did the fix encode a new project-wide convention?**

If the root cause exposed an architectural gap, a non-obvious constraint, or a rule that should bind future code (e.g., "GC code must fail-closed", "all bulk deletes must be transactional"), run `/record decision` to capture it as an ADR.

If neither question applies, skip this phase.

Mark task 5 as completed.

---

## Stop conditions

- **3 failed fix attempts:** Stop fixing and question the architecture. The bug may be a symptom of a deeper design issue.
- **Can't reproduce:** Don't guess. Add observability (logging, assertions) and wait for it to happen again.
- **Fix is larger than expected:** If the minimal fix touches many files, the root cause may be architectural. Discuss with the user before proceeding.

## What not to do

- Don't add try/catch to suppress the error — that hides the bug, it doesn't fix it
- Don't "fix" by adding defensive checks everywhere — fix the one place that's wrong
- Don't refactor while fixing — separate commits for separate concerns
- Don't claim "fixed" without a regression test proving it
