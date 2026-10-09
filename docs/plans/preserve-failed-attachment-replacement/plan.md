---
created: 2026-10-07
status: draft
requirements:
  - REQ-TASKS-DOCUMENTS-002
system_design:
  - ../../specs/tasks/system-design/plan-write-lifecycle.md
legacy_specs: []
---

# Implementation Plan: Preserve attachments when replacement fails

## Overview

Publish attachment metadata only after a unique private file contains all of the
new bytes. One sequential work order covers service behavior, causal storage
regressions, registered HTTP transport, and narrow Windows execution. The tasks
system owns this package because it owns durable task documents and attachments;
Office is a transport consumer.

The owning requirement is [documents.md](../../specs/tasks/requirements/documents.md)
(`REQ-TASKS-DOCUMENTS-002`, criteria `.1` through `.5`). Extend the existing
catalog-owned [persistence design](../../specs/tasks/system-design/plan-write-lifecycle.md)
and preserve its separate `REQ-TASKS-DOCUMENTS-001` plan-write boundary. No other
companion delivery package is linked from those two source documents at this base.

## Implementation admission and local results

ROOT issued the later explicit implementation release after the completed design
checkpoint and acceptance `/tmp/kandev-root-child71-design-acceptance-20261007.json`.
The same primary session owns execution. Historical design-only statements below
record the prior phase. Child71 now owns the exclusive serial local-heavy grant
through local checks, normal hooks and ready-PR delivery; merge authority is NONE.
No delegate, model session or additional tab was created.

Permanent causal RED was authored independently, without importing/replaying ROOT
proof. Session36706 joined `2efa9e`, exit1, group3254121 gone. Same-extension lookup
and update failures changed production download bytes while full persisted rows
remained equal; different-extension controls passed. Package0.359s.

Service GREEN session47066 joined `cb1f48`, exit0, group3264080 gone: 24 top-level
and 27 subcase passes, including 7 new attachment tests/18 subcases. Package1.761s.
Registered HTTP GREEN session88548 joined `7b72fc`, exit0, group3269443 gone:
13 top-level/6 subcase passes, package3.087s. The legacy fixture was then made
explicitly `<base>/attachments/task-doc/legacy.bin`; its only changed test passed
in session67741 joined `8c072b`, exit0, group3272225 gone, package1.332s. No broad
passing suite was replayed. Logs/terminal receipts are under
`/tmp/kandev-child71-attachment-publication-20261007/` with labels `red`,
`green-service`, `green-http`, and `green-legacy-layout`.

Production helper `document_attachment.go` stays within the service boundary.
The private per-service file factory drives real partial-write/close failures;
no generic filesystem/database engine was added. The Windows native workflow
step follows the existing service prewarm and requires actual executed new
cases; hosted evidence remains pending. Initial scoped lint session20550 joined `b455ef` exit1, group3277485 gone,
with one gocritic ifElseChain in the new metadata test. Its equivalent switch
correction passed that changed test only: session56146 joined `83ff5a`, exit0,
group3288628 gone; 1 top-level/3 subcases, package1.354s. Corrected scoped lint session13669 joined `113ab6`, exit0, group3293791 gone,
zero issues, with concurrency2/allowserial and the exact original base/caps/bounds.
Catalog/spec gates `061758` and local coverage `07a7fb` passed for all 11 actual
files. Protected proof check `29a0a1` preserved0400 and the qualified SHA.
The single conditional frozen `corepack pnpm@9.15.9 install --frozen-lockfile`
from `apps/` completed exit0 in2s: session90389 joined `b1ee80`, group3299584 gone.
All935 packages reused; lockfile unchanged. Hooks were already ACTIVE.
Normal hook receipts, published SHA, hosted acceptance and final work-order
completion are recorded in the live task plan; repository status is the
prepublication checkpoint to preserve the frozen published SHA.

Internal docs updated. No public-doc change is needed: route, DTO, command,
visible copy and public storage contracts are unchanged; this is correction of
failed replacement behavior. No rendered UI or browser capture is required.

## Scope

### In scope

- Unique complete candidate files, metadata publication, and owned failure cleanup.
- First uploads, same/different extension replacement, lookup/preparation/create/
  update failures, error-after-commit uncertainty, success, and legacy downloads.
- Persisted-row and production-download byte assertions with real SQLite/files.
- Existing registered multipart upload/download routes and narrow native Windows
  execution of the portable service cases.

### Out of scope

- Generic filesystem/database transactions, attachment history, schema/SQL/API
  changes, authorization, UI, metadata/text-document rewrites, and ACL preservation.
- Broad concurrency serialization, immediate old-file reclamation, crash garbage
  collection, PostgreSQL fixtures, feature toggles, and baseline policy expansion.
- Broad local suites, builds, browser/E2E runs, synthetic merges, proof replay,
  optional polish, or implementation during this design turn.

## Technical approach

`DocumentService.UploadAttachment` in
`apps/backend/internal/task/service/document_service.go` resolves metadata before
allocation, prepares an exclusive `os.CreateTemp` file with a fixed safe prefix,
checks complete write and close, and writes its path with existing
`CreateDocument`/`UpdateDocument`. The opaque path is internal. Replace the
canonical-path assertion in `document_service_test.go` with containment,
distinct-path, row-identity, and production-download assertions. Keep a separately
seeded legacy canonical fixture to prove backward compatibility.

Cleanup removes only an operation-owned definitely unpublished candidate after
preparation failure and handle close, before metadata invocation. Once
`CreateDocument`/`UpdateDocument` invocation begins, retain the complete candidate
on every returned error and log one bounded diagnostic with the wrapped primary
error. Remove the proposed verification re-read entirely: a later pointer cannot
prove this candidate was never published. Successful uploads retain superseded
paths under the existing reclamation policy. No stale metadata rollback or
old/canonical-path unlink occurs.

Add regressions in new focused test files instead of growing the existing large
service test file. HTTP tests register `RegisterDocumentRoutes` and use real
storage plus repository wrappers for rejection. Handler production logic should
remain sufficient; correct its inaccurate `data/attachments/key.ext` comment if
touched. Add only a scoped service-test step to the existing Windows native lane
in `.github/workflows/backend-tests.yml`.

| Storage/transport | Identity and publication | Evidence | Uncertain/unsupported behavior |
| --- | --- | --- | --- |
| SQLite + local filesystem | Task/key row selects unique complete file | Real service/row/download matrix | Every metadata invocation error retains candidate |
| Legacy canonical file | Existing persisted `DiskPath` remains authoritative | Seeded canonical download/delete regression | No path migration or reclamation |
| Registered Office HTTP | Existing multipart routes and public DTO | Router upload rejection then download; success control | Existing HTTP error mapping |
| Windows local filesystem | Native task directory, closed unique files | Executed hosted portable service cases | No ACL/delete-while-open promise |
| PostgreSQL | Existing unchanged row methods | No new SQL/dialect claim | Re-plan only if repository contract changes |

## Tests

Planned tests in `apps/backend/internal/task/service/document_attachment_publication_test.go`:

| Criterion | Named test and causal assertion |
| --- | --- |
| `.1` | `TestDocumentAttachmentFailedReplacementPreservesPublished`: same/different extension lookup/update failures, full row equality and original production-download bytes |
| `.1`, `.2` | `TestDocumentAttachmentPreparationFailures`: deterministic directory/candidate failure plus narrow production partial-write/close failure; no metadata publication and owned cleanup |
| `.2` | `TestDocumentAttachmentFirstPublication`: success and rejected create, real FK rejection, no exposed candidate or row on rejection |
| `.3` | `TestDocumentAttachmentSuccessfulReplacement`: one row, same ID/creation time, new metadata/complete download bytes, no revisions |
| `.3` | `TestDocumentAttachmentLegacyDownloadAndDelete`: seeded canonical path downloads; successful delete removes row and download becomes not found; existing file-retention policy persists |
| `.4` | `TestDocumentAttachmentCleanupIsolation`: prepublication failure removes only its own candidate after close; current/prior files and another key's published sentinel survive; metadata rejections retain candidates |
| `.4` | `TestDocumentAttachmentMetadataOutcomeUncertain`: wrapper performs real commit then returns error, followed by independent successful overwrite or deletion; previously resolved download path remains readable, current/previous bytes survive and no stale row rollback occurs |

The error wrapper must fail only its targeted call. Assertions reload through the
real repository/normal service to avoid testing injected read errors instead of
the published state. Write/close fault coverage enters production upload and
checks row/download outcomes, rather than testing a tautological helper.

## End-to-end transport evidence

Planned `apps/backend/internal/office/dashboard/document_attachment_publication_test.go`
uses `TestDocumentHandler_AttachmentFailedReplacement` and
`TestDocumentHandler_AttachmentSuccessfulReplacement`. Registered routes receive
real multipart requests and subsequent GET downloads. For `.5`, lookup/update
rejection produces 500 and original bytes/headers; first upload and same/different
extension success produce 200 with matching DTO, headers and new bytes. A failed
first upload leaves the GET unavailable. No rendered interface changes or
Playwright/browser run is needed for this storage publication boundary.

## Work orders

- [ ] [Task 01: Publish complete attachment candidates safely](task-01-publish-attachment-candidates.md)

Only one sequential work order. No delegates, new sessions, model sessions, tabs,
or implementation authority follow from this manifest.

## Evidence and authority record

`<kandev-system>` Child71 task `e0c14afe-7a32-4885-a925-59fee23fd3ae`; primary
session `0c648092-1d4e-45c8-85e4-4efada9cb6b7`; ROOT
`14825981-b175-411d-999a-31ddc2aa5fc3`. Title verified as **Preserve attachments
when replacement fails**. Design HEAD is
`55a231c2c8a87f13508f345ab434bc571e888c36`, matching ROOT proof base.
Starting worktree was clean. Source blobs compared once and matched:
service `bb4d8382e41946cba285b61fedf8478ab7efdb4c`, existing test
`c1d2656262e0a77d9505fc119bc599088f3027fd`.

Accept ROOT's qualified real causal proof without replay, import, or mutation:
readonly0400 `/tmp/kandev-root-document-attachment-replacement-candidate_test.go`,
SHA256 `b8ed997faaef9ad377a82b8522a7b369ac986e8f9c24d4bacd3c3cfdd1c43d70`.
Receipts in `/tmp/kandev-root-document-attachment-discovery-20261007/`:
`qualification.json`, `proof-receipt.json`, `native.json`, `proof.log`.
Actual joined exit1, native99569/f11d20 to550d6d, Go package0.338s. Real service,
SQLite and filesystem; only repository error injection. Same-extension lookup
and update failures preserved the row but corrupted production download bytes.
Success control passed ID/filename/size/download. ROOT clean and owned group3194111
gone verified47138c. These are accepted prior receipts, not new child test results.

DESIGN ONLY ends with the four uncommitted artifact files and zero owned running
resources. Wait for a LATER explicit ROOT `delivery_mode=interrupt` to THIS primary
session before implementation. GLOBAL LOCAL-HEAVY NONE until a separate ROOT
grant; Child70 is the other hosted-only slot. The live task plan preserves the
system marker, identity, user edits, phase state, question barrier and receipts.
Autopilot resolves routine reversible details; a critical unsafe/uninferable
choice uses the parent-question tool and immediately ends that turn. No operator
question, inferred implementation, or child-to-ROOT interrupt. Optional callbacks
never gate progress; ROOT may read this primary session and plan directly.

## Later execution and delivery gates

- Retain every command/observer handle and join to a known exit. Work only in
  owned worktree/temp resources; preserve ROOT proof, shared caches and unproved
  database volumes. No automatic retry, cache wipe, foreign kill, or duplicate
  passing replay after a timeout/unknown transport result; checkpoint ROOT.
- After explicit release, use the existing Node runtime with explicit PATH and
  bash `login:false`. If dependencies are absent, one frozen `apps/` install with
  pinned pnpm9.15.9 is allowed only after release. This backend-only package does
  not independently require a frontend install; normal hooks may.
- Future local Go uses `-trimpath -tags fts5 -race -p 1`, GOMAXPROCS2,
  GOMEMLIMIT512MiB; one global heavy operation after grant. Scoped lint uses
  concurrency2/allowserial, CLI5m, GNU6m/kill10. A real later backend PR finding
  requires one full changed `./...` lint against the exact live PR API base with
  GOMAX2/GOMEM1GiB and the same bounds before push.
- Normal active hooks, no bypass/amend; ready PR then immutable published SHA
  except a real finding. No main-drift rebase. Read canonical current internal
  PR info, preserve live bot/checklist body, keep all five operational flags
  false. Require all six required checks plus actual Backend/Frontend/E2E parent
  SUCCESS, including actually executed new Windows cases.
- Accept authenticated configured CodeRabbit App347564 substantive FULL coverage
  of all actual files with sourceCommit=coveredCommit=head and kind=reviewed
  first. One necessary full request only after a proved exact-head gap with no
  running review; no optional Claude wait or ACK approval. Disposition every
  actual finding and defer optional polish.
- One retained90m all-terminal observer, GNU91m/kill10/interval60, fixed identity
  and cutoffs. Join and freshly prove owned observer gone before a bounded
  ROOT-authorized replacement. No timer GitHub polls, duplicate observer, or
  unknown retry.
- Merge authority NONE until separate ROOT serial grant. Then normal
  expected-head squash without admin/bypass, followed by independent actual
  merged SHA/tree/all owned blobs/remote-main verification and all handles joined
  with proven owned cleanup. ROOT archives only after independent acceptance
  and releases checksum proof.

## Design review correction

ROOT's design review identified that a negative current-pointer check cannot
prove historical nonpublication: real commit then error, followed by independent
overwrite/delete, may leave a download already using the older candidate. The
four-file package now removes the verification read and retains every complete
candidate once metadata invocation begins. Prepublication preparation cleanup
remains restricted to the owned candidate after handle close. No additional
query, transaction, history, commit marker or download-lifetime scheme is needed.
The test matrix now covers this precise interleaving and retention boundary.

This is DESIGN review only. Implementation interruption remains later;
GLOBAL LOCAL-HEAVY NONE and MERGE NONE for Child71. Child70 owns serial merge.
All original cheap commands completed in their initial response; none was
aborted or left running by the turn interruption. ROOT proof remains untouched.

## Verification results

Changed-document validation after ROOT's cleanup correction completed with
exit0: catalog `1f891c` (360 decisions/1427 specifications), all-spec lint
`329961`, local reference coverage `4573c0` (`covered`, no errors), and
whitespace/exact-four-file/one-work-order/staged-empty checks `6737c8`.
The original passing spec-linter tests were not replayed. Every correction
command completed synchronously; no yielded handle or owned resource remains.

Design checks on 2026-10-07 all completed synchronously with exit0:

- `python3 scripts/list-docs.py validate`: 360 decisions/1427 specifications;
  receipt `137cbc`.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed; receipt `35c928`.
- `python3 scripts/lint-spec-files.py --all`: all specs passed; receipt `bd6240`.
- `git diff --check`, status and size inspection: clean whitespace, only the
  owning pair and new bounded plan directory changed; receipt `20b405`.
- Explicit Node24.21.0 local `validateCoverage` with the four on-disk artifacts
  and prospective service path: `covered`, no errors, exact requirement/design
  reference accepted; receipt `2e03c1`. This is local reference validation, not
  a live PR or a production-test result.

Discovery-only path misses were resolved: absent `system-design/documents.md`
(exit1) by the catalog's existing `plan-write-lifecycle.md`; absent `ci.yml`
(exit2) by `backend-tests.yml`; an Office-storage-pattern no-match (exit1) by
the actual `office/routes.go` construction. No command yielded a running
session handle; all actual commands terminated in their initial response.
No owned process, observer, temporary resource or background wait remains.

Implementation, local Go/lint, Windows execution, PR checks, review and merge are
pending separate authorization. No heavy command, dependency install, product
run, or permanent test change is part of this design turn.

## Risks

- Every metadata invocation error retains complete candidate bytes, potentially
  orphaned. Only definitely prepublication preparation failures permit cleanup;
  a later row pointer is not historical nonpublication evidence.
- Successful replacements retain superseded files under the existing policy;
  this package supplies no reclamation or crash recovery.
- Internal canonical-path assumptions must become opaque-path assertions while
  explicit legacy read/delete fixtures remain. Windows execution must be visible
  in hosted test output; compilation is insufficient.

## Next action

Hand off the uncommitted package, then wait for ROOT's later explicit interrupt
to this primary session. Do not implement or start a wait observer in design.
