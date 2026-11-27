---
id: "01-preserve-replacement-permissions"
title: "Preserve upload replacement permissions"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-WORKSPACE-FILE-TRANSFER-001
  - REQ-UI-WORKSPACE-FILE-TRANSFER-003
  - REQ-UI-WORKSPACE-FILE-TRANSFER-004
acceptance_criteria:
  - AC-UI-WORKSPACE-FILE-TRANSFER-001.6
  - AC-UI-WORKSPACE-FILE-TRANSFER-003.1
  - AC-UI-WORKSPACE-FILE-TRANSFER-003.4
  - AC-UI-WORKSPACE-FILE-TRANSFER-003.5
  - AC-UI-WORKSPACE-FILE-TRANSFER-003.6
  - AC-UI-WORKSPACE-FILE-TRANSFER-004.4
  - AC-UI-WORKSPACE-FILE-TRANSFER-004.6
  - AC-UI-WORKSPACE-FILE-TRANSFER-004.7
  - AC-UI-WORKSPACE-FILE-TRANSFER-004.8
  - AC-UI-WORKSPACE-FILE-TRANSFER-004.9
system_design:
  - ../../specs/ui/system-design/workspace-file-transfer.md
---

# Task 01: Preserve upload replacement permissions

## Summary

Repair regular-file Replace in the upload writer while preserving streamed, rooted publication
and ordinary new/Keep-Both defaults. Independently authored filesystem/script and registered-route
regressions prove both the repair and unchanged compatibility behavior. ROOT reviewed the
DESIGN package, then granted authoring and later implementation with exclusive GLOBAL LOCAL-HEAVY.

## In scope

- `workspace_upload.go` only for production: carry the final locked target check's existing
  `FileInfo` to an optional `Perm()` selection, apply it to the still-open staging descriptor,
  sync/close before rooted rename, preserve cleanup and notification ordering. Missing and
  Keep-Both targets receive no inherited mode; preserve zero mode. A small upload-only publication
  helper may encapsulate this lock/failure boundary. Keep source reads outside `mutationMu`.
- Author the [manifest's test matrix](plan.md#tests): direct native script and real mode/content
  controls, current-target changes during a channel-gated stream, new/Keep-Both/missing Replace,
  failures before publication, special-bit exclusion and rooted/symlink/source-root compatibility.
  Isolate umask `0022`/`0077` in bounded marked test-executable children; never change the parent
  mask or use parallel execution for those helpers. Release/join every gated reader and child.
- Registered `Server.Router().ServeHTTP` with `newUploadTestServer`/`postUpload` and actual tracker:
  authenticated multipart, exact `201` response/path/count/resolution and bytes/modes, plus
  new/Keep-Both/409/size-mismatch/containment controls. Direct handler or mocked tracker is
  insufficient. Check original mode/content, staging cleanup and notifications on failures.
- Native Windows process tests cover Go-supported writable/read-only outcomes or native refusal
  preserving original and cleaning staging. Common content/path tests stay enabled; gate only
  POSIX assertions. Permission-denial controls probe enforcement first. Cross-compilation is
  not native evidence. Register all owned cleanup before assertions.

## Out of scope

Shared `workspace_files.go` or its permanent tests, save producers/child67 overlap, general
no-clobber rename, upload HTTP production handlers, client/session wire APIs, metadata frameworks,
ownership/ACL/xattr/special-bit copying, destination read-only clearing or deletion, streaming
artifact/Git staging changes, new packages/dependencies, frontend/copy/layout/localization,
browser/E2E/build, installs/product checks in DESIGN, or extra work orders. Any overlap or new
contract requires a concrete ROOT checkpoint. Accepted ROOT proof remains read-only and is
never replayed/copied/imported/mutated/deleted. Preserve foreign state and managed resources.

## Acceptance

1. `004.7`: independently authored RED establishes successful Replace breaks direct script launch
   and drops destination permissions; GREEN preserves real supported modes with exact uploaded
   bytes/path/count and success through the tracker and registered upload route. Final-check
   metadata is used; special flags/ownership/ACLs/xattrs are not transplanted.
2. `004.8`, `004.4`, `004.6`: new, vanished-target Replace and Keep-Both files retain umask-limited
   defaults, including `0077`, and the original Keep-Both file is unchanged. Unresolved conflicts,
   directory refusal, scoped names and rooted containment retain their behavior.
3. `004.9`, `001.6`, `003.4` through `003.6`: pre-publication failures retain old content/mode and
   clean staged files without success notification. Stream reads stay outside the publication
   lock; complete bytes are published by the existing native rename path. No rollback is promised
   after a committed write suffers later transport failure; native Windows limits are reported.

## Verification

Run nothing below under DESIGN except the separate documentation block. After both later grants,
mark this work order `in_progress`, load `/tdd` and backend test guidance, and author the permanent
regressions without importing ROOT's candidate. From repo root, in non-login Bash, the first two
commands are separate expected-RED invocations before production edits (retain and join each):

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH"
export GOMAXPROCS=2 GOMEMLIMIT=512MiB
(cd apps/backend && timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=4m -run '^TestWriteFileStreamReplacementPermissions$' ./internal/agentctl/server/process)
(cd apps/backend && timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=4m -run '^TestHandleFileUpload_ReplacementPermissions$' ./internal/agentctl/server/api)
```

Inspect the expected causal assertion failures. Compiler/resource/timeout/transport/unexplained
failure is not RED proof: checkpoint ROOT without automatic retry. Implement the bounded design,
then run these task-defined GREEN checks sequentially, once on the resulting changes:

```bash
set -euo pipefail
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH"
export GOMAXPROCS=2 GOMEMLIMIT=512MiB
(cd apps/backend && timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=4m -run '^(TestWriteFileStream.*|TestCheckUploadConflicts|TestWorkspaceFileOperationsAllowRegisteredLinkedSource|TestWorkspaceFileOperationsWithNoAllowedSourceRootsFailClosed|TestWorkspaceFileMutationsRejectDescendantSymlinkSwap)$' ./internal/agentctl/server/process)
(cd apps/backend && timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=4m -run '^(TestHandleFileUpload_.*|TestHandleUploadPreflight.*)$' ./internal/agentctl/server/api)
(cd apps/backend && timeout --kill-after=10s 6m golangci-lint run ./internal/agentctl/server/process ./internal/agentctl/server/api --new-from-rev=dabc68fc15f6ecabe01e62c146eab1da87256131 --concurrency=2 --allow-serial-runners --timeout=5m)
git diff --check
```

The initial lint comparison is the recorded baseline, subject to ROOT's reviewed base checkpoint;
do not fetch/rebase moving main. Format only owned Go files before checks/active hooks. No full
local backend suite, frontend/build/E2E or second generic audit is part of initial publication.
Only an actual subsequent backend fixup adds one full CHANGED `golangci-lint run ./...` against
the exact GitHub API PR base, concurrency 2/serial runners, CLI 5m/GNU 6m/kill 10s, with
`GOMAXPROCS=2 GOMEMLIMIT=1GiB`, once under the separate heavy grant.

Applicable native Windows tracker evidence is supplied by the hosted Windows process suite;
registered-route native Windows execution is not covered by its allowlist. On a ROOT-admitted
native Windows runner the targeted command is `go test -trimpath -tags fts5 -race -p=1 -count=1
-timeout=4m -run '^TestWriteFileStreamReplacementWindowsPermissions$' ./internal/agentctl/server/process`
from `apps/backend`, with the same Go resource caps and a native bounded/joined wrapper. Do not
add a workflow or claim this command was run in the Linux worktree.

Cheap design/documentation validation, from repo root in non-login Bash using the above existing
Node/Go PATH, is authorized now:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/upload-replacement-permissions
git status --short -- docs/specs/ui docs/plans/upload-replacement-permissions
node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [...new Set([
  ...execFileSync('git', ['diff', 'HEAD', '--name-only', '-z']).toString().split('\0'),
  ...execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z']).toString().split('\0'),
].filter(Boolean))];
const documents = [
  'docs/specs/ui/requirements/workspace-file-transfer.md',
  'docs/specs/ui/system-design/workspace-file-transfer.md',
  'docs/plans/upload-replacement-permissions/plan.md',
  'docs/plans/upload-replacement-permissions/task-01-preserve-replacement-permissions.md',
];
const fileContents = Object.fromEntries(documents.map(p => [p, fs.readFileSync(p, 'utf8')]));
for (const [label, changedFiles] of [
  ['actual diff', paths],
  ['planned production coverage', [...new Set([...paths,
    'apps/backend/internal/agentctl/server/process/workspace_upload.go'])]],
]) {
  const result = validateCoverage({ changedFiles, fileContents });
  console.log(JSON.stringify({ label, ok: result.ok, status: result.status,
    errors: result.errors, workOrders: result.workOrders,
    references: result.acceptedReferences.length }));
  if (!result.ok) process.exitCode = 1;
}
NODE
```

## Files likely touched

- Production: `apps/backend/internal/agentctl/server/process/workspace_upload.go` only.
- New tests: `apps/backend/internal/agentctl/server/process/workspace_upload_permissions_test.go`,
  `workspace_upload_umask_unix_test.go` (explicit Unix build constraint), and
  `workspace_upload_permissions_windows_test.go` in that process directory.
- New route tests: `apps/backend/internal/agentctl/server/api/workspace_upload_permissions_test.go`.
- Owning requirement/design pair plus this work order and `plan.md` (exactly four DESIGN artifacts).

## Dependencies and inputs

No predecessor work order. Read the current owning pair, manifest, ROOT grant and MCP task plan;
scoped backend/agentctl/API guidance; `workspace_upload.go` and existing upload tests; read-only
resolver/registered-source/mutation-barrier controls in `workspace_files.go` and its tests; actual
route registration, bounded reader, runtime client and session-route call sites. Recheck exact
symbols/callers before implementing without broadening ownership. All source paths exist now;
new test paths above are deliberate later additions.

## Parallelism and execution ownership

`sequential`, same primary, no delegates/tasks/sessions/model switches. The MCP task plan for
`bab5404b-fcf1-448a-aca8-6e1a9349ace9` retains ROOT's complete standing gates: one global
local-heavy, original native receipts/actual joins/fresh gone proof, no retry on unknown failures,
preserved foreign resources/proofs, separate HOSTED and serial expected-head MERGE releases,
full configured exact-head semantic review, and ROOT-owned closure/archive/proof release.
Read those gates before execution/delivery; design or local success never releases the next phase.

## Risks

Mode snapshot timing, zero-value ambiguity, chmod defeating umask, unjoined test subprocesses,
external-process races and Windows attribute/rename limitations are bounded in the paired design.
Use `/mobile-parity`'s data-only exception and `/docs-maintainer`'s internal-design classification
as grounded in the manifest; reassess only on concrete scope change. No later-transport rollback.

## Results

ROOT reviewed/sealed the DESIGN package before authoring; hashes matched before legitimate
status changes. A later FULL LOCAL release authorized this same primary's minimal correction,
scoped checks and ready publication under exclusive GLOBAL LOCAL-HEAVY. No delegates were used.

Original public-writer RED native53618 (2a40fd to ddc1bb) and registered-route RED native78424
(deefa9 to d22019) compiled and actually joined exit1 before production edits, with causal
permission loss/direct script failure despite successful exact bytes/path/count. Final writer
GREEN native8542 (6e17cb to1e34bc), route GREEN native24575 (b5b20b to80f445), and scoped lint
native56169 (8028cd to675da0) actually joined exit0 after the final production edit; lint had0issues.
Use the exact commands above. The first lint's two test-only switch suggestions were corrected.

The sole production file preserves optional final-check regular destination Perm() (including0000)
through staging-descriptor Chmod/Sync/Close and rooted rename, retains umask defaults and reads
outside the lock, and cleans only unpublished staging names. Causal regressions and compatible
content/current-target/umask/failure/notification/containment controls passed. Added real closed
staging descriptor chmod/sync failures retain the original or absent destination and release the
lock. No workspace_files.go/save producer/framework/API/dependency changes were made.

Catalog357decisions/1418specifications, all36spec-linter tests, all-spec lint and whitespace
passed; pinned pnpm9.15.9 completed one frozen apps install. Native receipts/logs/immutable
cutoffs/actual joins/fresh gone proofs are retained at
`/tmp/kandev-child69-upload-permissions-20261007T092136Z/` and in the task's durable MCP plan.
Normal hooks/ready publication and separate ROOT hosted review/native Windows evidence/merge
remain delivery gates. No native Windows registered-route or full browser/E2E/build claim.


ROOT authorized a bounded CI fixture dependency correction after exact hosted merge b87854d4
(at upload head9aa5bc76/base b3f207b2, attempt1) failed Playwright listing: duplicate `session`
declarations in the desktop disabled-continuation test. The upload head's fixture matched its
original baseline; the parser regression came from the base fixture. Only this existing fixture
is added to ownership. Incorporate its base-side test block and rename the second binding and
associated references, preserving both navigations, all waits/assertions/native trace semantics.
Actual hosted parser failure is RED. Validate only this file's chromium Playwright `--list` and
applicable scoped lint. No browser/backend/build/full E2E or passing Go replay. TS-only correction
does not trigger changed-backend lint. Normal hooks/new fixup commit/push and physical LOCAL
return precede a separate ROOT HOSTED release; no observer restart or merge is authorized.


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
