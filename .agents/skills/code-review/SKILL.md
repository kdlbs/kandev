---
name: code-review
description: Review changed code for quality, security, and architecture compliance. Use after implementing features or before opening PRs.
---

# Code Review

## Planner Entry

Load `/planner-orchestration`. The user-started primary session delegates this
procedure to the registered `code-review` worker and reviews its findings. It
assigns any fixes to an implementer and delegates security-sensitive follow-up
to `security-auditor`; it does not review or edit the change directly. An
explicitly assigned review worker continues below and does not spawn workers.

Review the current changes in the codebase (Go backend + Vite/React SPA monorepo). Every finding needs a `file_path:line_number` reference, an explanation of *why* it matters, and a concrete fix.

Start from intent and evidence: read the spec/task first when available, then changed tests before production code. Tests reveal the expected behavior and whether the change is actually verified.

## Available skills

- **`/tdd`** — Recommend when flagging untested logic. The author can use this to add tests.

## Steps

### 1. Identify changed files and check scope

Determine the right diff scope:
- **Local changes**: `git diff --name-only` (unstaged) and `git diff --cached --name-only` (staged)
- **PR review**: `git diff origin/<base_branch>...HEAD --name-only` to diff against the base branch

For an existing PR, first confirm the exact head under review. Do not assume the
local checkout is current: inspect the PR's base branch and head SHA, fetch the
head if needed, and use that immutable SHA in the diff. If the current PR head
cannot be fetched, say so rather than reporting a stale checkout as a review of
the current PR.

Record the base and head SHA for each review round. A new contributor push starts
a new round: reassess prior findings and the verdict against the new head, and
verify checks or workflow results for that head rather than relying on a PR
number or author summary.

Compare the PR description, checklist, and claimed manual validation with that
exact head, especially after a major refactor. Report stale claims separately;
they are not verification evidence for the current diff.

Read each changed file in full — understand surrounding code, not just the diff. Navigate callers, interfaces, and tests to understand changes end-to-end.

For each file, identify which requirement or intent it serves. Flag any changes that don't map to the task — scope creep is a blocker.

### 2. Review tests and verification first

Before reviewing implementation details:
- Read changed tests and nearby existing tests.
- Check whether tests assert behavior, not implementation details.
- Check whether the selected test level is appropriate: unit for pure logic, integration for boundaries, E2E for critical browser flows.
- Identify missing coverage for happy path, key error paths, edge cases, auth/workspace boundaries, and concurrency/order-sensitive behavior.
- For concurrent or event-driven changes, require a deterministic schedule that checks ownership or generation identity, stale-event handling, cancellation, and lock scope. Channel/barrier coordination is preferable to timing sleeps.
- For stale-event races, cover both event-before-successor and delayed-old-event-after-successor orderings. Prefer integration coverage for cross-package event or callback paths when practical.
- Treat missing tests for new or changed non-UI logic as a blocker unless the change is explicitly untestable and says why.

### 3. Review for issues

Check every changed file for the following layers. Skip layers that don't apply to the change.

**Security** (blockers if found):
- No secrets, tokens, or credentials in code
- Input validation at system boundaries (user input, API handlers, external data)
- No SQL injection, XSS, command injection, or path traversal risks
- Authentication and authorization checks in place for new endpoints
- No insecure crypto (MD5/SHA1 for passwords, weak random)
- Workspace and office boundaries are enforced; no cross-workspace data, credentials, logs, or agent context leakage
- Agent/tool execution is constrained by code, not prompt text alone

**Architecture:**
- Frontend: no direct data fetching in components (must go through store), shadcn imports from `@kandev/ui` not `@/components/ui/*`
- Backend: provider pattern for DI, context passed through call chains, event bus for cross-component communication
- Search `docs/specs/` and `docs/decisions/` for the affected subsystem; flag an accepted spec or ADR that the change makes inaccurate
- New abstractions justified — no over-engineering
- Concerns cleanly separated (single responsibility)

**Logic & correctness:**
- Edge cases handled (empty input, nil/null, zero, max values)
- Error paths covered and not silently swallowed
- Race conditions or concurrency issues in concurrent code
- Async events carry an immutable identity when they can outlive the operation that created them; stale events cannot mutate a replacement operation
- Locks protect only the atomic ownership boundary and are not held across unbounded I/O or a full asynchronous operation
- Synchronous callbacks cannot re-enter a lock they already need; moving publication asynchronous also requires an immutable value snapshot, clear shutdown ownership, and protection against a delayed event changing successor state
- When a generation, token, or lease authorizes a side effect, validate and mutate within one critical section. Check every terminal path separately: success, raw error, cancellation, timeout, and disconnect.
- Detached goroutines have immutable snapshots and a real happens-before relationship before reading state that can otherwise transition underneath them

**Performance:**
- No N+1 queries (loop with individual DB calls)
- No memory leaks (unclosed connections, streams, listeners)
- Missing database indexes for new query patterns
- Algorithm complexity appropriate for the data scale

**Complexity limits** (CI also enforces these, but catch them early to avoid pushing and waiting):
- Go: functions ≤80 lines, ≤50 statements, cyclomatic ≤15, cognitive ≤30, nesting ≤5
- TS: files ≤600 lines, functions ≤100 lines, cyclomatic ≤15, cognitive ≤20, nesting ≤4
- If too large or complex, split into smaller cohesive files/functions

**Code quality:**
- No duplicated logic — extract shared helpers or constants
- No dead code, unused imports, or commented-out code
- Check for orphaned code: if the PR refactored or removed callers, grep for functions/types/exports that lost their last consumer
- No speculative code — unused flags/options, "reserved for future" scaffolding, one-off abstractions with a single call site, options parsed but never used
- Naming clear and consistent with project conventions
- Deep nesting (>3 levels) — use early returns

**AI slop detection:**
- Comments that restate code or narrate obvious steps
- Unnecessary try/catch that swallow errors or return silent defaults in trusted internal paths
- Redundant validation where inputs are already parsed/typed
- `as any` or `as unknown as X` casts used to dodge type errors instead of fixing types
- Defensive checks abnormal for the area of the codebase — compare with surrounding code patterns

**Testing (blocker if missing):**
- Backend (Go): new or changed functions/methods must have corresponding `*_test.go` tests
- Frontend (JS/TS libs only): new utility functions, hooks, API clients, and store slices must have `*.test.ts` tests
- We do NOT test React components — skip those
- Exceptions: config files, generated code, React component markup
- Missing tests for new or changed logic is a **blocker** — suggest what tests to add and recommend `/tdd`

### 4. Report

When the user says not to post or modify the PR, do not make any GitHub
mutation: no fixes, comments, review submissions, or thread resolution. When
the user asks for a review only, or when reviewing an external contributor's
branch, do not edit the checkout or push code; report findings through the
channel the user requested. Do not submit or resolve reviews unless explicitly
asked.

Report findings to the planner with a concrete suggested fix. Do not edit the
checkout; the planner creates a bounded implementer assignment for remediation.

### 5. Output

Use this format:

---

### Findings

#### Blocker (must fix before merge)
*Security holes, data loss risk, broken logic, crashes, missing tests for new/changed logic*

1. **[Title]** — `file.go:42`
   - Issue: what's wrong
   - Why: why it matters
   - Fix: concrete suggestion or code snippet

#### Suggestion (recommended, doesn't block)
*Performance problems, poor error handling, architectural concerns*

### Summary

| Severity | Count |
|----------|-------|
| Blocker | N |
| Suggestion | N |

**Verdict:** Ready to merge / Ready with suggestions / Blocked — fix blockers first

---

**Rules:**
- Only report findings you're >=80% confident about — quality over quantity
- Don't mark style preferences as blockers — linters cover formatting
- Every criticism needs a suggested fix
- Say when uncertain and recommend a specific investigation instead of guessing
- Don't give feedback on code you didn't read
- Omit empty severity sections

**Not a finding (skip these):**
- Pre-existing issues on lines the change didn't modify
- Things linters, typecheckers, or CI already catch (imports, types, formatting) — exception: still report complexity-limit violations since they require code changes to fix
