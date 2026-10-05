---
created: 2026-10-05
status: implemented
requirements:
  - REQ-UI-TASK-WORKSPACE-CONTENT-SEARCH-001
system_design:
  - ../../specs/ui/system-design/task-workspace-content-search.md
legacy_specs: []
---

# Implementation Plan: Preserve Exact Workspace Search Filenames

## Overview

Correct Git-backed inventory once, so cached Files search and fresh Contents search retain native filename identity through their registered API results. One sequential work order owns the parser, real-Git regressions and registered aggregate/selected API proof. ROOT released this reviewed package for implementation in the existing session on 2026-10-05. Delivery requires the current local-heavy lease and a separate later merge lease.

Actual clean starting base: `e095ca17790d3dd0e4700b473780add77ac7a230`. Both suspect source blobs match proofbase `91da5a242f145b0bb7d0303b4969fdc1552811e5`: `workspace_files.go` = `0b15f380fed26113355dc8eab0437b1a78cfa81a`; `workspace_content_search.go` = `b34348de1503a81826c742060a43079b6de66d0f`.

ROOT may overlap this child's design/read-only work and hosted CI with a disjoint persistent child. One global local-heavy-command lease spans ROOT and both children. This child needs ROOT's explicit current lease before install, Go tests/lint, Vitest/typecheck/build or heavy normal hooks, even after implementation release. Actually join the local tranche, record zero owned live handles and return the lease before hosted waiting; reacquire for later local corrections. ROOT serializes merge and independent archive. No shared locking/harness mechanism or delegates are introduced; the paused oversized task remains untouched.

## Evidence and root cause

The accepted ROOT `TestRootNextExactInventoryDiagnostic` used real production `getFileList`, `updateFiles`/`SearchFiles` and `SearchContent` with private real Git repositories. Inner Go exit was 1; the wrapper exit 0 honestly records that failure. Handle 95599 was joined; elapsed 8.085 seconds, package 0.113 seconds. The ordinary `plaininventory.txt` control passed while five supported Unicode/leading-space/tab/newline cases failed exact inventory, filename search and content search. Retain the immutable `/tmp/kandev-exact-inventory-repro_test.go` (mode 0444, SHA256 `821484a37235ee89517be408c5a3949e694a38b8f78b800c1ce6b68ccaa0952c`), receipt and log; do not replay or remove them.

At the starting base, `getFileListClass` splits newline output, trims each record/path and assumes any tab separates tracked metadata. Default Git quoting and escaped output lose Unicode/control-character identity; trimming loses spaces. Untracked filenames can resemble the tracked header. Content search then silently skips nonexistent altered paths. `resolveSafePath` preserves supported native names and needs no redesign.

ROOT's `/tmp/kandev-root-next-file-list-raw-probe.json` is only raw Git/source-parser evidence, including an untracked metadata-lookalike and a genuine Gitlink. The independent design grammar receipt `/tmp/kandev-exact-inventory-grammar-design-receipt.json` is also raw Git only: Git 2.43 confirms H/S/M/? tags, modes 100644/100755/120000/160000, stages 0..3 and exact untracked remainder. Its SHA256 is `1b42f532d7982f0b7f1258ad1963743d915cba1544d5610565776fece8e59058`; its private repository was removed. Neither raw probe establishes production GREEN.

## Scope

### In scope

- Reuse active task-search AC .6/.7/.8 and add only missing filename identity AC .9 in the existing owner.
- Source-local tagged NUL inventory from one `ls-files` call, including legitimate indexed tags/stages, true Gitlink and root-only marker handling.
- Real production inventory/cache/search and registered aggregate search plus selected file-read transport proof.

### Out of scope

- File protocol redesign, generic parser framework, broad permission/path audit, cache ownership migration, status/monitor changes, ranking/deduplication changes and invalid-UTF8 filename JSON support.
- Frontend interaction changes, browser/build/local E2E, broad backend suites, PostgreSQL and new flags or dependencies.
- Proof replay, foreign resources/shared cache changes, native agents, delegates, new tasks/sessions/tabs or model changes.

## Technical approach

Implement the [exact inventory design](../../specs/ui/system-design/task-workspace-content-search.md#exact-git-inventory-and-search-consumption) in `apps/backend/internal/agentctl/server/process/workspace_files.go:getFileListClass`. Preserve the current admission/deadline/environment/error plumbing. Extract a small source-local helper only if needed for readability/lint; do not introduce a shared framework or test-only production seam. `SearchContent` and safe-path production code should require no edits.

| Boundary | Identity contract | Evidence and limits |
| --- | --- | --- |
| Git output | Tagged NUL records; H/S/M header versus ? literal payload | Real Git fixtures; no shell output display helper as parser input |
| Inventory/cache | Exact slash-separated repository-relative path; fresh returned slice | Production enumeration and `updateFiles`, no seeded `currentFiles` substitute |
| Filename API | Legacy files plus repository-qualified structured results | Registered GET search and JSON decoding |
| Content API | Repository-relative path plus raw repository identity/ranges/preview | Registered GET content-search and JSON decoding |
| Selected read | URL-encoded repo/path reaches correct same-path sentinel | Registered GET file/content; aggregate routes remain aggregate |
| Native Windows | Unicode/ordinary and transport remain covered | Narrow only actually unsupported quote/control/trailing-name or symlink cases; no whole-suite skip |

## Tests

Primary inventory/search regressions live in new `workspace_inventory_exact_paths_test.go`; transport regressions in new `workspace_search_exact_paths_test.go`. The [work order](task-01-preserve-exact-search-paths.md#verification) names exact methods/commands. Cover tracked/untracked Unicode, whitespace, quote/tab/newline cases, an actual untracked stage-header lookalike with a distinct decoy, default/true/false quotePath, ordinary/empty controls, ignored content, Gitlinks, nested/root markers, indexed modes and stage multiplicity. Assert independent expected strings and sentinels, content coordinates/ranges/preview, and before/after Git/index/config/file invariants.

Existing submodule/root, bare sibling aggregation, content eligibility/ranges and admission/cancellation tests provide bounded compatibility checks. Existing `submodule-workspace-search` and `content-search-match-reveal` packages are completed historical delivery records; this correction changes none of their work orders, E2E matrices or prior results.

## End-to-end evidence and mobile assessment

Registered server routes backed by real Git and filesystem are the end-to-end boundary for this data correction. Aggregate search results are decoded and used to request the selected file in the same server. Mobile-parity was assessed: no rendering, touch, navigation, scrolling, breakpoint or frontend logic changes. Desktop and phone consume shared corrected data; no browser, build, ASCII UI preview or local E2E is scheduled.

## Documentation and engineering guidance

Public `sessions-and-review.md` already promises tracked/nonignored content search and independent repository results. The correction restores that documented behavior; no command/config/API shape, workflow, terminology or screenshot changes. No public-doc edit is needed. Reassess if actual implementation scope changes.

Root, backend, agentctl and API engineering guides remain accurate: classified managed Git, instance environment snapshots, containment, scoped reads and cleanup remain intact. No package, adapter, store, exported contract or repo convention changes; no guide correction is necessary. Before publication, recheck the final diff and update the relevant guide if this assessment stops being true. No ADR: this local parser correction preserves an existing contract, and its narrow transport rationale is sufficiently recorded in the owning design/work order.

## Work orders

- [x] [Task 01: Preserve exact search paths](task-01-preserve-exact-search-paths.md)

## Verification results

Task 01 implemented after ROOT's explicit release on 2026-10-05. Production changes only the managed `ls-files` flags and source-local record extraction in `workspace_files.go`. The two new real-Git regression files exercise the accepted inventory/cache/search and registered transport matrix, including independent same-path sentinels, read-only bytes and stage multiplicity. Scope and guide/public-doc assessments remain accurate.

Faithful permanent RED handle 51865 joined with exit 1 (three new families fail, 14 controls pass); GREEN handle 6250 joined with exit 0 (17 top-level tests: 13 process, 4 API; packages 2.694s/1.991s). Scoped two-package lint handle 21986 joined with exit 0 and zero issues against the starting base. Full commands and receipts are in the [work order](task-01-preserve-exact-search-paths.md#results). A prior missing-directory fixture failure was corrected and is not reported as production RED. Supplied ROOT proof remains untouched.

Design catalog, all-spec lint, 36 linter tests, projected documentation coverage and whitespace checks passed before release. Final catalog/specification lint and whitespace checks passed. Actual staged-path documentation coverage is checked before commit; committed-head coverage is checked before publication. The single pinned pnpm 9.15.9 frozen install completed successfully (handle 30915 joined, exit 0). Normal hook/commit/push/ready PR are covered by the initial local lease; hosted CI/review and a separate ROOT merge lease remain delivery gates. No local browser/build/E2E/PG or broad backend suites are scheduled.

## Risks

- Assuming only H tracked records would hide skip-worktree or unmerged entries. Preserve their tags and stage multiplicity.
- Trimming, unquoting or splitting a path's tabs/newlines can select a decoy. Exact path plus distinct sentinel assertions must catch this.
- Cache initialization can make API tests pass empty or leak monitor goroutines. Await an observable complete scan and join only owned managers.
- Native filesystem restrictions vary; scope skips by case, retaining ordinary/Unicode/transport assertions on Windows.
- Resource/transport failures are no pass and require a durable WAITING checkpoint to ROOT, without automatic retry.
