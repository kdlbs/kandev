---
name: fix
description: Fix bugs and issues — reproduce, find root cause, minimal fix with regression test. Use when something is broken.
---

# Fix

Systematic bug fixing: reproduce the problem, find the root cause, apply a minimal fix with a regression test.

## Planner Entry

In the user-started primary session:

1. Delegate reproduction and root-cause diagnosis to an `implementer` worker
   with production edits forbidden. It may create or update only the minimal
   failing regression test when that test path is explicitly owned. This
   diagnosis packet explicitly overrides the generic implementer workflow:
   reproduce and trace, return evidence and root cause, do not patch production
   code, and do not continue to Phase 3.
2. Review the evidence and present the root cause to the user.
3. Delegate the minimal production patch to an `implementer` worker. It
   preserves or completes the diagnosis regression test and runs green targeted
   verification without duplicating that test unnecessarily.
4. Apply `/planner-orchestration` risk routing: obtain local `code-review` or
   qualifying current-head PR AI semantic-review evidence, run risk-triggered
   QA, always run final `verify`, and delegate `security-auditor` for sensitive
   fixes.

Stop after dispatching and coordinating these assignments. Do not continue into
the direct fix procedure below. A diagnostic worker performs phases 0-2 and
returns evidence and root cause. It returns a red-test result when it owns a
test path; otherwise it returns concrete reproduction evidence and a proposed
regression-test scenario/path. Production code remains forbidden, and
diagnostic instrumentation is test-only when owned or non-mutating. It must
not continue to Phase 3. A fix implementer performs the minimal production
patch, preserves or completes that regression test without unnecessary
duplication, and runs targeted tests. No worker performs planner phases 4-5 or
spawns other workers.

## Available skills and subagents

- **`/tdd`** — Use for implementing the fix with a regression test (Red-Green-Refactor).
- **`/e2e`** — Use when the bug is in a user-facing flow and needs a Playwright regression test.
- **`/verify`** — The planner launches this after the fix worker reports targeted checks green.
- **`/record`** — The planner uses this when the worker reports a durable architectural decision.

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

0. **Read the issue + view attachments** — When the bug originates from an issue tracker, fetch the canonical issue and view every image attachment before hypothesizing
1. **Reproduce the bug** — Write a test or find a reliable reproduction case
2. **Find the root cause** — Trace the code path, narrow the scope, state the cause clearly
3. **Fix with TDD** — Minimal fix with regression test, no surrounding refactors
4. **Review and verify** — Planner delegates risk-routed semantic review and
   QA, mandatory final verification, and conditional security audit; assigns any remediation
5. **Record** — Planner updates durable artifacts when worker evidence exposes a requirement or architecture gap

Then start with task 0 when the bug is issue-sourced, otherwise task 1. Mark each task in_progress when you begin it and completed when you finish it. Do not skip ahead — fixing without reading the source issue (when one exists) or reproducing leads to patches that don't address the real problem. Fixing without understanding the root cause leads to whack-a-mole.

---

## Phase 0: Read the issue + view attachments

Mark task 0 as in_progress (skip this phase if there is no issue tracker source — e.g. a locally discovered bug with no linked issue).

When the bug originates from an issue tracker, fetch the **canonical issue** and view **every image attachment** before hypothesizing. Handed-down restatements (Kandev task/subtask text, Slack paste, PR description) are leads only — verify against the source.

```bash
gh issue view <N> --repo <owner/repo> --json title,body,comments
curl -sL "https://github.com/user-attachments/assets/<id>" -o /tmp/issue-<N>-<n>.png   # then Read the local file
```

- **Transcribe exact strings from screenshots** into the repro (paths, error messages, UI labels). Screenshots often hold details missing from the text summary.
- **Mine structured issue-template fields** (OS, install mode, clean-state, version) to constrain repro conditions.
- **Video attachments** cannot be Read as stills — note their presence and rely on the issue text/comments for motion-specific details.

Mark task 0 as completed.

---

## Phase 1: Reproduce

Mark task 1 as in_progress.

Before anything else, reproduce the bug reliably:

- **If the diagnosis packet explicitly owns a test path:** write and run the
  minimal failing regression test at the appropriate level, then return its red
  result with the evidence and root cause.
- **If it does not own a test path:** use existing tests, manual or
  non-mutating runtime evidence, or read-only tracing. Do not edit tests or
  production. Return concrete reproduction evidence plus the proposed
  regression-test scenario and path for the Phase 3 patch worker.

If it can't be reproduced, use test-only assertions/instrumentation only when
the test path is owned; otherwise use non-mutating runtime evidence — don't
guess at a fix.
Find the minimal reproduction case: strip away everything that isn't needed to trigger the bug.

For flaky Playwright failures, do not stop after one clean focused run. Use the
`/e2e` flake reproduction flow: run the exact CI shard in
`ghcr.io/kdlbs/kandev-ci:runtime-latest` with `CI=true`, then add resource
limits such as `--cpus=2 --memory=4g --memory-swap=4g` and repeat the failing
test or full spec file. Preserve nearby test ordering when a single-test repeat
passes, and inspect `error-context.md` from the failed repeat before fixing.

Mark task 1 as completed.

---

## Phase 2: Find the root cause

Mark task 2 as in_progress.

Don't guess and patch — systematically narrow the scope:

**Trace the code path:** Follow the data from input to the failure point. Use
test-only assertions/instrumentation only when the test path is owned;
otherwise use non-mutating runtime evidence at the midpoint of the call chain.
Is the data correct there? If yes, the bug is downstream. If no, upstream.
Repeat until you find the exact line where things go wrong.

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
1. Preserve or complete the diagnosis regression test; if diagnosis did not
   own a test path, add the proposed regression test, then confirm the exact
   bug fails
2. Write the minimal fix — change only what's necessary, don't refactor surrounding code
3. Confirm the regression test and targeted verification pass

Mark task 3 as completed.

---

## Phase 4: Verify

Mark task 4 as in_progress.

This is a planner coordination phase. After the fix implementer reports its
targeted test results and compact handoff capsule (intent/acceptance; base/head
SHA when applicable; changed files and entry points; named spec/ADR sections;
risk tags; exact targeted commands/results; uncertainties), apply
`/planner-orchestration` risk routing. Qualifying PR AI semantic-review
evidence may cover routine work; without complete exact-current-head evidence,
launch local `code-review`. Require `qa` for integration, public contracts,
persistence, concurrency, important error/recovery behavior, cross-component
wiring, or missing faithful independent behavior evidence. A user-flow may skip
QA only when faithfully tested with no integration boundary; record the skip and
reason. Always run final `verify`, and launch
`security-auditor` for auth, workspace isolation, filesystem/process
execution, integrations, webhooks, secrets, or agent/tool permissions. PR AI
review never replaces verification or security audit. The planner's acceptance
check is not a substitute for required gates.

Route every finding to a bounded implementer packet. Reuse the same native
thread when role, change, and file scope remain materially the same; use a new
thread after major redesign, unrelated scope, stale/noisy context, or when
independent judgment is needed. Rerun only affected semantic, QA, and security
checks after remediation; qualifying PR AI evidence must cover the new head.
Always rerun final `verify`. The fix worker only reports targeted results, its
handoff capsule, and any similar patterns it noticed.

Mark task 4 as completed.

---

## Phase 5: Record

Mark task 5 as in_progress.

Two questions, in order:

**1. Did the fix expose a requirement gap in an existing spec?**

A bug can mean the spec was wrong, ambiguous, or silent about the scenario that broke. Ask: "If someone re-implemented this feature from the spec alone, would they reproduce this bug?" If yes, the spec is incomplete.

- Find the related spec under `docs/specs/<slug>/spec.md` (check `docs/specs/INDEX.md`).
- The planner updates it to cover the missing requirement, usually with a new
  line under **What** or a new GIVEN/WHEN/THEN scenario. The fix worker only
  reports the gap.
- If no spec exists yet but should (this category of behavior is feature-shaped and load-bearing), flag it to the user — don't create one unilaterally during a fix.
- If the bug was in infra, tooling, or behavior not covered by any feature spec, skip — there is nothing to update.

**Do NOT create a new spec for the bug fix itself.** Bugs aren't features.

**2. Did the fix encode a new project-wide convention?**

If the root cause exposed an architectural gap, a non-obvious constraint, or a
rule that should bind future code, the planner runs `/record decision`. The fix
worker reports the candidate decision and alternatives; it does not create the
ADR.

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
