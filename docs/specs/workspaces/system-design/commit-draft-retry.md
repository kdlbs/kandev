---
status: current
system: workspaces
created: 2026-10-08
requirements:
  - REQ-WORKSPACES-COMMIT-DRAFT-RETRY-001
---

# Commit draft retry system design

## Purpose and boundaries

Restore commit retryability within the mounted native VCS provider using a local
acknowledgement and captured draft ownership. Workspaces owns the paired
requirement; this does not introduce a new system, storage boundary or Git
transport. Platform's [workspace Git status](../../platform/requirements/workspace-git-status.md)
contract supplies status/fan-out inputs, and Tasks retains environment bindings.

## Requirement mapping

| Criteria of REQ-WORKSPACES-COMMIT-DRAFT-RETRY-001 | Design section |
| --- | --- |
| `.1`, `.2`, `.3`, `.8` | Acknowledgement and settlement |
| `.4`, `.5` | Admission and captured draft |
| `.6`, `.7` | Local ownership and reopening |
| `.9` | Desktop and phone verification |

## Current source and consumers

At inspected main/HEAD `fd4f6c7583e77b546a926128cfe1fda0965af0f8`:

- `components/vcs/vcs-dialogs.tsx`: `handleCommit` closes first, then clears
  title/body/repo after awaiting a void feedback hook. `useCommitDialogState`
  also resets title/body/Stage all on every opening.
- `hooks/use-git-with-feedback.ts`: false results and exceptions update the
  error toast and return void; success is indistinguishable to the caller.
- `useVcsDialogs` consumers are `components/vcs-split-button.tsx`,
  `components/session-commands.tsx` and `components/task/changes-panel-data.tsx`.
  They open the shared provider, including explicit per-repository openings.
  `components/task/task-page-inner.tsx` mounts that provider above task content
  with `effectiveSessionId`; it has no session key forcing remount.
- Other `useGitWithFeedback` callers in the split button (pull/push/rebase/merge)
  and session commands (awaiting a wrapper) ignore its return value. Keep their
  signatures, operations and feedback unchanged; no caller migration is needed.
- `hooks/domains/session/use-session-git.ts` dispatches explicit scopes directly
  and aggregates fan-out. `aggregatePerRepoResults` reports success only when
  all participating repositories succeed. `hooks/use-git-operations.ts` sends
  `worktree.commit` with `session_id`, `message`, `stage_all`, `amend`, and an
  optional `repo`; `repositoryScopePayload` preserves `repo === ""`.

These four causal/transport sources have no diff from known main
`7697d361c51cf1e73f6ab72b41d19a57a1a397d5`. Source comparison supplies reconciliation,
not a new test result. Existing helper-only `vcs-dialogs.test.ts` does not prove
commit failure recovery. The adjacent rendered dialog/field tests demonstrate
native Radix, TooltipProvider and locale setup patterns.

## Acknowledgement and settlement

Make `useGitWithFeedback` explicitly return `Promise<boolean>`: true only for
`result.success`; false for a false result or caught request exception, after
the existing toast update. This internal acknowledgement is sufficient; no
result union or shared mutation framework is required. Preserve loading,
success/error titles, descriptions, truncation and translation behavior.

`handleCommit` keeps the dialog open until settlement. On false, leave its
current raw draft untouched and mark it retryable. On true, close/reset only
the unchanged draft still owned by that attempt. If values changed after
admission, keep the latest raw values open (unless explicitly dismissed) and
retryable for a later user submission. Do not infer success from lack of a
throw. Aggregate false, including partial success, follows the same retention
path; fan-out and already completed Git writes remain unchanged.

## Admission and captured draft

Commit state is extracted into
`components/vcs/use-commit-dialog-state.ts` to keep the existing
600-line dialog file within lint limits. It is a feature-local hook, not a
reusable mutation framework. Capture an immutable attempt containing raw title,
raw body, Stage all, exact repository scope, current session/environment scope
generation and draft revision. Trim only its payload title/body. Build
`title + "\n\n" + body` only when the trimmed body is non-empty. Capture the
commit callback and submit once with amend false.

Ref-based admission owns the pending attempt before any await, including
same-tick repeated callbacks. Blank title, missing session, current pending or
existing Git loading admit nothing. Project local pending alongside existing
`isGitLoading` into the existing button's loading/disabled prop; markup and copy
remain unchanged. Draft edit revision advances on actual field changes, even
when an edit is later reverted to its earlier bytes. Completion must release
only its own attempt. Never let unrelated status refresh or an old request's
loading reset release the local admission guard.

## Local ownership and reopening

Keep one local current draft, not a map of drafts. A scope comprises the mounted
owner, session ID, its `environmentIdBySessionId` binding, and exact repository
choice. Reuse `useSessionGitPendingScope` for the existing session/environment
identity. A fresh scope object and draft-owner token form a local generation
for identity changes and deliberate
different-scope openings so A-to-B-to-A cannot revive an old attempt. Compare
current committed ownership at admission and settlement, including callbacks
captured before a scope change; retire ownership on unmount. This guard is
commit-local and does not alter session/store lifetime or cancel transport.

For same-scope openings, preserve pending/failed drafts and newer unacknowledged
edits. Opening a different repository deliberately starts fresh and retires the
old attempt's UI authority. Ordinary dismissed, never-submitted drafts retain
the existing reset-on-open behavior. After unchanged success, the next opening
is fresh (Stage all false). Normalize only actual repo strings; retain the
defensive event-argument handling. Do not collapse undefined and empty string.

Cancel/Escape/outside dismissal changes visibility only for retryable drafts.
Failure never forces a dismissed dialog open. Unchanged success can clear its
own hidden draft. Settlement of a retired attempt may still emit its original
toast, but cannot change a newer dialog, draft or pending owner. Session or
environment changes project a fresh/closed commit surface before stale callbacks
can act; changing back is still a new generation. These are bounded admission
and settlement guards, not a promise of retention across navigation.

## Desktop and phone verification

Use the `/mobile-parity` pure state/data exception: no rendered composition,
copy, classes, touch behavior, scroll owner, breakpoint or navigation changes.
The existing `CommitDialog` and `CommitBodyField` are the shipped surface and
remain shared. A new mobile Playwright scenario or browser build is unnecessary
for this correction. Native rendered integration tests must exercise controls,
pending/failure/success and dismiss/reopen outcomes. Parameterize the shared
flow at desktop and phone window sizes, and change viewport during a pending
edit without remounting. Happy-dom proves state/control outcomes, not browser
geometry, touch ergonomics or live backend Git behavior. No such claims are made.

Keep real `VcsDialogsProvider`, StateProvider/createAppStore, ToastProvider,
TooltipProvider, `useSessionGit`, `useGitOperations` and `useGitWithFeedback` in
the integration suite. Mock only WebSocket requests and frontend report
transports; utility generation remains unused. Seed actual session-to-environment
and per-repository status inputs. Assert transport payloads and native inputs,
not only a helper or a mocked commit callback. Use deferred responses for
pending/late-owner tests, resolving every admitted request before teardown.

## Persistence, permissions and feedback

No persisted draft, migration, storage write, backend/API change or new
permission rule. The existing toast is the diagnostic surface; no new metric
or copy is required. Public recovery guidance belongs in the existing commit
section of `docs/public/sessions-and-review.md` during implementation. Its
how-to content should say that a failed commit retains the draft for retry in
the same scope; successful commit clears it. Do not promise retention across
session/environment changes or reload, or imply rollback of partial success.

## Implementation plans

[Commit draft retry plan](../../../plans/commit-draft-retry/plan.md).
