---
name: fix
description: Fix bugs and issues — reproduce, find root cause, minimal fix with regression test. Use when something is broken.
---

# Fix

Systematic bug fixing: reproduce the problem, find the root cause, apply a minimal fix with a regression test. **No fix without root cause investigation first.**

## Steps

### 1. Reproduce

Before anything else, reproduce the bug reliably. Pick the right method based on where the bug lives:

- **Backend** (API, logic, data): write a Go test that calls the function/endpoint and asserts the wrong behavior. Run with `go test -v -run TestName ./path/...`
- **Frontend** (UI, state, interaction): write a Playwright E2E test using `/e2e` that navigates to the page and triggers the bug. Run with `make test-e2e-headed` to see it visually.
- **Full-stack** (user flow breaks end-to-end): Playwright E2E test that exercises the full path from UI through API to DB and back.
- **Unclear where it lives**: start by reading the code path from the reported symptom (a page, an error message, a wrong value) back to its source. Then write the test at the appropriate level.

If it can't be reproduced, add logging/assertions to gather more info — don't guess at a fix.
Find the minimal reproduction case: strip away everything that isn't needed to trigger the bug.

### 2. Narrow down the cause

Don't guess and patch — systematically narrow the scope:

**Trace the code path:** Follow the data from input to the failure point. Add assertions or logging at the midpoint of the call chain. Is the data correct there? If yes, the bug is downstream. If no, upstream. Repeat until you find the exact line where things go wrong.

**Narrow the input:** What's the smallest input that triggers the bug? What's the largest input that succeeds? Strip away everything that isn't needed to trigger it.

**Check history (only if it used to work):** If a feature regressed, use `git bisect` to find the commit that broke it. Skip this for bugs that were always present.

### 3. Confirm root cause

Before fixing, state the root cause clearly:
- What is the actual cause (not the symptom)?
- Why does it happen? (e.g., "empty string bypasses validation and reaches the DB layer")
- Under what conditions? (e.g., "only when the input is whitespace-only")

If you can't state this clearly, you haven't found the root cause yet — go back to step 2.

### 4. Fix with TDD

Follow `/tdd`:
1. Write a test that reproduces the exact bug — confirm it fails
2. Write the minimal fix — change only what's necessary, don't refactor surrounding code
3. Confirm the test passes and no other tests regress

### 5. Verify

- Run `/verify` to ensure nothing else broke
- Check that the fix addresses the root cause, not just the symptom
- If the same category of bug could occur elsewhere, grep for similar patterns

## Stop conditions

- **3 failed fix attempts:** Stop fixing and question the architecture. The bug may be a symptom of a deeper design issue.
- **Can't reproduce:** Don't guess. Add observability (logging, assertions) and wait for it to happen again.
- **Fix is larger than expected:** If the minimal fix touches many files, the root cause may be architectural. Discuss with the user before proceeding.

## What not to do

- Don't add try/catch to suppress the error — that hides the bug, it doesn't fix it
- Don't "fix" by adding defensive checks everywhere — fix the one place that's wrong
- Don't refactor while fixing — separate commits for separate concerns
- Don't claim "fixed" without a regression test proving it
