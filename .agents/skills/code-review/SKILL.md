---
name: code-review
description: Review changed code for quality, security, and architecture compliance. Use only when the user explicitly requests local review or a PR finding requires it.
---

# Code Review

## Planner Entry

Run this local review only when the user explicitly asks or an actionable PR/CI
finding requires it. Do not use it automatically before opening a PR: the two
configured PR AI reviewers are the semantic-review gate.

Review the current changes in the codebase (Go backend + Vite/React SPA monorepo). Every finding needs a `file_path:line_number` reference, an explanation of *why* it matters, and a concrete fix.

Start from intent and evidence: read the spec/task first when available, then changed tests before production code. Tests reveal the expected behavior and whether the change is actually verified.

### Architecture discussion gate

For a large architectural change, require a linked issue with maintainer
discussion before the PR opens. If the issue or discussion is missing, report a
blocker. Prefer one logical change and a small diff because this limits risk and
maintainer burden.

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
number or author summary. From `scripts/pr-state --summary`, also record
`pr.base_ref_name`, `pr.base_head_oid`, `pr.merge_base_oid`, and
`pr.base_advanced_since_head` when available.

For a follow-up round, inspect `<previous-reviewed-head>..<current-head>` first
to isolate the author's response, then re-evaluate `<base>...<current-head>` for
complete PR coverage. Record both immutable heads.

When `pr.base_advanced_since_head` is `true`, validate the actual merge result
before declaring the PR ready. Record the latest base and immutable head SHAs,
create a temporary worktree from that base, merge the head with `git merge
--no-commit --no-ff <head-sha>`, and run focused verification in the merged tree
before removing the worktree. GitHub's `mergeable: MERGEABLE` status proves
conflict compatibility, not that the merged result was tested. If an older
`pr-state` helper lacks the base fields, resolve the current base
head only as a fallback with `gh api
repos/{owner}/{repo}/git/ref/heads/{base}` and derive/record the merge base
before making the same decision; do not try the unsupported
`gh pr view --json baseRefOid` field.

For an existing GitHub PR, inspect both `scripts/pr-state --summary <PR>` and
`scripts/pr-resolve list <PR>` before treating review feedback as clean. Read
the body of any exact-current-head review as well as inline threads: bots can
place actionable findings outside the diff. A review's `commit_id` is the
current-head signal; timestamps are only collection order. If `pr-state`
reports hidden unresolved threads, use `pr-resolve list` to inspect them rather
than assuming the filtered list is complete.

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
- When an HTTP mutation returns a full entity while WebSocket/event updates can
  update the same entity, ensure a delayed HTTP response cannot overwrite the
  newer event. Prefer a narrow mutation response or guard a full merge with an
  immutable revision/`updated_at`; cover it with a deferred-response test that
  applies the newer event first.
- For ordering guarantees across an event bus, trace producer, remote
  transport, and gateway/client delivery. Sequential publishes on separate
  subscriptions do not establish client order; require a unified stream or
  sequence-aware buffering, with a transport-boundary test and local-emulator
  coverage.
- For terminal event streams, block an earlier publication, enqueue a terminal event (for example delete or cancellation), then enqueue a stale update. Assert no later mutation reaches an upserting consumer; queues must tombstone the entity or discard pending work at the terminal boundary.
- When completion events lack a stable workload identity, test N outstanding registrations with N completion signals and duplicate delivery. A single-registration test cannot prove that uncorrelated completions retire work correctly. Compare this behavior with the accepted spec or ADR; a passing test that contradicts the contract is still a blocker.
- Treat missing tests for new or changed non-UI logic as a blocker unless the change is explicitly untestable and says why.

### 3. Review for issues

Check every changed file for the following layers. Skip layers that don't apply to the change.

**Security** (blockers if found):
- No secrets, tokens, or credentials in code
- When persisted configuration is copied into UI or session metadata, trace it
  through the applicable sanitizer/redaction boundary; storage-safe values are
  not automatically presentation-safe.
- Input validation at system boundaries (user input, API handlers, external data)
- No SQL injection, XSS, command injection, or path traversal risks
- Authentication and authorization checks in place for new endpoints
- No insecure crypto (MD5/SHA1 for passwords, weak random)
- Workspace and office boundaries are enforced; no cross-workspace data, credentials, logs, or agent context leakage
- Agent/tool execution is constrained by code, not prompt text alone

**Architectural fit (highest priority):**
- Changes belong in the correct layer/module and follow the dependency direction used by the codebase
- Business/domain logic is not placed in controllers, transport handlers, repositories, data sources, or infrastructure code
- Controllers handle protocol concerns, use cases orchestrate workflows, repositories define persistence needs, and data sources handle external systems
- Domain/application code does not depend on frameworks, transport models, database models, or vendor-specific types
- Changes do not bypass existing boundaries, duplicate responsibilities, or introduce unnecessary coupling between modules or domains
- New interfaces and abstractions have clear ownership and represent a meaningful boundary, rather than wrapping a single implementation
- Compare with neighbouring features and established patterns, but flag deviations only when they create a real architectural or maintainability problem
- Treat fundamental architectural misplacement or broken dependency direction as a blocker
- Frontend: no direct data fetching in components (must go through store), shadcn imports from `@kandev/ui` not `@/components/ui/*`
- Backend: provider pattern for DI, context passed through call chains, event bus for cross-component communication
- Search `docs/specs/` and `docs/decisions/` for the affected subsystem; flag an accepted spec or ADR that the change makes inaccurate
- New abstractions justified — no over-engineering
- Concerns cleanly separated (single responsibility)

**Data & state modelling:**
- Domain entities, value objects, DTOs, persistence models, and external API models remain separate where their responsibilities differ
- State transitions and invariants are explicit and cannot create invalid or partially updated state
- There is a single clear source of truth; state or business rules are not duplicated across layers
- Nullability, optional fields, defaults, and invalid combinations are modelled deliberately
- Persistence schemas or transport types are not leaking implementation details into domain/application contracts
- Concurrency, retries, partial failures, and duplicate requests cannot corrupt state or apply transitions more than once
- Backward compatibility, migrations, and mixed-version behaviour are considered when contracts or persisted data change

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

**Build and platform boundaries:**
- For changed Makefiles, shell scripts, or CI path filters, trace each changed
  target through the shell and platform branches. Distinguish executable naming
  from recipe-shell syntax; inspect `OS`, `MSYSTEM`, and `SHELL` assumptions.
- When simulating a Windows Make branch from POSIX, prefer
  `scripts/check-make-shells`. If a manual `make -n` is necessary, neutralize
  its parse-time probes as that checker does (`NULL_REDIR= BUILD_TIME=simulated`)
  so POSIX does not create `NUL` artifacts. Compare `git status --short` with
  the initial snapshot afterward.
- Use `make -n <changed-target>` for every affected platform branch that is
  available, and confirm CI invokes the changed target. Include docs or
  configuration paths when a validator or test reads them.

**AI slop detection:**
- Comments that restate code or narrate obvious steps
- Unnecessary try/catch that swallow errors or return silent defaults in trusted internal paths
- Redundant validation where inputs are already parsed/typed
- `as any` or `as unknown as X` casts used to dodge type errors instead of fixing types
- Defensive checks abnormal for the area of the codebase — compare with surrounding code patterns

**Testing (blocker if missing):**
- Backend (Go): new or changed functions/methods must have corresponding `*_test.go` tests
- Frontend: new utilities, hooks, API clients, and store slices must have focused tests. Pure React markup may skip a unit test, but behavior-bearing components (conditional status, accessibility, store-derived state, or responsive/mobile variants) need focused `*.test.tsx` coverage and/or E2E. Route responsive user-facing changes through `/mobile-parity`.
- Async UI lifecycle: loading/busy state must clear through `finally` or equivalent terminal cleanup for success, error, cancellation, and early/no-op returns; require focused tests for those terminal paths.
- Exceptions: config files, generated code, and pure React component markup
- Missing tests for new or changed logic is a **blocker** — suggest what tests to add and recommend `/tdd`

### 4. Report

When the user says not to post or modify the PR, do not make any GitHub
mutation: no fixes, comments, review submissions, or thread resolution. When
the user asks for a review only, or when reviewing an external contributor's
branch, do not edit the checkout or push code; report findings through the
channel the user requested. Do not submit or resolve reviews unless explicitly
asked.

Before a read-only review ends, compare `git status --short` with the initial
snapshot. Remove only diagnostic artifacts demonstrably created during the
review; preserve all pre-existing user changes.

Report findings with a concrete suggested fix. Do not edit the checkout during
a review-only request; otherwise remediate in the same primary conversation.

Before drafting or posting a copyable PR comment, map each proposed point against
exact-current-head review bodies, top-level discussion comments, and all
unresolved or hidden threads. If a point is already raised, omit it from the
author-facing comment but retain it in the private review summary; repeat it
only when the user explicitly asks for reinforcement. Re-fetch immediately
before posting and start a new review round if `headRefOid` changed.

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
