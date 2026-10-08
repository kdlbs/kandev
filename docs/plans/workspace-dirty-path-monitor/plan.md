---
created: 2026-10-02
status: in_progress
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
system_design:
  - ../../specs/platform/system-design/workspace-dirty-path-monitor.md
legacy_specs: []
---

# Implementation plan: Exact dirty-path monitor refresh

## Overview

Preserve raw tracked filenames in the quick workspace monitor so a second edit
to an already-dirty file starts the existing refresh branch. One sequential
work order covers real-Git regressions, the minimal correction, and delivery.
Implementation starts only after a later explicit parent interrupt.

## Evidence and confirmed scope

Baseline is clean `9fffff9c4ed1aea5d0a1189b0e1877146f7aacfd` after PR 4162.
The parent supplied a completed production `getWorkspaceState`/`changed` probe:
`TestTemporaryDirtyPathRefreshProbe` failed for leading space, trailing space,
tab, newline, double quote, and Unicode with `core.quotePath=true`; plain.txt
passed. Two post-baseline writes used fixed mtimes ten seconds apart.
Command 12420 was joined with exit 1, package duration 0.235s. Its temporary
repository test was removed; `/tmp/kandev-dirty-path-refresh-repro.go` remains
parent-owned until actual merge. This package does not rerun that proof.

Source confirms `git diff-files --name-only` returns text, then
`buildDirtyFilesID` trims/splits lines before statting those textual names.
The dirty path list remains constant across repeated edits. Missing exact-file
mtime evidence therefore loses the quick state change and all actions in its
`monitorTick` branch. This is a missing explicit polling criterion, now `.42`,
within the active Workspace Git Status requirement, not a new vertical contract.

Assumption check: exact filenames, mtime polling, one work order, native supported
cases, direct ticks, resource bounds, and normal merge are caller-confirmed.
Source verifies the missed path and the separate porcelain-entry Git poll.
There is no unresolved material choice and no additional ADR is required.

## Scope

### In scope

- NUL-framed dirty tracked query and parsing with exact-path stat evidence.
- State and subscriber regressions for clean-to-dirty, dirty-to-second-edit,
  stable no-op, decoy identities, supported quoting/filename modes, deleted and
  renamed paths, repository scope, and tracked dependency paths.
- Existing focused admission, environment, untracked policy and status tests.
- Internal requirement/design/plan traceability and actual hosted delivery.

### Out of scope

- New hash/content scanning, watchers, generic parsers, UI, copy or API changes.
- Changing untracked/index policies, cadence, admission/deadlines, subscription
  payloads, repository containment, or full-observation/enrichment fingerprints.
- Git mutation, numstat/count/path/literal/status metadata repairs already merged.
- Running app/browser/DB, broad local suites, E2E, background polling test loops,
  extra workers/tasks/sessions, optional cleanup/polish, or main-only rebases.

## Technical approach

`workspace_monitor.go:getWorkspaceState` adds `-z` to its existing polling Git
query. `buildDirtyFilesID` splits only on NUL and retains exact path bytes for
`sanitizePath` and `os.Stat`. Use NUL separators between path and optional mtime
evidence to avoid punctuation-based record ambiguity. Missing files retain their
path. Keep the three state fields and `changed` comparison and tick path intact.
No shared parser or untracked-fingerprint refactor is necessary.

`workspace_monitor_dirty_paths_test.go` uses real Git and direct ticks with
buffered stream capture. Reuse disposable repo, copied environment, publication
and enrichment-completion helpers. Stop and join tracker-owned work at cleanup;
do not call `Start` or add production test hooks. Evidence must prove accepted
status, refreshed cached file data, and the actual `refresh` stream payload.
Gate or join existing enrichment when separating tick events from later detail
events, so an unchanged tick assertion cannot race asynchronous publication.

The Git poll may mitigate initial membership/classification changes. Its
porcelain-entry hash need not change on repeated dirty edits; its index-only
handler refreshes status without a Files refresh. Explicit/focus/direct-mutation
refreshes remain possible. Report the proven missed monitor change, without
claiming total permanent stale UI.

## Tests

| Criteria | Planned evidence |
| --- | --- |
| `.42` | `TestWorkspaceStateDirtyExactPaths`: fixed-mtime real-Git state transitions, no-op, default/true/false quotePath, supported native names and exact decoys |
| `.42` | `TestMonitorTickDirtyExactPaths`: direct ticks, initial dirty and second edit, accepted status, current file metadata, repository-qualified refresh payload, settled no-op |
| `.42`, `.14` | `TestWorkspaceStateDirtyPathTransitions`: tracked dependency path, dirty deletion, staged rename followed by repeated destination edit, stable controls |
| `.13` through `.15` | Existing untracked exclusion/query tests |
| `.31` | Existing deadline/admission/overlap tests and detached Git environment controls |

## E2E evidence and docs/mobile assessment

The real-Git producer-to-subscriber test is the end-to-end evidence for this
backend event contract. There is no frontend change or viewport-dependent
behavior. Desktop and phone consume the same payloads; mobile-parity assessment
requires no layout preview, copy, or browser/E2E addition. PR 4162's separate
Git diff file metadata specification stays unchanged.
Public Git/Files guides, README and screenshot catalog need no change: this
restores existing refresh behavior without changing operator procedures,
interfaces, settings or terminology. Internal docs are updated.

## Work orders

- [x] [Task 01: Restore exact dirty-path refresh](task-01-exact-dirty-path-refresh.md) (implementation verified; hosted delivery pending)

## Verification results

Design checkpoint on 2026-10-02:

- `python3 scripts/list-docs.py validate`: passed, 343 decisions and 1309 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed; status confirms all five artifact files are unstaged.
- `.github/scripts/pr-docs.cjs:validateCoverage` with complete referenced contents
  and explicitly simulated planned source/test paths: `covered`, no errors.
  The first shell attempt lacked `node`; the repository-pinned runtime via
  `mise exec -- node` completed successfully without an install or cache mutation.
- Existing status design is 32,580 bytes, below its 32 KiB limit. The monitor
  supplement avoids expanding that near-limit document with an unrelated rewrite.
- Source inspection confirms the parallel Git poll limitations described above.
  Parent's proof is retained as supplied evidence and was not replayed.

That checkpoint left production/permanent tests unchanged and owned handles joined.

Implementation continuation on 2026-10-02:

- Permanent RED: exact state and direct tick tests failed for repeated dirty
  writes, stale diff/cache, missing refresh/status events, and decoy mtime capture.
  Plain and punctuation controls passed. Joined exit 1, package 5.023s.
- Focused GREEN command in Task 01: passed with `-trimpath -race -p 2`, package
  11.523s. All supported native shapes and default/true/false quotePath modes
  ran, plus deletion, rename, tracked dependency and preserved boundary tests.
- Scoped `golangci-lint` command: passed, zero issues.
- Catalog validation/spec lint and actual-path documentation coverage preflight:
  passed; coverage `covered`, no errors. Whitespace gate passed.
- Active hooks confirmed. Worktree dependencies installed once via
  `mise exec -- pnpm install --frozen-lockfile` from `apps/`, passed in 2.2s.
  No tracked lockfile or unrelated file changed.
- `currentFiles` supplies path membership and capture timestamp, not populated
  file mtime/size evidence; regression asserts recapture plus published new diff.
- All owned local check/install handles joined. Normal commit, hosted CI/review,
  actual expected-head squash merge and owned cleanup are the remaining
  task-level delivery gates; implementation verification alone is not completion.

## Risks

- Native filesystem filename restrictions must skip only unsupported shapes.
- An asynchronous detail notification must not masquerade as a tick event.
- Index-mtime changes or decoy mtimes must not accidentally make a faulty
  fingerprint pass. Fixed times and explicit index/decoy controls isolate it.
- Stat races and timestamp-preserving writes retain existing polling limits.
- Shared caches, edits, published heads and owned process handles must remain safe.
