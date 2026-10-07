---
status: current
system: workspaces
requirements:
  - REQ-WORKSPACES-SAVED-FILE-CONTENT-001
---

# Saved File Content System Design

## Boundary and ownership

Workspaces owns saved filesystem content. The repair is bounded to
`WorkspaceTracker.ApplyFileDiff` in
`apps/backend/internal/agentctl/server/process/workspace_files.go`.
No compatible saved-content pair exists in the current Workspaces catalog;
editor-file-containment covers lexical editor admission, while
symlink-identification covers metadata. The
[UI mutation design](../../ui/system-design/file-editor-mutation-ownership.md)
continues to own editor reply publication independently.

This documents missing intended behavior and corrects local patch ownership
within existing boundaries. It requires no new ADR, global lock, shared
framework, API field, persistence model, or runtime flag.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| `AC-WORKSPACES-SAVED-FILE-CONTENT-001.1` | Request-owned patch, Verification |
| `AC-WORKSPACES-SAVED-FILE-CONTENT-001.2` | Caller contract, Request-owned patch, Verification |
| `AC-WORKSPACES-SAVED-FILE-CONTENT-001.3` | Compatibility and failure handling |
| `AC-WORKSPACES-SAVED-FILE-CONTENT-001.4` | Compatibility and failure handling, Verification |

## Caller contract

The registered `POST /api/v1/workspace/file/content` handler
`Server.handleFileUpdate` forwards `FileUpdateRequest` through `JoinRepoPath`
to `ApplyFileDiff`, including `original_hash` and `desired_content`. On success
it returns HTTP 200 with `success`, `new_hash`, `resolution`, and the requested
`path`; errors retain HTTP 400 and the existing failure response.

`performSaveFile` in `apps/web/hooks/use-file-save-delete.ts` submits a diff,
original hash, and actual desired snapshot. It accepts `success && new_hash`;
`publishSavedFile` advances the saved baseline to that submitted snapshot and
clears dirtiness when no later typing occurred. These callers need no change.
The accepted real-Git proof establishes a disk/result defect, and source
inspection establishes its editor implication. Neither proves a browser flow
or backend database persistence.

## Request-owned patch

Previously a request wrote `.kandev-patch.tmp` before waiting for Git admission.
Another request could replace those bytes, so the first applied the second patch
and returned an applied result with its own unchanged target's hash.

`writeFileDiffPatch` replaces that fixed-name staging block with `os.CreateTemp`
in the existing workspace directory, using an absolute directory path and a private
`.kandev-patch-*` pattern. The nearby `workspace_git_index.go` uses the established
create-temp/owned-cleanup pattern. Keep the created patch's default private
permissions. Write the rewritten diff through its returned descriptor and check
both write and close errors. Close the descriptor before invoking Git so Windows
can read and later remove it.

Cleanup is registered immediately after creation. Every exit closes any remaining
descriptor and best-effort removes only the path returned for that invocation.
Do not remove/recreate the file between creation and application, reuse the old
fixed filename, sweep sibling patches, or remove another request's patch.

Pass the absolute patch filename through the unchanged direct argv
`apply -p0 --unidiff-zero --whitespace=nowarn`. Retain `NewGitCommand`,
`RunGitCombinedAfterAcquire`, `GitInteractive`, the existing working directory,
and `gitCommandTimeout`. Queue wait stays outside the execution timeout under
[the shared admission decision](../../../decisions/2026-08-02-class-aware-git-subprocess-admission.md).
Unique patch ownership also works across independent trackers; a tracker-local
mutex would not provide that property.

## Compatibility and failure handling

Keep path validation, pre-save content/hash reads, symlink-header rewriting,
post-apply target read/hash, resolution values, and existing notification and
logging logic. Preserve the original-hash conflict and failed-Git
desired-content fallback branches, including nil versus empty content.
Preparation failures return errors after owned cleanup. Git cancellation or
deadline errors still bypass fallback. Other Git errors retain fallback when
provided. Post-apply read failure remains failure; this repair adds no rollback.

Same-file races, aliases of one physical target, changes to patch-header target
authorization, and the separate repository-scoped header-target candidate are
outside this design. Request isolation does not introduce those guarantees.

## Verification

Author permanent tests independently; the immutable ROOT diagnostic is evidence
only and must never be replayed, copied, or imported. Reuse `setupTestRepo`,
`runGit`, `writeFile`, and `newTestLogger` for real-Git process tests. Gate with
the existing `SetCapForTest(1)`, `AcquireGit`, and admission waiter snapshot:
queue alpha, observe its waiter, queue beta, observe both, then release. Use
matching original hashes and actual desired content in both requests. Test one
shared tracker and two independent trackers for the same workspace, plus fresh
sequential controls. After joining both requests, assert exact disk bytes,
independently computed SHA256, applied resolution, and an untouched neighbor.

Additional outcome tests cover applied, overwritten after Git failure, rejected
Git failure without fallback, and queued cancellation with desired content.
After settlement, assert no newly created patch artifacts remain and unrelated
sentinel bytes survive, including a pre-existing `.kandev-patch.tmp`. Cleanup
assertions supplement actual disk/result evidence, never replace it. Register
release/cancel/drain cleanup before any failure path; restore global admission
state only after all request goroutines and subprocesses are joined. No
`t.Parallel`, production hook, new goroutine, or mocked Git is needed.

Reuse `newGitAPIFixture`, `workspaceRequest`, `decodeWorkspaceBody`, `runGitAPI`,
and `writeFileAPI` in the API package for the same deliberate overlap through
the registered router. Assert each HTTP status/path/success/resolution/hash
against independent expected content and final disk bytes. Add sequential
registered-route controls. This proves the producer-to-HTTP outcome, without
claiming network, WebSocket, rendered-editor, or native-platform execution.

Linux Go 1.26.0 race-enabled execution passed all ten selected process tests and
five selected registered HTTP tests. This includes shared/independent tracker
overlap, fresh sequential controls, empty overwrite fallback, cancellation with
a successful peer, cleanup and unchanged neighboring bytes. The independent RED
failed on alpha's actual disk bytes and applied hash before the correction.

These Linux checks did not establish native Windows compatibility. Hosted
Windows process compilation at initial head `b96495a` failed because the new
tests reused an admission-wait helper from a `!windows` test file. The save
fixture now owns a portable context-bound waiter over
`AdmissionSnapshot().Waiters`, preserving the deliberate queue interleaving.
Native Windows success must come from the corrected head's hosted execution.

Exact selectors, actual results and serial bounds live in the
[single work order](../../../plans/prevent-overlapping-file-saves/task-01-isolate-save-patches.md).

## Presentation and documentation

No frontend, rendered composition, touch, focus, copy, or viewport behavior
changes; mobile-parity adds no UI preview or browser check to this backend-only
repair. Existing public docs describe file opening/saving without a temporary
patch contract. Restoring that behavior changes no documented workflow, option,
API shape, or terminology. Internal requirements/design/delivery records suffice.
