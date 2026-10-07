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
`apps/backend/internal/agentctl/server/process/workspace_files.go`, with immediate
file-update handler wiring. The existing requirement already covers repository
selection through the requested-target and no-neighbor criterion (.2); no new
requirement or path authority is introduced. Editor-file-containment covers lexical editor admission, while
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
| `AC-WORKSPACES-SAVED-FILE-CONTENT-001.2` | Caller contract, Repository save target, Request-owned patch, Verification |
| `AC-WORKSPACES-SAVED-FILE-CONTENT-001.3` | Compatibility and failure handling |
| `AC-WORKSPACES-SAVED-FILE-CONTENT-001.4` | Compatibility and failure handling, Verification |

## Caller contract

The registered `POST /api/v1/workspace/file/content` handler
`Server.handleFileUpdate` forwards `FileUpdateRequest` through `JoinRepoPath`
to `ApplyFileDiff`, including `original_hash` and `desired_content`. The
repository-target correction also carries the submitted repository-relative
path separately from that admitted workspace-relative path. On success
it returns HTTP 200 with `success`, `new_hash`, `resolution`, and the requested
`path`; errors retain HTTP 400 and the existing failure response.

`performSaveFile` in `apps/web/hooks/use-file-save-delete.ts` submits a diff,
original hash, and actual desired snapshot. It accepts `success && new_hash`;
`publishSavedFile` advances the saved baseline to that submitted snapshot and
clears dirtiness when no later typing occurred. These callers need no change.
The accepted real-Git proof establishes a disk/result defect, and source
inspection establishes its editor implication. Neither proves a browser flow
or backend database persistence.

## Repository save target

The admitted path and patch headers currently use different coordinates:
`JoinRepoPath("beta", "one.txt")` selects `beta/one.txt`, while the patch still
names `one.txt` and Git executes at the workspace root. A matching root file can
therefore receive the edit, followed by a success hash read from unchanged beta.
The merged request-owned patch helper fixes temporary-file isolation only.

Carry an explicit `diffPath` into the internal `WorkspaceTracker.ApplyFileDiff`
call alongside `reqPath`: the API passes `reqPath=scopedPath` and
`diffPath=req.Path`.
Direct tracker consumers with workspace-relative headers pass the same path for
both. After existing target admission and hash-conflict handling, translate only
matching submitted file headers from `diffPath` to `filepath.ToSlash(reqPath)`.
Then run existing symlink resolution, which translates the admitted path to its
resolved target. All content reads, fallback writes and notifications continue
using the admitted request identity. Keep Git's working directory and argv.

Reuse the local header-rewrite boundary in `workspace_files.go`. Separate the
header filename from a tab-delimited suffix, preserving that suffix. Restrict
translation to file headers, preserving hunk bytes even when deleted/added text
resembles a header. Retain the existing prefixed symlink-header forms and
unchanged behavior for unmatched paths. This is coordinate translation for the
existing single-file operation, not new multi-file patch authorization.

Source inspection of locked `diff` 8.0.3 confirms the editor's real shape:
`Index:` and separator preamble, unprefixed `--- path\t` / `+++ path\t` headers
because `generateUnifiedDiff` supplies empty header strings, and three lines of
context. Both `performSaveFile` and tablet `handleFileSave` send desired content;
Review's `revertBlock` uses the same formatter and omits desired content. All
flow through `updateFileContent`, `wsUpdateFileContent`, the agentctl HTTP client
and the registered handler. Canvas scaffold calls use workspace-relative paths
and an empty repository. These transport consumers need no contract changes.

The correction and its verification are recorded in the
[repository-target work order](../../../plans/repository-save-target/task-01-correct-save-target.md).
Use independently authored real-file process and registered-route tests with
the genuine formatter shape plus ordinary no-tab headers. Seed the same relative
filename under root, alpha and beta with matching edited/context lines and
different distant sentinels. Cover a plain task root and separately initialized
alpha/beta Git repositories, selecting each repository on fresh fixtures and
including a root positive control. Assert actual selected and nonselected
bytes, independent SHA256, applied resolution, HTTP request/response path and
immediate root-tracker write-event identity. A fallback result cannot prove
successful patch targeting; include a nil-desired-content consumer control.
Nested paths, header-like hunk text and scoped symlinks exercise the translation
itself. Preserve existing cancellation, conflict and private-patch checks.

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

Same-file races, aliases of one physical target and changes to patch-header
target authorization remain outside this design. Coordinate translation does
not introduce those guarantees.

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
Hosted native Windows process execution subsequently passed after this fixture
correction. A later retained-outcome correction in the same work order requires
fresh native Windows success at its own published head.

Exact selectors, actual results and serial bounds live in the
[single work order](../../../plans/prevent-overlapping-file-saves/task-01-isolate-save-patches.md).

## Presentation and documentation

No frontend, rendered composition, touch, focus, copy, or viewport behavior
changes; mobile-parity adds no UI preview or browser check to this backend-only
repair. This is not reliance on a frontend state/data exception. Public
`developer-tools.md`, `sessions-and-review.md`, the root README and screenshot
catalog describe repository-aware editing and existing saving behavior without
a patch-coordinate or temporary-file contract. Restoring that behavior changes no documented workflow, option,
API shape, or terminology. Internal requirements/design/delivery records suffice.

## Retained-outcome dependency during delivery

The file-save contract and implementation remain unchanged by the separately
released backend shutdown correction in the same work order. Retained-outcome
synchronization is owned by the existing
[Executors design](../../executors/system-design/agent-survival-across-restart-03.md#turn-outcome-across-the-detached-gap),
which isolates recorder wiring from lifecycle shutdown locking. The startup
artifact's causal interleaving remains unproved; the order independently tests
terminal publication while lifecycle ownership is held.
