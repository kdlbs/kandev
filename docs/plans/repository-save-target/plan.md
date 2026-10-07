---
created: 2026-10-07
status: implemented
requirements:
  - REQ-WORKSPACES-SAVED-FILE-CONTENT-001
system_design:
  - ../../specs/workspaces/system-design/saved-file-content.md
legacy_specs: []
---

# Implementation Plan: Correct Repository Save Targets

## Overview

Save the edit to the selected repository file and return its actual saved hash.
One sequential work order independently establishes RED, corrects the path
coordinates at the producer and its immediate caller, and verifies filesystem,
registered HTTP and compatibility outcomes. ROOT later reviewed the sealed four-file package and explicitly released
implementation and the exclusive local-heavy lease in the same primary session.

## Root cause and accepted evidence

The handler joins repository scope onto the request path but leaves the diff
headers repository-relative. `ApplyFileDiff` executes Git at the workspace root
and then reads/hash-acknowledges the selected path. A same-named root file can
receive the edit while the selected file stays unsaved.

Accepted ROOT RED at baseline `72a840a860143c6b55e401fe27c92167415b1e4a`:
native32127 / initial038cc0 / terminale01e98, joined exit1 at
2026-10-06T22:18:18Z. Root control passed; selected beta stayed unchanged while
root changed, with HTTP success/applied/old selected hash. This was a plain
filesystem fixture using real Git and the registered API/Manager, without
independently initialized Git metadata or browser execution. The immutable proof
and exact receipt identity are in this task's platform plan; never replay, copy,
import, modify or remove it.

Admission HEAD is `1b70c91a412361458a640b9fe25a8093ebc043f6`, after PR4280
actually merged per ROOT admission. Source inspection confirms its private patch
helper and unchanged target/header handling. Independent permanent process and registered-API RED tests subsequently reproduced
the defect on this admitted HEAD before production edits; see the work order.

## Scope and technical approach

- Carry submitted `diffPath` and admitted `reqPath` explicitly into internal
  `ApplyFileDiff`; normalize matching file headers before existing symlink
  handling. The API keeps `JoinRepoPath` and the root tracker. Mechanically
  update direct tracker test calls to provide the same path twice.
- Preserve the merged `writeFileDiffPatch` ownership, descriptor lifecycle,
  cleanup, direct Git argv/admission/cancellation, fallback and hash behavior,
  notifications, HTTP/WS schemas and response path.
- Own only `workspace_files.go`, the immediate `workspace.go` call, their
  causal test call sites and focused process/API regressions. Exclude general
  patch authority, StageAll artifacts, producer refactors, frontend/layout,
  storage, runtime flags, dependencies and unrelated backend work.

## Tests and traceability

All criteria below use prefix `AC-WORKSPACES-SAVED-FILE-CONTENT-001`.

| Criteria | Outcome evidence |
| --- | --- |
| .2 | New `TestApplyFileDiff_RepositoryTarget` and `TestHandleFileUpdate_RepositoryTarget`: real Git, same relative filenames, selected bytes/hash and untouched neighbors, root/alpha/beta controls, genuine editor headers and nil desired content |
| .2, .3, .4 | New `RepositoryTargetCompatibility` tests in both packages: nested/header-like content, scoped symlink, stale hash and patch failure with nil/empty/nonempty desired content, queued cancellation, truthful error/overwrite/applied results and write-event identity |
| .1-.4 | Existing overlapping-save, sequential, patch-cleanup, cancellation, regular/symlink/conflict and route rejection tests; exact selectors in work order |

## End-to-end operation evidence

Registered `POST /api/v1/workspace/file/content` plus real Manager and Git proves
the disk-to-HTTP outcome. Source-only traces cover desktop, tablet and Review
requests through WS/client forwarding. No executed browser, WebSocket network or
mobile claim. There are no rendered or frontend changes, so no ASCII preview or
Playwright work is scheduled. The owning design records the mobile and public
docs assessments. Existing public editing/saving instructions remain accurate.

## Work orders

- [x] [Task 01: Correct the repository save target](task-01-correct-save-target.md)
  (`done`, including the bounded Windows fixture correction;
  sequential; no internal dependency)

## Verification results

Lightweight design preflight passed: catalog validation (357 decisions, 1416
specifications), all 36 specification-linter tests, full specification lint,
catalog discovery, whitespace and prospective `validateCoverage` (`covered`,
errors[]). Original native15361 / initial18a80d / terminalcaf8d2 joined exit0;
all seven preflight subprocesses were waited and their groups observed gone.
Receipts/logs: `/tmp/kandev-child64-design-receipts.jsonl` and its per-command
log paths. This is document evidence, not product verification.

At design handoff product checks were deferred, with the lease held by child63.
ROOT subsequently released implementation and exclusive local-heavy ownership
to child64. Independent RED and GREEN completed; the final race-enabled process
selector passed 16 top-level tests and the API selector passed seven, without
skips on Linux. Scoped lint passed with zero issues. Final document/reference preflight passed: catalog357/1416, all36
spec-linter tests, full specification lint, actual changed-path coverage
(`covered`, errors[]) and whitespace.
Exact actual results and original handles are in the work order and platform plan.

## Risks and delivery

The Windows fixture correction is uncommitted. Its affected process/API race
regressions passed all 24 target cases and compatibility controls, with no Linux
skips, and scoped lint reported zero issues. Required full changed-code lint
against live base `aad793bfa93e637e74d5d74d4cf194bc3d02f23b` timed out at the
six-minute outer bound with exit124 and no diagnostics. Its original was joined
and all four corrective local command groups/wrappers are gone. ROOT authorized
exactly one identical recovery with retained caches; it passed with zero issues
and joined exit0 at 2026-10-07T05:34:49.992631Z. Its group and wrapper are gone;
the first timeout remains failed with unknown cause. Normal-hook fixup
publication is authorized. The original hosted collector then joined exit1 with
53 passed, two failed, 18 skipped, one neutral and no pending checks. Its handle
and original deadline receipts remain retained; no successor is authorized.

The real formatter includes empty tab suffixes; comparing whole header lines
would miss them. Hunk text must remain unchanged. Switching to a repository
tracker would change event/admission behavior, so retain the root tracker.
Linux checks cannot prove native Windows behavior; require affected hosted
Windows success at the actual published head. Symlink skips must be reported.

The platform task plan owns exact resource, receipt, review and delivery gates.
The later ROOT release granted implementation and local-heavy ownership.
Merge still requires a separate ROOT grant.
No new ADR is needed: this restores the existing requested-target invariant.
