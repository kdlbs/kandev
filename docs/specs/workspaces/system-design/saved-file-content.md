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
| `AC-WORKSPACES-SAVED-FILE-CONTENT-001.5` | Request-owned patch, Save and staging verification |

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

The reviewed baseline gives each request a unique `.kandev-patch-*` file
through `writeFileDiffPatch`. This prevents one save consuming another save's
patch, but creates the file in the workspace before Git admission. A queued
`GitOperator.Stage(ctx, nil)` can execute `git add -A` while that private patch
exists, leaving it in the real index after save cleanup removes the file.

The correction sends the already rewritten request string directly to
Git. Inside the existing post-admission command builder, set
`cmd.Stdin = strings.NewReader(unifiedDiff)` and use direct argv
`apply -p0 --unidiff-zero --whitespace=nowarn -`.
[Git's documented `-` input](https://git-scm.com/docs/git-apply) consumes stdin.
Remove the sole patch-file helper and its caller's removal defer. No patch file
is created, opened, recreated or swept anywhere. The existing immutable request
string remains private to that invocation, including across independent trackers.

`NewGitCommand`, `RunGitCombinedAfterAcquire`, `PrepareGitCommand` and managed
Start/Wait retain supplied stdin. The managed lifecycle sets output wiring,
environment policy, process ownership and pipe wait bounds; it does not replace
the reader. `exec.Cmd` owns copying the finite reader and joining its pipe work
when waiting. No manual stdin pipe, writer goroutine, shell, new admission
layer or subprocess wrapper is required. Cancellation continues through the
existing managed process lifecycle on Unix and Windows.

Keep `GitOperator.Stage` and registered stage/save caller contracts unchanged.
Do not filter dotfiles, alter ignores, serialize all writers or mutate the
index to undo a leaked artifact. Relocating to the native default temp directory
would still permit a caller's `TMPDIR`, `TMP` or `TEMP` to place private patches
inside a checkout. Stdin needs no such filesystem boundary and avoids Windows
open-file/delete sharing concerns. The Git index snapshot helper stores its
own read snapshots next to the index in Git metadata; it has a different
consumer and remains outside this correction.

Retain `NewGitCommand`,
`RunGitCombinedAfterAcquire`, `GitInteractive`, the existing working directory,
and `gitCommandTimeout`. Queue wait stays outside the execution timeout under
[the shared admission decision](../../../decisions/2026-08-02-class-aware-git-subprocess-admission.md).
This is a local patch-input correction. The requirement and this design retain
its scope and rationale; it does not require a separate ADR or global writer
framework.

## Compatibility and failure handling

Keep path validation, pre-save content/hash reads, symlink-header rewriting,
post-apply target read/hash, resolution values, and existing notification and
logging logic. Preserve the original-hash conflict and failed-Git
desired-content fallback branches, including nil versus empty content.
There is no disk patch-preparation failure or artifact cleanup after the
correction. Existing user-owned patch-like files must remain untouched. Git cancellation or
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

## Save and staging verification

Current-source inspection at `905fa03c5b8ae90c661dc9fec2e324d355e269aa`
confirms workspace patch creation before admission and Stage All's `add -A`.
It includes merged repository-target dependency
`9b4af97250f491d69d14629b549b3f33ddc66c12`; its producer blob is unchanged
between that dependency and this head. These are static findings.

ROOT's archived diagnostic at baseline
`62b39941214ffe63ce72d307b6e599a7bd2a7b63` used the fixed
`.kandev-patch.tmp` helper. Stage All was queued first under held capacity one,
then a valid save second. Both operations succeeded with correct final file
bytes and save hash, but the real index included the private patch. A fresh
sequential save then Stage All control passed. Native session `68425`, initial
chunk `b5a4f6`, terminal chunk `08b5ef` and the protected source SHA256
`0f928ca22d6f51e72ddd4cd35ad02e55258c7e1a129cb281b3f21a153f7e5111`
identify that evidence. It was read only, never replayed or imported here.
It is not executed proof of today's random helper, HTTP, browser or commit flow.

After a later implementation release, independently author permanent real-Git
process and registered-route regressions. Hold capacity one, queue Stage All,
observe one waiter, start the actual current save producer with matching hash
and valid patch, observe the second waiter, then release. Register cancellation,
release and joining before failure paths; restore admission only after all
requests settle. Assert the exact real index path set and indexed blob contents,
both operation results, final requested bytes and independently computed hash,
and untouched neighbors. The earlier stage must retain the original file blob;
the later save can remain unstaged. This is the queue's FIFO behavior, not an
atomic save/stage promise. Include legitimate untracked patch-like dotfiles,
ordinary additions and a tracked deletion so filtering or losing Stage All
semantics cannot pass. No prefix-only artifact assertion is sufficient.

Use fresh sequential controls to prove saved content is staged, plus a finite
patch larger than a pipe buffer with non-ASCII content and no desired-content
fallback to prove stdin application, rather than silent overwrite. Test native
temp environment values pointing inside the fixture checkout. Preserve existing
distinct-file, fallback, cancellation, scoped repository and symlink regressions.
The registered route test proves save ACK/path/hash and Stage All against actual
disk and index; a scoped save must preserve same-named nonselected files. No
browser, commit or native Windows execution is claimed until it actually runs.

The [private-save-patch work order](../../../plans/private-save-patches/task-01-private-patch-input.md)
owns exact commands and later results. Native hosted Windows execution of its
portable process/route cases remains a delivery gate; cross-compilation alone
does not prove native behavior.

After ROOT's later implementation release, independently authored permanent
regressions reproduced the actual random-helper leak at the reviewed baseline:
four overlap cases failed solely on extra real-index entries while both save
and stage succeeded with correct saved bytes/hash/ACK. Fresh process and
registered-route sequential controls passed. This independent RED is distinct
from ROOT's fixed-name archive and never imports it.

The stdin correction then passed all 13 selected process tests (60 cases) and
six registered API tests (47 cases) under race-enabled Linux Go 1.26.0, without
skips. The new cases assert exact index paths/blobs, working-tree bytes and
truthful ACKs, genuine user patch-like dotfiles/additions/deletions, selected
repository bytes, and a large non-ASCII patch without overwrite fallback while
all native temp variables point into the checkout. Existing fallback, queued
cancellation, distinct-file and symlink controls passed. This establishes real
Git and registered-router outcomes, without claiming browser, commit-flow or
native Windows execution. Retained identities and exact commands live in the
work order; hosted native-platform evidence remains pending its separate release.

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
