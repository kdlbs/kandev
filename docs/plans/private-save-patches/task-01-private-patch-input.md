---
id: "01-private-patch-input"
title: "Use private patch input for file saves"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-SAVED-FILE-CONTENT-001
acceptance_criteria:
  - AC-WORKSPACES-SAVED-FILE-CONTENT-001.1
  - AC-WORKSPACES-SAVED-FILE-CONTENT-001.2
  - AC-WORKSPACES-SAVED-FILE-CONTENT-001.3
  - AC-WORKSPACES-SAVED-FILE-CONTENT-001.4
  - AC-WORKSPACES-SAVED-FILE-CONTENT-001.5
system_design:
  - ../../specs/workspaces/system-design/saved-file-content.md
---

# Task 01: Use Private Patch Input for File Saves

## Summary

Prevent private save input entering the real Git index when Stage All executes
ahead of a pending save. Independently author current-producer RED, send the
request-owned rewritten diff over managed Git stdin, and verify actual disk,
index and registered caller outcomes. ROOT subsequently reviewed and released
implementation in this same primary; this order is now in progress.

## In scope

- Hold Git cap one, queue actual Stage All first and observe its waiter, queue
  a matching-hash eligible `ApplyFileDiff` second and observe both waiters,
  then release. Join both operations and inspect the exact index path set and
  indexed blobs with real Git. RED must fail on private content in the index
  before production edits, while both operations and saved bytes/hash succeed.
- Author independent process and registered-route regressions in the two new
  test files below, reusing existing Git fixture/admission patterns. Use fresh
  sequential controls; prove original file content was staged in deliberate
  overlap and edited content in sequential save then stage. Include genuine
  untracked patch-like dotfiles, ordinary additions and tracked deletions.
- Include a valid finite patch larger than a pipe buffer with non-ASCII bytes
  and nil desired content, and temp overrides `TMPDIR`, `TMP`, `TEMP` inside
  the checkout. No helper-name predicate or overwrite result can prove the
  core regression. Preserve named-repository save hash/path and nonselected
  same-name neighbors through the registered route.
- In `workspace_files.go`, replace the filename with explicit `-`, set
  `cmd.Stdin = strings.NewReader(unifiedDiff)` after existing header/symlink
  rewrites, and remove the sole-use `writeFileDiffPatch` and removal defer.
  Preserve flags, working directory, admission/timeouts, managed Start/Wait,
  existing fallback/cancellation/result/event logic and caller signatures.

## Out of scope

No Stage filtering, ignore changes, global writer framework, parser/header
admission redesign, frontend, browser, layout, copy, translations, builds,
dependency updates, runtime flags or unrelated fixes. Never replay, copy,
import, mutate or remove protected ROOT diagnostic artifacts. No new task,
session, delegate, model switch or worktree.

## Acceptance

1. Independently authored RED fails for the real-index leak on the actual
   current random-helper producer; GREEN excludes every private artifact while
   Stage All still stages genuine dotfiles/additions/deletions with exact
   expected indexed bytes. Do not assert that overlap stages a future edit.
2. Save results preserve requested repository bytes, independent SHA256,
   applied/overwritten/error semantics, ACK path and neighboring bytes. Existing
   distinct-file/fallback/cancellation/symlink controls pass; cancellation never
   writes desired fallback and all owned requests/processes settle.
3. Production diff is confined to the causal producer and its unused helper.
   Registered real-Git operation evidence and native hosted Windows execution
   prove supported behavior, with any skips or unexecuted evidence explicit.

## Verification

Run only after later explicit ROOT interrupt implementation release and ONE
GLOBAL LOCAL-HEAVY grant. Mark this order `in_progress` at release. Re-read the
source-qualified producer and paired design; do not rebase moving main or
replay accepted archives. If dependencies or boundaries changed materially,
record a ROOT checkpoint before executing.

ROOT's platform plan owns retained native handles/chunks, immutable deadlines,
UTC/argv/cwd/log/PID/PGID, joins and proof of owned group/wrapper teardown.
Use only `/tmp/kandev-child67-private-save-patches/` for owned delivery logs;
verify ownership/existence before creating it. Never use default
`scripts/run-quiet` cleanup, sweep `/tmp` or alter shared caches. The following
are exact command payloads for that retained runner, from repo root. A timeout,
resource error, transport loss or unknown result is a ROOT checkpoint, not a
retry authorization.

Use `/bin/bash` with `login:false` and explicitly activate the existing
runtime paths for every later normal-hook or verification invocation. The
default login zsh strips the supplied PATH. Do not edit runtime or harness
configuration to work around it:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH"
```

First run independently authored RED, before touching production:

```bash
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 5m go test -trimpath -tags fts5 -race -p 1 -parallel 1 ./internal/agentctl/server/process ./internal/agentctl/server/api -run '^Test(ApplyFileDiff|HandleFileUpdate)_StageAll(Overlap|SequentialControl)$' -count=1 -timeout=2m -v)
```

After the minimal production correction, run GREEN and affected controls once:

```bash
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 5m go test -trimpath -tags fts5 -race -p 1 -parallel 1 ./internal/agentctl/server/process -run '^TestApplyFileDiff_(StageAllOverlap|StageAllSequentialControl|ConcurrentDistinctFiles|SequentialDistinctFiles|PatchCleanup|CancelledQueuedSave|RegularFile|Symlink|ConflictDetection|ConflictWithDesiredContent|ConflictWithoutDesiredContent|RepositoryTarget|RepositoryTargetCompatibility)$' -count=1 -timeout=2m -v)
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 5m go test -trimpath -tags fts5 -race -p 1 -parallel 1 ./internal/agentctl/server/api -run '^TestHandleFileUpdate_(StageAllOverlap|StageAllSequentialControl|ConcurrentDistinctFiles|SequentialDistinctFiles|RepositoryTarget|RepositoryTargetCompatibility)$' -count=1 -timeout=2m -v)
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m golangci-lint run ./internal/agentctl/server/process/... ./internal/agentctl/server/api/... --allow-serial-runners --concurrency=2 --new-from-rev=905fa03c5b8ae90c661dc9fec2e324d355e269aa --timeout=5m)
```

Run document gates from repo root under the same grant, without product builds:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/workspaces docs/plans/private-save-patches apps/backend/internal/agentctl/server/process apps/backend/internal/agentctl/server/api
git status --short
```

Use the exported `validateCoverage({changedFiles, fileContents})` from
`.github/scripts/pr-docs.cjs` against actual changed paths and full workspace
documents, including this manifest, order, requirement and design. Record
its `covered`/errors result and reference checks. Catalog/lint alone do not
prove work-order coverage. Do not use a no-docs override.

Before any pnpm hook in a fresh worktree, use the explicit runtime PATH above
and pinned `corepack pnpm@9.15.9`. If dependencies are absent, perform exactly
one `(cd apps && corepack pnpm@9.15.9 install --frozen-lockfile)` under the
grant. Preserve dependencies that already exist. Do not install during design.
Normal hooks stay active, with the same explicit PATH and no bypass.

An actual backend fixup requires one full CHANGED lint against the exact PR
API base. Populate `PR_API_BASE_SHA` from the retained API evidence, never a
moving local ref, then execute exactly once:

```bash
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --kill-after=10s 6m golangci-lint run ./... --allow-serial-runners --concurrency=2 --new-from-rev="$PR_API_BASE_SHA" --timeout=5m)
```

After physical local-heavy return, wait for a separate ROOT hosted release
before the ONE retained original 90m observer (GNU 91m, kill10), with no
successor/reset/retry. Require all six required checks, actual Backend,
Frontend and E2E product parents and native platform proof at the published
head, plus authenticated App347564 full substantive all-files source-covered
review. Accept sufficient automatic review first; only one necessary request
after a proved gap. Freeze the head except actual correction. Ready PR exact
association automation must be disabled and verified. Merge requires separate
ROOT SERIAL MERGE grant, expected-head normal squash/no admin, actual remote
inclusion/tree/blob and joined-owned-cleanup receipts. ROOT independently
verifies/archive/protected-proof release. Preserve managed worktree/dependencies.

## Files likely touched

- `apps/backend/internal/agentctl/server/process/workspace_files.go`
- `apps/backend/internal/agentctl/server/process/workspace_file_save_staging_test.go` (new)
- `apps/backend/internal/agentctl/server/api/workspace_file_save_staging_test.go` (new)
- Paired saved-file-content requirement/design and this delivery package for
  accurate results; existing tests only if a proved causal fixture issue requires it.

## Dependencies

No internal work-order dependency. Merged selected-repository-save
`9b4af97250f491d69d14629b549b3f33ddc66c12` is present at initial HEAD.
Later implementation, local-heavy, hosted observer and merge releases are
separate authorization gates, not implicit consequences of this plan.

## Risks

Native pipe behavior, large-input application and cancellation require actual
outcome evidence. Use portable admission helpers; no shell shims or Windows
skips for the core regression. Set test environment before Manager creation.
Register cancel/release/join cleanup before any assertion can fail; no
`t.Parallel` while mutating process-global admission. Preserve foreign files,
processes, refs and cached dependencies. Any causal scope expansion needs ROOT.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/workspaces/requirements/saved-file-content.md), `.1` through `.5`.
- [Design](../../specs/workspaces/system-design/saved-file-content.md), Request-owned patch, Compatibility, Save and staging verification.
- [Admission decision](../../decisions/2026-08-02-class-aware-git-subprocess-admission.md).
- Existing `workspace_file_save_test.go`, `workspace_file_save_target_test.go`,
  their API counterparts, `git_handlers_test.go` fixture/router helpers and
  backend TDD reference. ROOT's archived fixed-name proof is attribution-only.

## Results

### Design checkpoint (historical)

The design checkpoint's ROOT-authorized cheap preflight passed catalog
validation (357 decisions, 1416 specifications), all 36 spec-linter tests, full
spec lint, local links/whitespace and inventory. Exported coverage classified
actual fourdocs `exempt` and prospective producer `covered`, both errors `[]`.
Source audit confirms unchanged stdin through managed Git on Unix/Windows;
this is static evidence, not executed native-platform or pipe behavior.
Selectors match exactly 11 existing process and four API controls, with two new
names per package planned only and no expanded suite.

Original native `56219` / `a694bc` to `e3660e` joined exit zero. All 11
commands and their groups were joined/observed gone; wrapper PID/PGID
`1263341` was observed gone at `2026-10-07T06:56:54.665884Z`.
Exact logs and metadata are retained in
`/tmp/kandev-child67-private-save-patches/design-preflight-61x11k3s/receipt.json`
and sibling `native-terminal.json`. Zero live native handles. The earlier
node-absent invocation remains unrun; this one corrected known-runtime invocation
was explicitly released by ROOT, without install or product replay.

Only fourdocs are unstaged/uncommitted; no apps, permanent tests, production,
heavy/product checks, install, runtime/harness edits, commit, PR, hosted run or
merge. END amended DESIGN; await a later explicit ROOT implementation interrupt
and exclusive global local-heavy lease in this same primary.

### Later released implementation

ROOT's later explicit interrupt granted implementation and exclusive local-heavy
ownership after reviewing/sealing the four-document package. Independent
permanent RED exercised the actual random producer before production edits:
all four overlap cases failed on an extra private real-index entry, with
truthful successful save/stage results and correct bytes/hash. Both fresh
sequential controls passed. Original `12205` / `4b5c88` to `ee8487` joined exit
one; receipt `RED-4y21r4bt` in the owned directory below.

The smallest reviewed stdin correction then passed the exact process selector
(13 top-level tests, 60 cases, no skips), original `84121` / `6ca787` to
`fc4fd5` exit zero; the API selector (six top-level tests, 47 cases, no skips),
original `35404` / `5a4743` to `c02078` exit zero; and scoped lint, zero issues,
original `58544` / `d2c675` to `714d3a` exit zero. All three original command
groups and wrappers were subsequently observed gone.

Exact argv/cwd/PID/PGID/resource/deadline/UTC/log and native terminal receipts
are retained under `/tmp/kandev-child67-private-save-patches/`, in
`RED-4y21r4bt`, `GREEN-process-zs8kbqx9`, `GREEN-api-9iojiagt` and
`scoped-lint-l3agd4eh`. No protected ROOT archive was replayed or copied. No
production/test edit followed GREEN. Final document/reference gates passed:
catalog 357/1416, all 36 spec-linter tests, all-spec lint, actual changed-path
coverage (`covered`, errors `[]`) and whitespace/inventory. Original `2986` /
`05fba1` to `08169b` joined exit zero, with all six inner commands joined;
receipts `document-gates-7x__dwjv` and `document-gates-mjkzr77y`.

One pinned pnpm 9.15.9 frozen apps install completed because dependencies were
absent, original `15982` / `254473` to `f6c931` joined exit zero, receipt
`frozen-install-y53c11hg`. It reused 935 packages and downloaded none; normal
active hooks retain the explicit reviewed runtime PATH. Publication receipts
and physical local-heavy return belong in the platform plan. The order stays
in progress until its separately released hosted/native evidence is satisfied.
Hosted and merge authority remain absent; no broad local audit was added.
