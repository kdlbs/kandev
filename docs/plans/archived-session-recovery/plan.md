---
created: 2026-09-08
status: complete
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004
system_design:
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
legacy_specs: []
---

# Implementation Plan: Archived Session Recovery

## Overview

Keep archived task history free of failed recovery attempts. Recheck recovery
after unarchive and show compact, actionable feedback for actual failures.
Implement the backend eligibility guard first, then client lifecycle handling,
then error presentation. All work orders run sequentially in the primary session.

The agent system owns this package because it owns session recovery eligibility
and feedback. Task cleanup remains the owner of workspace resource lifetime.
The existing requirement/design pair is extended; there is no separate UI spec.

## Confirmed root cause

The reported task is `3601d3d6-24ea-4f6c-ae1e-2242efc66c5b`; its session is
`d9c1f0bb-0ebb-4ac3-be13-e64cb92ad212`. Read-only inspection found the session
`CANCELLED`, last updated on 2026-08-27. Backend logs on 2026-09-08 at
19:40:00 +01:00 show `resume` rejected with `task is archived`, followed by
`restore_workspace` rejected with `session workspace not ready` and no path.
The session's referenced task environment was also reported missing.

`GetTaskSessionStatus` treats archive-cancelled sessions as resumable without
loading the task's current archive state. Both token and fresh-start branches
have this gap. `useSessionResumption` trusts those flags and has no archive
input. Its fallback attempts restore after every resume failure. The executor
rejects archived resume correctly, but `launchRestoreWorkspace` reaches the
runtime without the same task guard. The UI concatenates both errors and
renders them under a generic creation-error title.

The hook's one-attempt marker resets on task/session changes, not on unarchive.
This is a source-confirmed gap; in-place unarchive recovery has not yet been
reproduced on an isolated runtime. The reported task was not mutated.

### Smallest regression

Seed an archived task with an archive-cancelled session and no running row.
Call `GetTaskSessionStatus`: current code incorrectly returns
`needs_resume=true` and `is_resumable=true`. Repeat with a retained resumable
token. Opening that task reproduces the two launch attempts in the screenshot.

## Requirement conformance

- Extend the existing recovery contract with `REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004` because archive eligibility was not specified there.
- Preserve `AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.1` through `.7`. Add `.8` and `.9` to define compact feedback with accessible details.
- Preserve provider identity and explicit branch replacement under existing recovery requirements `001` and `003`.
- Reuse `AC-TASKS-RUNTIME-CLEANUP-001.7` and the completed [worktree resume package](../worktree-resume-after-unarchive/plan.md). Do not replace its recovery implementation.
- Preserve [prevent-auto-start behavior](../../specs/tasks/requirements/prevent-agent-autostart-on-open.md).

## Scope

### In scope

- Archive-aware status and direct restore rejection, plus typed launch conflict.
- Task detail, preview, and Quick Chat recovery lifecycle, including hydration,
  archive races, in-place unarchive, busy state, and stale fallback suppression.
- Short automatic recovery feedback with separately labeled details and all
  supported locales.
- Isolated desktop/mobile evidence for archived browsing, unarchive recovery,
  and disclosure/retry behavior.
- Public documentation of the corrected archive and recovery flow.

### Out of scope

- Changing or unarchiving the user's live task, starting its agent, or repairing
  its database during this package.
- Reconstructing missing or ambiguous environment ownership, schema migrations,
  changing cleanup, or restoring code that no longer exists.
- Automatic branch replacement, token clearing, new session creation as a
  substitute for resume, or changing Office/message-driven start policy.
- A general error framework, new settings, feature flags, or an alert redesign
  outside automatic session recovery.

## Technical approach

1. In `apps/backend/internal/orchestrator/task_operations.go`, load the authorized
   owning task before runtime status side effects and eligibility evaluation.
   Return false recovery flags and `resume_reason=task_archived` while archived.
   Guard `launchRestoreWorkspace` in `session_launch.go`. Preserve existing
   locked resume rejection. In `handlers/handlers.go`, map `ErrTaskArchived`
   to conflict details `{kind: task_archived}` using the existing error envelope.
2. In `use-session-resumption.ts`, add resolved archive state to the request
   lifecycle. Update every caller. Unknown state defers automatic operations.
   Extend commit-phase request generations to archive changes; check current
   identity before every downstream operation, including fallback. Consume the
   typed archive conflict without showing an error or attempting restore.
   Refresh task state through the existing data layer. Recheck once on successful
   unarchive, respecting the user's automatic-start preference.
3. Retain structured automatic recovery causes through the hook and its consumers.
   Render summaries and a semantic details disclosure in
   `components/task/ensure-session-error.tsx`. Keep creation errors separate.
   Preserve manual branch recovery and read-only fallback semantics. Update all
   locale catalogs using the existing translation generation/check tools.

No persistence or runtime ownership change is needed. The design follows the
existing [task worktree ownership decision](../../decisions/2026-08-08-task-owned-worktree-lifetime.md)
and [explicit branch recovery decision](../../decisions/2026-08-31-explicit-new-branch-session-recovery.md).
No new ADR is required for these conformance corrections.

## Tests

| Acceptance criteria | Regression evidence |
| --- | --- |
| `004.1`, `004.2` | New `task_session_archive_status_test.go`: `TestGetTaskSessionStatus_ArchivedTaskDisablesRecovery`; `session_launch_archive_test.go`: `TestLaunchRestoreWorkspace_ArchivedTask`; handler conflict test |
| `004.3`, `004.4` | New `use-session-resumption.archive.test.ts`: deferred status, deferred resume, archive/unarchive/archive generations, failed unarchive, preference enabled |
| `004.5`, `004.6` | Existing archive-cancelled resume/worktree tests plus isolated recoverable-workspace E2E and missing-workspace feedback fixture |
| `002.3`, `002.5`, `002.6`, `002.8`, `002.9` | `ensure-session-error.test.tsx` and hook tests: collapsed summary, both causes after expansion, typed detail retention, busy Retry, unchanged creation error |
| `002.7` | Existing resumption navigation tests remain green |

Test names above are planned additions unless explicitly described as existing.
Write each regression and observe its expected failure before changing its
production path. Do not grow oversized Go test files; use focused siblings.

## E2E tests

- Add `tests/task/archived-session-recovery.spec.ts` (`chromium`) and
  `tests/task/mobile-archived-session-recovery.spec.ts` (`mobile-chrome`). Cover
  `004.1`, `.4`, `.5`: history visible, no recovery launch frames while archived,
  Unarchive in place, same session recovery and usable workspace. Add the
  prevent-auto-start variant. Use real archive/unarchive APIs and isolated mock
  agents; do not stub the status result in the successful recovery case.
- Add failure/disclosure cases to these specs for `004.6`, `002.8`, and `002.9`.
  Inject bounded failures through the existing WS test pattern. Show both causes
  on expansion, retry, busy disablement, and recovery after a successful retry.
- Phone actions use `.tap()`, 44px coarse-pointer hit areas, keyboard-accessible
  disclosure, and no document horizontal overflow. Capture the final expanded
  and collapsed states. Reuse the shipped task layout and existing Unarchive
  action; no new navigation or drawer.
- Existing `tests/task/unarchive-detail-topbar.spec.ts` and
  `tests/task/unarchive-storage-recovery.spec.ts` remain compatibility checks.

## Work orders

- [x] [Task 01: Gate backend recovery on archive state](task-01-backend-archive-gate.md)
- [x] [Task 02: Reconcile recovery across archive transitions](task-02-client-archive-lifecycle.md)
- [x] [Task 03: Present compact recovery feedback](task-03-recovery-feedback.md)

Each work order contains exact commands and must record its red/green results.
Managed E2E commands build the current backend and web artifacts and run
sequentially. Install workspace dependencies once before package commands.

## Verification results

- Backend focused archive and recovery checks passed: 36 orchestrator tests, 1
  handler test, 22 executor tests, and 1 worktree test, all with `-race`.
- Web focused tests passed: 9 files, 103 tests. Typecheck, lint, i18n checks,
  and the new-code ratchet passed.
- Managed browser checks passed: 3 Chromium tests and 2 mobile-Chromium tests.
  They cover archived history, in-place recovery, prevent-auto-start, typed
  failure details, retry, touch targets, and overflow.
- Public documentation validation passed: 61 validator tests and 46 published
  pages.
- `rtk gofmt -l` reported no Go files. `git diff --check` passed.

## Risks

- The affected live session references a missing environment. This package
  removes the false archived recovery attempt; it does not guarantee recovery
  of that historical workspace after unarchive.
- Guarding setters alone leaves stale network operations possible. Check the
  request generation before fallback and refresh as well.
- Archive-state reads do not replace existing execution and cleanup concurrency
  barriers. Preserve locked resume checks and cover direct restore rejection.
- A server already processing a launch cannot be cancelled by ignoring its
  client callback. Existing archive teardown remains responsible for it.
- Mixed client/server versions must retain ordinary error messages. Typed
  archive details are additive; server rejection remains authoritative.
- Compact summaries must retain both original causes in accessible details.
  Do not replace errors with a generic-only message or infer branch loss from text.
