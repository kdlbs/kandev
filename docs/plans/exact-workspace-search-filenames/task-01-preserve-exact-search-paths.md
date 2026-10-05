---
id: "01-preserve-exact-search-paths"
title: "Preserve exact search paths"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-WORKSPACE-CONTENT-SEARCH-001
acceptance_criteria:
  - AC-UI-TASK-WORKSPACE-CONTENT-SEARCH-001.6
  - AC-UI-TASK-WORKSPACE-CONTENT-SEARCH-001.7
  - AC-UI-TASK-WORKSPACE-CONTENT-SEARCH-001.8
  - AC-UI-TASK-WORKSPACE-CONTENT-SEARCH-001.9
system_design:
  - ../../specs/ui/system-design/task-workspace-content-search.md
---

# Task 01: Preserve Exact Search Paths

## Summary

Preserve native filename identity at the inventory producer and prove it reaches cached filename search, content search and registered aggregate/selected HTTP behavior. Begin only after ROOT's later same-session implementation INTERRUPT; mark this work order in progress then. All work remains strictly sequential in the existing primary session.

This work order's `done` status records completed local implementation and verification, following `/fix` Phase 4 before Phase 5 PR review. It does not complete the persistent Kandev task or delivery. The external task plan tracks the pending six required hosted gates, full semantic review, findings disposition, separately authorized actual merge and joined cleanup. These delivery barriers remain mandatory after local completion.

## In scope

- Replace newline/trim/tab-guess parsing in `getFileListClass` with the design's single tagged NUL call and source-local parsing. Preserve true mode160000 exclusion, root-only marker hiding and exact path payloads.
- New `TestWorkspaceInventoryPreservesExactFilenames`: private real Git; tracked and untracked Unicode (including non-BMP), leading/trailing whitespace, quotes, tabs and newlines supported by the native filesystem. Default/unset, true and false `core.quotePath`; compare byte-exact fixture names to production `getFileList`, call real `updateFiles` and query `SearchFiles`, then `SearchContent`. Limits must fit the fixture; exact membership and result identity are stronger than mere nonzero counts. Include empty repository and ordinary positive controls, ignored text, a tracked ignore-pattern match, nested/root ownership markers and a genuine Gitlink. Assert `RepositoryName`, `IsDir`, content preview, one-based coordinates and UTF-16 ranges with a non-BMP prefix.
- In that same test, create an actual untracked `160000 <object> 0<TAB>decoy.txt` name (and an ordinary-mode header lookalike) with selected content distinct from the real `decoy.txt`. Search for the selected marker and for the decoy separately. Assert full literal paths and matching content; no parser-generated expected paths or mocked inventory.
- New `TestWorkspaceInventoryPreservesTrackedModesAndStages`: real index fixtures for 100644/100755, supported symlink120000 and Gitlink160000 pointing at a real commit object; H ordinary/assume-unchanged, S skip-worktree and M unmerged stages 1/2/3. Keep per-stage multiplicity and existing downstream ranking. Use local index-info fixtures without shared helper rewrites or external network; stages need not be created by a merge command. A cached deleted path remains indexed; content filtering may skip absent/nonregular paths as before.
- New `TestRegisteredWorkspaceSearchPreservesExactFilenames`: real `NewServer` router, decoded JSON from aggregate GET `/api/v1/workspace/search` and `/api/v1/workspace/content-search`, followed by registered GET `/api/v1/workspace/file/content`. Cover native Unicode/ordinary controls everywhere and literal whitespace/quote/tab/newline/metadata-lookalike names where supported; URL encode query/path with `url.Values`. Check legacy files versus structured results, exact selected content and coordinates/ranges/preview.
- New `TestRegisteredWorkspaceSearchPreservesRepositoryIdentity`: independent alpha/beta repositories under a bare task holder with identical unusual repository-relative paths and distinct sentinels. Aggregate searches must preserve both repository identities without duplicate bare-root matches. For each content result request `repo=repository_name` and its relative path and assert that repository's sentinel, excluding the other's; for Files use its established prefixed path consistently. Existing initialized-root/submodule tests retain empty-root plus named-child behavior and true Gitlink exclusion.
- Snapshot Git HEAD/refs, raw index and config bytes, and relevant file bytes after fixture setup and before search; compare after production calls and selected reads. Use binary/raw subprocess output for snapshots. Register cleanup immediately and join owned managers, including on fatal paths.

## Out of scope

No safe-path redesign, file protocol/framework, cache/status/monitor/permission changes, deduplication/ranking changes, new invalid-UTF8 transport contract, second inventory subprocess, frontend changes or unrelated shared test-helper edits. Never replay/remove ROOT evidence or touch foreign resources/caches.

## Acceptance

1. AC .8/.9 are proven through real inventory, cache update and both search modes across supported tracked/untracked native names, quoting modes and metadata-lookalike/decoy fixtures; ordinary controls, ignore/marker/Gitlink boundaries and legitimate indexed modes/tags/stages remain correct.
2. AC .6/.7/.9 are proven through registered aggregate routes and selected file reads with independent same-path sentinels, unchanged response shapes, repository identity, content preview/ranges and read-only Git/index/config/filesystem assertions.
3. Exact task-defined checks pass, every handle is actually joined and results are recorded. Windows retains ordinary/Unicode/transport coverage; skip only unsupported individual name/symlink cases with a concrete reason. No whole-suite platform skip or tautological parser-helper tests.

## Verification

Load `/tdd` and its backend-tests reference on release. Accept supplied production RED as already joined; do not replay that proof. Add permanent regressions before the production edit and run the first command on unchanged production to capture their own RED, then run the same command after the minimal fix for GREEN. Controls that pass before the fix are valid compatibility evidence.

Commands run from repository root using Bash `login:false`, existing Node24 path, one heavy command at a time. ROOT's explicit current global local-heavy-command lease is required before install, Go tests/lint, Vitest/typecheck/build or heavy normal hooks; implementation release alone does not grant a later corrective lease. Actually join each tranche, record zero owned live handles and hand back the lease before hosted CI waiting. Reacquire before any later local corrective tranche. Lightweight source, raw-Git grammar, docs and catalog checks may continue without the lease. ROOT serializes merge/archive with the disjoint second child; never introduce a shared source/harness lock or touch the paused oversized child.

The observed Node path is `/home/jcfs/.nvm/versions/node/v24.18.0/bin`; preserve the current PATH behind it. Only after release and explicit current lease, if `apps/node_modules` is absent, run exactly one pinned frozen install:

```bash
(cd apps && PATH=/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH corepack pnpm@9.15.9 install --frozen-lockfile)
```

No automatic install retry. Join its handle before any heavy command. Exact targeted RED/GREEN gate:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 ./internal/agentctl/server/process ./internal/agentctl/server/api -run '^(TestWorkspaceInventoryPreservesExactFilenames|TestWorkspaceInventoryPreservesTrackedModesAndStages|TestRegisteredWorkspaceSearchPreservesExactFilenames|TestRegisteredWorkspaceSearchPreservesRepositoryIdentity|TestGetFileList_HidesOnlyRootOwnershipMarker|TestSearchFiles_HidesOnlyRootOwnershipMarker|TestWorkspaceTrackerSearchContentRanksContiguousMatchAndUsesUTF16Ranges|TestWorkspaceTrackerSearchContentUsesGitFileSetAndSkipsUnsafeOrBinaryFiles|TestWorkspaceTrackerSearchContentHonorsCancellation|TestSearchContentReportsGitAdmissionStarvation|TestSearchContentReportsGitCommandTimeout|TestManagerSearchWorkspaceFileResultsIncludesRootAndSubmodule|TestManagerSearchWorkspaceFileResultsExcludesSubmoduleGitlink|TestManagerSearchWorkspaceContentIncludesRootAndSubmodule|TestManagerSearchWorkspaceContentGroupsResultsByRepository|TestHandleFileSearchIncludesEveryTaskRepository|TestHandleWorkspaceContentSearchReturnsTypedMatches)$' -count=1 -timeout=90s -v)
```

Scoped lint against the actual starting base (replace only if ROOT explicitly releases a different baseline):

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --kill-after=10s 6m golangci-lint run ./internal/agentctl/server/process ./internal/agentctl/server/api --new-from-rev=e095ca17790d3dd0e4700b473780add77ac7a230 --concurrency=2 --allow-serial-runners --timeout=5m)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/exact-workspace-search-filenames
```

Run the repository `.github/scripts/pr-docs.cjs` exported `validateCoverage` preflight with actual changed files and all linked work-order/plan/requirement/design contents. During design it can validate the anticipated runtime paths against the working-tree documents; record that as projected coverage, not hosted exact-head success. Before PR use actual changed paths and exact committed contents.

No broad test suite, browser, build, local E2E or PG. Retain every returned session/cell handle and actually join before starting the next heavy command. Timeout/resource/transport failures are NO PASS: save a complete versioned checkpoint in this own Kandev plan and end WAITING for ROOT; no retry, cache wipe or foreign-process kill. Actual lint findings may be minimally corrected with affected checks and corrected-code validation.

## Files likely touched

- `apps/backend/internal/agentctl/server/process/workspace_files.go`
- `apps/backend/internal/agentctl/server/process/workspace_inventory_exact_paths_test.go` (new)
- `apps/backend/internal/agentctl/server/api/workspace_search_exact_paths_test.go` (new)
- `docs/specs/ui/requirements/task-workspace-content-search.md`
- `docs/specs/ui/system-design/task-workspace-content-search.md`
- `docs/plans/exact-workspace-search-filenames/plan.md`
- `docs/plans/exact-workspace-search-filenames/task-01-preserve-exact-search-paths.md`

## Dependencies

None beyond later explicit implementation release. Existing completed search/reveal packages remain historical; do not reopen them.

## Risks

Header detection without a tag can mistake a literal untracked path for a Gitlink. H-only handling can discard S/M entries. Native Windows cannot create some names; keep per-case platform constraints narrow. Manager startup/rescan is asynchronous, so API fixtures need an observed complete scan and cleanup, never invented seeded cache state.

## Parallelism

`sequential`. No agents, delegates, persistent tasks, sessions, tabs or model changes.

## Inputs

- [Owning requirement](../../specs/ui/requirements/task-workspace-content-search.md), AC .6/.7/.8/.9.
- [Owning design](../../specs/ui/system-design/task-workspace-content-search.md), exact inventory and scope/result sections.
- Source-local production flow in `workspace_files.go`, `workspace_content_search.go`, `workspace_tracker.go`, `api/workspace.go`, `api/server.go` and `Manager.JoinRepoPath`.
- Existing `workspace_files_test.go`, `workspace_content_search_test.go`, `workspace_search_submodule_test.go`, `api/workspace_file_search_test.go`, `api/workspace_content_search_test.go`, `api/workspace_rescan_test.go`, process TestMain/goleak and backend-tests reference. Reuse helpers as-is only when they preserve raw outputs and isolate Git environment; otherwise add private local helpers in the owned new files.
- Immutable ROOT production RED and separate raw-probe receipts recorded in the manifest/own task plan.

## Delivery after release

Normal active hooks, commit, push, ready PR, CI/review/fixup and normal merge are standing authorized only after ROOT releases implementation. Load delivery skills then. Freeze published SHA except a real correction; never rebase for main drift or run synthetic merged tests. A PR fixup that touches backend code requires one full changed lint, using exact PR base:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --kill-after=10s 6m golangci-lint run ./... --new-from-rev="$KANDEV_EXACT_PR_BASE_SHA" --concurrency=2 --allow-serial-runners --timeout=5m)
```

Resolve and record that base before this command. A docs-only correction with unchanged backend bytes needs scoped artifact checks and normal applicable hooks; it does not require replaying Go tests or full Go lint. One owned `scripts/pr-await` all-terminal monitor at a time; actually join before replacement, no manual `pr-state` timer polling. Require six configured required hosted gates terminal success and authenticated configured CodeRabbit App347564 substantive FULL exact-current-head/all-file coverage. Inspect completed automatic coverage before at most one necessary full request for a corrected head; ACK/skips/progress are insufficient. Disposition every finding; defer optional polish with grounded reasons. No blind/second hosted retry: save unrelated failure evidence for ROOT's bounded decision.

Merge only after ROOT grants the separate merge lease, by normal expected-head squash, without admin/bypass; independently verify actual merge/tree/all owned blobs/remote and join owned cleanup. The persistent Kandev task remains incomplete until actual verified merge and joined cleanup; ROOT owns independent verification/archive and parent proof release. Incoming parent callback queue is full: never retry notifications/questions or change harness/routing. Preserve task `3aeeb034-c702-41a8-a96f-0ca91275560f`, primary session `af1fe33c-1660-4f7d-9a77-59f11c1ee4fb`, title, marker, user edits and question/final-action barriers in own-plan checkpoints. End WAITING at design, resource failure, blocker or actual completion for ROOT polling.

## Results

Implemented after ROOT's explicit same-session release on 2026-10-05. Production changes only `workspace_files.go`: one managed `ls-files` adds `-z -t`, and its source-local H/S/M/? extraction preserves literal path payloads and existing Gitlink/marker exclusions. Cache, safe-path, ranking, status/monitor, API shapes and frontend code remain unchanged. Two owned new regression files cover all four planned test families; shared fixture helpers are untouched.

- Faithful permanent RED: exact anchored verification command, handle 51865 actually joined, exit 1. Three new families failed for missing/altered native inventory and registered search paths; 14 top-level controls passed. Log `/tmp/kandev-exact-inventory-permanent-corrected-red.log`. The preceding attempt exposed a missing nested fixture directory; that setup-only process failure is not counted as production RED. ROOT's original proof was never replayed or removed.
- GREEN: exact same task gate with corrected fixture and content-stage multiplicity assertion, handle 6250 actually joined, exit 0. All 17 top-level tests passed (13 process, 4 API); package times 2.694s and 1.991s. Log `/tmp/kandev-exact-inventory-permanent-final-green.log`; receipt `/tmp/kandev-exact-inventory-red-green-receipt.json` lists actual tests. This Linux run does not claim a native Windows run; new suites retain portable Unicode/ordinary/transport coverage with narrow unsupported-name/symlink cases.
- Scoped lint: documented two-package command against `e095ca17790d3dd0e4700b473780add77ac7a230`, handle 21986 actually joined, exit 0, `0 issues`. Log `/tmp/kandev-exact-inventory-scoped-lint.log`. No broad suites/browser/build/E2E/PG were run.
- Documentation and engineering-guide assessments remain valid for the final bounded diff: existing public search wording is restored; no guide convention/package/export change requires correction. The paired design is current and the existing requirement remains active.

Initial normal hooks, commit, push and ready PR publication completed; the initial local-heavy lease was returned with zero owned live local handles. Hosted gates, full review and findings disposition, actual merge and joined cleanup remain delivery work tracked in the external Kandev task plan. Local work-order completion does not satisfy those gates. Later publication hooks/commit/push require a new local-heavy lease; actual merge requires ROOT's separate merge lease.
