---
created: 2026-10-12
status: done
requirements:
  - REQ-PLATFORM-GIT-SUBPROCESS-ADMISSION-002
  - REQ-PLATFORM-GIT-CAPTURE-RECOVERY-001
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-004
system_design:
  - ../../specs/platform/system-design/git-subprocess-execution.md
  - ../../specs/platform/system-design/workspace-git-capture-recovery.md
  - ../../specs/workspaces/system-design/local-repositories.md
legacy_specs: []
---

# Implementation plan: Git and polling overhead

## Overview

Independent reductions of repeated background Git work. Each work order has
its own result, verification, and commit, so an order can be reviewed or
dropped without touching the others.

- Task 01: Git executable resolution walks `PATH` for every managed Git command.
- Task 02: Each Changes capture spawns four `rev-parse --verify` processes per
  evidence read, and every capture reads its evidence twice.
- Task 03: A slow-mode tracker scans every 30 seconds while nothing changes.

## Evidence and confirmed scope

Baseline is upstream `main` at `add376a2af17753ec349d9e7b042306c0f91100b`.
Source reading at that commit confirms each cost:

- `subproc.NewGitCommand` calls `exec.CommandContext(ctx, "git")`, which runs
  `exec.LookPath` per command. `GitExecutablePath` also calls `LookPath`. On
  Windows each lookup stats `PATH` x `PATHEXT` candidates. A downstream Windows
  deployment reported `os/exec.LookPath` at about 1.65 cores in a 20-second
  agentctl CPU profile; that number is reported, not re-measured here.
- `captureGitStatusComparisonEvidence` calls `gitRefOID` four times, each a
  separate `rev-parse --verify`. `validateBasicGitStatusEvidence` repeats the
  read. `gitRefOID` never returns an error, so a batch keeps the same error
  contract.
- `pollIntervals(PollModeSlow)` returns a fixed 30 seconds for both loops.
  Upstream already pauses idle sessions (`AC-WORKSPACES-LOCAL-REPOSITORIES-004.4`),
  so the remaining cost is unfocused trackers with an operation in flight and
  slow UI interest. A downstream 32-tracker slow-mode measurement over 420
  seconds reported process CPU 1.02 -> 0.51 cores; reported, not re-measured.

No open upstream pull request changes the production files in scope.

## Contract changes

- `AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.7` (new): reuse the resolved Git
  executable while `PATH` and `PATHEXT` are unchanged and the target remains executable.
- `AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.7` (new): one process per comparison
  evidence read, with per-reference fallback that keeps published values equal.
- `AC-WORKSPACES-LOCAL-REPOSITORIES-004.6` (new, Task 03): proposes a change to
  the documented unfocused cadence. The design fixed both slow loops at 30
  seconds; an unchanged loop can now lengthen to 5 minutes, and a change or
  mode push restores 30 seconds. An unfocused badge can therefore lag a change
  by up to 5 minutes after a long quiet period. Focused clients, turn-end final
  scans, and paused behavior are unchanged.

The Workspace Git Status requirement and design are within 100 bytes of their
size limits, so the comparison-reference rule lives in the capture recovery
pair, which owns the capture and validation evidence reads.

## Scope

### In scope

- `internal/common/subproc/git_command.go` executable memo and its unit tests.
- `agentctl/server/process` comparison reference batching and its unit tests.
- `agentctl/server/process` slow-mode idle backoff and its unit tests.
- The requirement criteria, design sections, this plan, and work orders.

### Out of scope

- Admission classes, deadlines, noninteractive environment preparation, or the
  raw Git audit.
- Other `rev-parse` callers, enrichment, and corrective capture policy.
- Fast and paused intervals, the startup grace, mode aggregation, focus
  handling, and turn-end final scans.
- New metrics, flags, settings, frameworks, integration or E2E suites.

## Work orders

- [x] [Task 01: Remember the resolved Git executable](task-01-remember-git-executable.md) (wave 1, no dependencies)
- [x] [Task 02: Resolve comparison refs in one Git process](task-02-batch-comparison-refs.md) (wave 1, no dependencies)
- [x] [Task 03: Back off unchanged slow-mode polling](task-03-slow-poll-idle-backoff.md) (wave 1, no dependencies)

## Verification strategy

Each order adds source-adjacent unit tests that fail on the baseline and pass
after its change, and keeps its existing neighbouring tests green. Commands
run only the named tests with `-run`; package-wide runs are not part of this
package. Backend `go build`, `go vet`, and scoped lint run once on the changed
packages. Documentation checks run once:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run `.github/scripts/pr-docs.cjs` `validateCoverage` locally with the actual
changed paths before publication.

## E2E, docs, and mobile assessment

All orders change backend tracking without layout, touch, navigation, or API
changes. The public Git operations guide documents the five-minute unfocused
badge freshness bound and immediate focus/refresh/turn-end behavior. Existing
browser/mobile behavior remains unchanged; full hosted CI validates integration.

## Risks

- Task 01: a stale remembered path after an in-place upgrade is bounded by the
  direct executable check; a test that swaps `PATH` resolves again.
- Task 02: `rev-parse` without `--verify` echoes unresolvable arguments; only
  a zero exit with one object ID per argument is accepted.
- Task 03: an unfocused badge can lag a change by up to 5 minutes after a long
  quiet period. Maintainers may prefer a different cap; the constants are local.
