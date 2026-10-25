---
created: 2026-10-07
status: in_progress
requirements:
  - REQ-UI-WORKSPACE-FILE-TRANSFER-001
  - REQ-UI-WORKSPACE-FILE-TRANSFER-003
  - REQ-UI-WORKSPACE-FILE-TRANSFER-004
system_design:
  - ../../specs/ui/system-design/workspace-file-transfer.md
legacy_specs: []
---

# Implementation Plan: Preserve permissions when replacing uploads

## Overview

Preserve the supported ordinary permissions of an existing regular destination when a streamed
Replace upload publishes complete incoming bytes. Deliver one bounded sequential work order in
the same primary session. The existing workspace-file-transfer pair owns this extension because
it already defines Replace, Keep Both, streamed publication and failure handling; do not create
a second filesystem or UI specification. ROOT reviewed the DESIGN package before later implementation.

The requested outcome and exclusions are settled by ROOT. Source inspection verifies the cause;
there is no material product question requiring an operator or model approval prompt. The local
permission rule and its rationale fit the existing design, satisfying `/record` without a new ADR.

## Confirmed evidence

At baseline `dabc68fc15f6ecabe01e62c146eab1da87256131`, `createUploadTemp` requests `0644`
subject to umask. `WriteFileStream` closes that inode and renames it over the destination,
discarding the destination's mode. ROOT accepted two causal failures: successful executable
replacement loses `0755` and direct native launch fails; restricted replacement changes
`0600` to `0644`. Correct bytes, returned path/count and success were verified. New-file and
Keep-Both controls passed under deterministic POSIX umask `0022`.

Read-only proof: `/tmp/kandev-root-upload-mode-discovery-20261007/{receipt,native,classification}.json`
and `proof.log`. Original native handle `19106`, chunks `650ac1` to `cdd6be`, actually joined
exit `1`; accepted classification records gone owned groups and clean ROOT worktree. Protected
`0400` source `/tmp/kandev-root-upload-replacement-mode-candidate_test.go` has SHA256
`68d4c33eb4774a39621dc821ce5344e0e76234ef7dd35c92e8af4df8ead0cbf3`.
Read/hash inspection only: never replay, copy, import, mutate or delete these artifacts. The
older `qualification.json` is source-qualified/unrun; ROOT separately reports synchronous
qualification chunk `62b532`, exit `0`, fresh groups gone/root clean. Do not invent its metadata.
There is no executed registered HTTP, browser, native Windows, ACL, owner or exploit proof.
Permanent regressions must be authored independently after the later execution grant.

## Scope

In scope: amend the existing [requirement](../../specs/ui/requirements/workspace-file-transfer.md)
and [design](../../specs/ui/system-design/workspace-file-transfer.md#replacement-permissions),
the upload-specific writer, real filesystem regressions and registered upload-route tests.

Exclude shared `workspace_files.go`, child67's save producer, general no-clobber rename,
streaming artifacts, Git staging, metadata APIs, ownership/ACL/xattr/privilege-bit copying,
whole-file buffering, frontend/layout/copy changes, new dependencies, flags and extra work orders.
Changes overlapping those boundaries require a concrete ROOT checkpoint. Historical companion
packages `workspace-file-transfer` and `upload-owner-lifetime` retain their completed results and
scopes; this repair does not reopen their browser matrices.

## Technical approach

Follow the paired design's [replacement permission flow](../../specs/ui/system-design/workspace-file-transfer.md#replacement-permissions).
Keep the staging descriptor open until final target resolution under `mutationMu`. Carry the
existing rooted target `FileInfo` into a nullable ordinary-mode selection for existing regular
Replace only; zero mode is a real value. Apply it through the staging descriptor, sync/close,
then retain the rooted rename and existing notification/reporting. Source reads stay outside
the lock. Do not chmod new/Keep-Both files to a fixed mode, or chmod the destination itself.
A small private upload publication helper can keep the function within backend limits and
allow real descriptor-failure coverage; no generic publisher or injectable filesystem framework.

Caller audit at this baseline: the sole production `WriteFileStream` caller is
`Server.writeWorkspaceUploadPart`, through the registered agentctl upload route and its bounded
size-checking reader. Private target/temp helpers are called only by this writer. The session
HTTP route and runtime client forward bytes/resolution and do not supply modes. Stable in-root
symlinks are canonicalized by the existing resolver; registered source roots retain their
authority. Rooted operations continue to reject escapes, including directory swaps. No new
external-process or inode transaction guarantee is made between final stat and rename.

| Environment/entry point | Behavior and verification | Limit |
| --- | --- | --- |
| Native POSIX tracker | Preserve ordinary mode and directly launch the replaced script; restricted/zero modes and restrictive umask controls | Filesystem must support these bits; permission-denial tests probe enforcement |
| Registered agentctl HTTP route | Authenticated multipart through `Server.Router()` and the actual tracker; response path/count/resolution plus native bytes/mode checks | Registered router/real-tracker evidence is recorded below; no full session-network/browser claim |
| Native Windows tracker | Common path/bytes/conflict controls plus writable/read-only outcome or native failure with preserved original and cleanup | Only Go-supported owner-write attribute; no POSIX script/mode, ACL or universal atomic-rename claim |
| Session route/client, desktop/phone | Same existing resolution/streaming wire contract reaches the writer | Read-only caller audit; no claim of executed full browser/session-network proof |

## Tests

All short criterion suffixes below mean `AC-UI-WORKSPACE-FILE-TRANSFER-<suffix>`.
Permanent tests follow this matrix; execution results and native limits are recorded below.

| Criteria | Permanent evidence |
| --- | --- |
| `004.7`, `003.4` | `process/workspace_upload_permissions_test.go`: `TestWriteFileStreamReplacementPermissions`, with real script direct launch before/after, `0755`, `0600`, `0000`, exact content/path/count and success; POSIX-only assertions explicitly gated |
| `004.7`, `004.8`, `004.6` | Same file: `TestWriteFileStreamReplacementCurrentTarget`, channel-gated reader changes mode, removes target, or creates a conflict during streaming; verify final-check mode/default or refusal and both old/new bytes |
| `004.8`, `004.4` | `process/workspace_upload_umask_unix_test.go`: `TestWriteFileStreamUploadUmaskCompatibility` and bounded marked `TestWriteFileStreamUploadUmaskHelper`; isolated children with masks `0022`/`0077`, new/Keep-Both/missing Replace controls, no parent umask change |
| `004.9`, `001.6`, `003.4`, `003.6` | `TestWriteFileStreamReplacementFailures`: failed reader, directory/target-check rejection, real closed staging descriptor through the upload publication helper, and rename failure; original content/mode unchanged, no temp or success notification |
| `003.1`, `004.7` | `TestWriteFileStreamReplacementContainment`: accepted stable in-root file link uses canonical destination mode; outside file/directory links and deterministic post-resolution directory swap leave outside bytes/mode untouched; existing registered-source rules stay covered |
| `004.7` through `004.9`, `003.6` | `process/workspace_upload_permissions_windows_test.go`: `TestWriteFileStreamReplacementWindowsPermissions`; native writable/read-only cases, successful complete writes where supported or native refusal preserving original and cleaning stage |
| `004.7`, `004.8`, `004.9`, `004.6`, `003.1` | `api/workspace_upload_permissions_test.go`: `TestHandleFileUpload_ReplacementPermissions` and `TestHandleFileUpload_ReplacementCompatibility`; registered route/real tracker, executable and restricted Replace on POSIX, new/Keep-Both/409/size-mismatch/containment controls, correct status/body/bytes/modes and no temp on pre-publication failure |

Reuse existing `TestWriteFileStream*`, `TestCheckUploadConflicts`, `TestHandleFileUpload*`,
`TestHandleUploadPreflight*`, registered-source and descendant-symlink-swap regressions.
Assert special privilege flags are not transferred using actual setup where the native
filesystem supports them. Register cleanup before failure paths, release/join every gated
reader/subprocess, and avoid sleeps or changing the test runner's global umask. Windows bytes
tests remain enabled; skip only unavailable POSIX semantics. Existing hosted Windows process
tests can provide native tracker evidence; agentctl API is not in that Windows allowlist, so
do not imply native Windows registered-route coverage without separately admitted evidence.

## Mobile and public documentation applicability

`/mobile-parity` is assessed: only backend filesystem data changes. Desktop and phone share the
same upload choices and wire path; there is no rendered, touch, navigation, scrolling or
breakpoint change. Use the data-only exception with filesystem/registered-route evidence.
No browser/E2E/build, viewport or localization check is causal to this repair.

`/docs-maintainer` search found no dedicated Files-transfer public page or permission guarantee
in `docs/public`, root README or screenshot catalog. This repair updates the owning internal
specs/plans. A future Files-panel how-to can explain that Replace retains ordinary supported
destination permissions and Keep Both uses new-file defaults; no new public page is warranted
for this local fix alone. No command/config/API shape/label/navigation/screenshot changes.

## Work orders

- [ ] [Task 01: Preserve upload replacement permissions](task-01-preserve-replacement-permissions.md)

One work order, sequential, no predecessor. Same primary session; no delegation or extra tasks.
ROOT granted implementation and exclusive GLOBAL LOCAL-HEAVY on 2026-10-07 after the
reviewed design and authoring checkpoints. The MCP task plan holds the standing execution, delivery and completion gates.

## Verification results

ROOT reviewed the four-artifact DESIGN package before authoring and later full LOCAL release
in this same primary. Original public-writer RED native53618 and registered-route RED native78424
actually joined exit1 before production edits: successful uploads lost0755/0600 and direct native
script launch failed. Writer controls also reproduced0400/0640/0000 becoming0644.

Final scoped writer GREEN native8542 and registered-route GREEN native24575 joined exit0 after
the final production edit, with the exact [work-order commands](task-01-preserve-replacement-permissions.md#verification).
They cover real scripts/modes/bytes, final-target changes during streaming, isolated umasks0022/0077,
new/missing Replace/Keep Both, failure cleanup and notifications, special-bit exclusion, stable
in-root links, registered sources and escape/swap rejection. Real closed-descriptor publication
cases cover chmod/sync failures and a subsequent same-tracker lock-release control.

Scoped lint native56169 joined exit0 with0issues, against the recorded baseline with concurrency2,
serial runners, CLI5m/GNU6m/kill10. Its first run found two test-only switch suggestions, corrected
before final checks. Catalog357decisions/1418specifications, all36spec-linter tests, all-spec lint
and whitespace passed. One frozen apps install completed with existing pinned pnpm9.15.9.

Original logs, UTC/argv/cwd/cutoffs/PIDs/groups and actual joins/fresh gone proofs are retained in
`/tmp/kandev-child69-upload-permissions-20261007T092136Z/` and the task's durable MCP plan.
Owning requirements/design are active/current. Work-order delivery remains in progress through
normal hooks, ready publication and separate ROOT hosted/merge releases. No broad local suite,
extra full lint, browser/E2E/build or native Windows execution is claimed. Hosted Windows tracker
evidence remains required; its allowlist does not establish registered-route Windows coverage.

## Risks

- Preflight/start-time modes can be stale. Only the final rooted resolution supplies the snapshot;
  outside writers remain unsynchronized. Existing in-root absolute/registered-source aliases are
  not expanded or newly rejected by this repair; outside absolute paths remain escape controls.
- An unconditional chmod would defeat restrictive umask or copy executable bits to Keep Both.
  Treat zero mode as present and strip special flags through `Perm()`.
- Metadata/sync/close failures must abort before rename. Native filesystem or Windows attribute
  restrictions can refuse replacement; never clear/delete the destination to force success.
- Test/build cross-compilation is not native execution evidence. Published bytes cannot be
  promised rolled back after transport loss. Any resource/timeout/transport/unknown/out-of-scope
  issue checkpoints ROOT without automatic retry, scope expansion or moving-base rebase.

LOCAL implementation release: ROOT superseded the authoring-only limit and granted this same
primary exclusive GLOBAL LOCAL-HEAVY. Original writer RED native53618 and registered-route RED
native78424 each compiled and actually joined exit1 with successful bytes/path/count but lost
executable/restricted permissions and direct native launch permission denied. Production was
unchanged before both. The minimal upload-only correction retains final-check regular-file
Perm(), including0000, through descriptor Chmod/Sync/Close and rooted rename. Scoped writer
GREEN native89140 and route GREEN native71531 were the initial passing runs. After the final
publication-cleanup guard, writer native8542, route native24575 and scoped lint native56169
actually joined exit0 and are authoritative. The guard prevents successful publication cleanup
from deleting a subsequently reused staging name. Normal hooks, ready PR4290 publication and
physical LOCAL return then passed; their receipts remain in the durable task plan.
Retained logs, UTC/argv/cwd/cutoffs/PIDs/groups and fresh gone proofs live at
`/tmp/kandev-child69-upload-permissions-20261007T092136Z/`.
Native Windows execution, hosted CI/review, merge and ROOT closure remain separate delivery gates.

ROOT authorized one bounded test-only CI dependency correction after the hosted Build's
Playwright listing failed on the base fixture's duplicate `session` declaration. The tested merge
b87854d4 and base b3f207b2 contained the same fixture; frozen upload head9aa5bc76 retained the
older fixture. Incorporate only that fixture's base-side disabled-continuation test changes and
rename its second SessionPage binding to `recoverySession`, preserving both navigations, waits,
assertions and native trace semantics. This adds the existing
`apps/web/e2e/tests/session/provider-interruption-continuation.spec.ts` to this order's bounded
ownership. No product/provider/runtime/geometry change, rebase or backend check replay.
Validation is one-file Playwright `--list` for project chromium and applicable scoped fixture lint,
followed by normal hooks, a new fixup commit and push. ROOT retains later HOSTED/MERGE gates.


The bounded fixture correction lists all15 tests in one file under the existing chromium config:
native22452 (8dede5 to825bb9) actually joined exit0; this is collection-only evidence, with no
browser/backend/build. Scoped fixture ESLint native32034 (d35ec8 to46aa1c) actually joined exit0.
Original hosted observer62265 was stopped only after ROOT declared it obsolete, then actually
joined143 with no passing/all-terminal verdict; fresh process groups are gone. Original fixed
receipts, causal upload RED/GREEN and parser failures remain retained. Normal fixup hooks/push,
physical local return and later hosted/current-head semantic/native Windows evidence remain
separate gates.


ROOT physical-return audit4427f2 actually joined exit1 solely because immutable main1204f0e and
fixup b3619d0d have a concrete conflict in the corrected fixture. Prior local resource/blob checks
passed but no ROOT acceptance PASS or HOSTED release was claimed. ROOT authorized one normal
merge of immutable1204f0e, solely to resolve this conflict. The single conflicting fixture resolves
to exact current-main bytes with only the second SessionPage binding and its three references
renamed `recoverySession`; every other incoming main blob stays intact. Owned upload code/tests
and specifications stay unchanged. This concrete-conflict exception permits no rebase, amend,
force push, browser/build/full E2E, passing Go replay or additional source decisions.


The newly resolved fixture lists all15 chromium tests in one file: native82924 (78118b to
b9b242) actually joined exit0. Scoped fixture lint native41074 actually
joined exit0. These are collection/lint evidence only, with no browser/backend/build or
passing Go replay. Normal hooked merge commit/push and physical conflict-free return remain
pending, followed by separate ROOT hosted and serial merge releases.


ROOT accepted the prior hooked main incorporation and physical return at published head
`c2f9b72b`. Its configured full review covered ten files, and the native Windows writable/read-only
cases actually ran and passed without skips. Those are historical head-specific results. Original
observer73592 joined exit2 at its unchanged 90-minute deadline with pending E2E work and no passing
verdict. ROOT later declared replacement76533 obsolete; it was stopped and joined143, with fresh
owned groups gone and no hosted workflow cancellation.

ROOT independently proved a second actual conflict against immutable main `330e02a4` and released
one normal merge solely for that fixture. Resolve the shared fixture to exact incoming blob
`1140158f5d5920aac805c3432d7c788316318757`: upstream now owns the prevention-setting restoration
and cleanup fix, superseding our rename-only correction. Preserve all other incoming main blobs.
Relative to this main, the upload package has nine paths: five Go files, the owning specification
pair and these two evidence documents. The seven code/test/specification blobs stay exact `c2f9b72b`;
only these two records receive the authorized incorporation evidence updates. Bounded static
inspection found the immediate writer and registered API caller unchanged by incoming main.

Validate only the affected file's chromium collection (file argument first), scoped fixture lint,
actual nine-path documentation coverage and normal merge hooks. No browser/build/full E2E or
passing Go/PG replay. Publish the new normal merge and return physical resources before any later
ROOT release of new-head hosted review/CI or serial merge. Old-head approval is not current approval.


Current incorporation validation: file-first chromium collection native90220 (5f433f to cac615)
actually joined exit0 and listed all15 tests in one file. Scoped ESLint native52648 (ae829d to
6dc452) actually joined exit0. Actual nine-path documentation coverage (b37acf, synchronous
joined0) is covered with no errors and one bounded work order. These are collection/lint/coverage
results, not browser, backend-build or full E2E execution. Normal hooks, corrected push, physical
return and later new-head hosted/serial merge gates remain separate.
