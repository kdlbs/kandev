---
id: "01-publish-attachment-candidates"
title: "Publish complete attachment candidates safely"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-DOCUMENTS-002
acceptance_criteria:
  - AC-TASKS-DOCUMENTS-002.1
  - AC-TASKS-DOCUMENTS-002.2
  - AC-TASKS-DOCUMENTS-002.3
  - AC-TASKS-DOCUMENTS-002.4
  - AC-TASKS-DOCUMENTS-002.5
system_design:
  - ../../specs/tasks/system-design/plan-write-lifecycle.md
---

# Task 01: Publish complete attachment candidates safely

## Summary

Implement the attachment publication and owned-cleanup boundary in the existing
document service. Prove rejected replacement preserves persisted metadata and
real download bytes, with registered HTTP evidence and executed Windows cases.

## In scope

- TDD service/SQLite/filesystem regressions in the plan's exact test matrix.
- Unique private complete files, metadata publication, owned prepublication
  cleanup after handle close, unconditional candidate retention on every metadata
  invocation error, and unchanged superseded-file policy. No cleanup re-read.
- Existing path assertion update, explicit legacy canonical read/delete fixture,
  registered multipart upload/download rejection and success tests, and narrow
  Windows native-lane execution.
- Keep plan/work-order results accurate and recheck the owning pair after success.

## Out of scope

Everything excluded by the plan: SQL/schema/API/auth/UI changes, generic
transactions, history, concurrency locks, ACL policy, reclamation, PG fixtures,
broad checks/build/browser/E2E, proof replay, or additional agents/sessions/tabs.

## Acceptance

1. The production upload path satisfies `.1` through `.4` with full real-row and
   production-download byte assertions across the named matrix, including
   partial-write/close failure and cleanup isolation before metadata invocation.
   Every metadata error retains its candidate; real commit then error followed
   by independent overwrite/delete must preserve previously resolved download
   bytes, current/prior files and the independent operation's authoritative row.
2. Registered HTTP transport satisfies `.5`: expected upload status, preserved or
   replaced bytes, matching filename/MIME headers and unchanged DTO shape.
3. The exact targeted checks pass after authorized execution, and the hosted
   Windows native lane actually runs the new portable service cases. Record
   commands, test/subtest counts, exits and receipts; do not infer execution from
   a successful compile or a skipped lane.

## Verification

Do not run Go, lint, install or product checks during design. After ROOT's later
explicit interrupt and separate heavy grant, execute sequentially from the repo
root with bash `login:false` and explicit tool PATH. Each command is independent;
retain any returned session handle and join before the next heavy command.

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p 1 ./internal/task/service -run '^(TestDocumentAttachment.*|TestDocumentService.*)$' -count=1 -timeout=4m -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p 1 ./internal/office/dashboard -run '^TestDocumentHandler_' -count=1 -timeout=4m -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m golangci-lint run ./internal/task/service ./internal/office/dashboard --new-from-rev=55a231c2c8a87f13508f345ab434bc571e888c36 --concurrency=2 --allow-serial-runners --timeout=5m)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/preserve-failed-attachment-replacement
```

For RED, first run only
`^TestDocumentAttachmentFailedReplacementPreservesPublished$` using the same Go
flags/bounds. The qualified ROOT proof is already accepted and must not be
replayed/imported. Author permanent regressions from the owned matrix and prove
their own meaningful RED before implementation. GREEN is the full targeted
commands above; no duplicate passing replay. If HEAD/source has changed on later
release, inspect the relevant diff rather than blindly replacing the design base.
PR-finding remediation resolves the exact live PR base via API and uses the full
changed `./...` lint gate and 1GiB limit described in the plan, once under bounds.

Add this equivalent bounded test command to the existing Windows native lane
(PowerShell step timeout6m, Go timeout4m), with the actual resulting names visible:

```powershell
$env:GOMAXPROCS = '2'
$env:GOMEMLIMIT = '512MiB'
go test -trimpath -tags fts5 -race -p 1 ./internal/task/service -run '^TestDocumentAttachment.*$' -count=1 -timeout=4m -v
```

The step must fail on nonzero Go exit and run when backend changes select the
native suite. Inspect actual hosted output for these cases before delivery; no
local Windows compile or unrelated test run substitutes for execution.

Use `.github/scripts/pr-docs.cjs`'s exported `validateCoverage` against the
uncommitted plan/work order and referenced pair for local traceability preflight;
its CLI is GitHub-event based. Do not fabricate a live PR result. Documentation
checks and preflight are cheap and permitted during design. No public-doc
validator/install is required for this internal design package. Public docs
impact must be reassessed at implementation; this correction adds no route,
command, visible copy or public storage-path contract.

Exact local reference preflight, from repo root (prospective service path is
included solely to exercise coverage validation):

```bash
PATH=/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH node <<'NODE'
const fs = require('node:fs');
const validator = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/tasks/requirements/documents.md',
  'docs/specs/tasks/system-design/plan-write-lifecycle.md',
  'docs/plans/preserve-failed-attachment-replacement/plan.md',
  'docs/plans/preserve-failed-attachment-replacement/task-01-publish-attachment-candidates.md',
];
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = paths.map(filename => ({filename, status: 'modified'}));
changedFiles.push({filename: 'apps/backend/internal/task/service/document_service.go', status: 'modified'});
const result = validator.validateCoverage({changedFiles, fileContents});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

## Files likely touched

- `apps/backend/internal/task/service/document_service.go`
- `apps/backend/internal/task/service/document_attachment.go` (private candidate writer)
- `apps/backend/internal/task/service/document_service_test.go` (canonical path assertion)
- `apps/backend/internal/task/service/document_attachment_publication_test.go` (new)
- `apps/backend/internal/office/dashboard/document_attachment_publication_test.go` (new)
- `apps/backend/internal/office/dashboard/document_handlers.go` (storage comment only if needed)
- `.github/workflows/backend-tests.yml` (one narrow Windows step)
- The owning requirement/design pair and this plan/work order (status/results only).

Existing repository implementation/model, prompt attachment cleanup, frontend
and other workflow steps are not owned by this work order. A necessary narrow
file writer helper may live beside `document_service.go`; preserve one service
boundary and avoid a generic filesystem interface.

## Dependencies

None between work orders. Execution requires ROOT's later explicit interrupt to
primary session `0c648092-1d4e-45c8-85e4-4efada9cb6b7` and separate resource grant.
Merge requires a separate ROOT serial grant. All identity, evidence, observation,
delivery and cleanup barriers in [plan.md](plan.md) remain binding.

## Risks

Retained metadata-error/superseded files, cleanup diagnostics, legacy internal path
assumptions and actual Windows execution are the bounded risks in the design.
Use temporary native paths and immediate cleanup registration. Close all handles;
use synchronization barriers for the one independent-success interleaving, not
sleeps. No broad concurrency or cross-resource rollback claim follows.

## Parallelism

`sequential`

## Inputs

- [Owning requirement](../../specs/tasks/requirements/documents.md), `REQ-TASKS-DOCUMENTS-002`.
- [Owning design](../../specs/tasks/system-design/plan-write-lifecycle.md), attachment sections.
- Backend and Office scoped guides; `/tdd` and its backend test reference.
- Existing `newDocumentTestService`, `stubDocRepo`, `newDocumentTestDeps`,
  `RegisterDocumentRoutes`, and `AttachmentService.writeAttachmentBytes` as
  nearby patterns; do not copy prompt-attachment lifecycle policy.
- Accepted ROOT receipts and authority/resource record in [plan.md](plan.md).

## Results

In progress after ROOT's later explicit implementation release. Permanent RED
joined exit1 on actual same-extension lookup/update corruption; service GREEN
joined exit0 (24 top-level/27 subcase passes), registered HTTP GREEN joined exit0
(13 top-level/6 subcases), and the changed exact canonical legacy fixture passed
its targeted check. All original groups are gone. Receipt details are in
[plan.md](plan.md), with raw logs/JSON under
`/tmp/kandev-child71-attachment-publication-20261007/`. Scoped lint passed with zero issues after its one equivalent test-style
correction; that changed test passed alone. Normal hooks, ready PR and hosted
Windows execution are pending. The repository status records
the prepublication checkpoint; final hosted acceptance is recorded in the live
task plan without an optional status-only change to the frozen published SHA. Merge authority remains NONE.
