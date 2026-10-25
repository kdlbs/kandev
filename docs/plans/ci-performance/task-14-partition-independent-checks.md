---
id: "14-partition-independent-checks"
title: "Partition frontend and Windows verification"
status: in_progress
wave: 10
depends_on: ["13-e2e-runner-processes"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-003
  - REQ-PLATFORM-CI-PERFORMANCE-007
acceptance_criteria:
  - AC-PLATFORM-CI-PERFORMANCE-007.1
  - AC-PLATFORM-CI-PERFORMANCE-007.2
  - AC-PLATFORM-CI-PERFORMANCE-007.3
  - AC-PLATFORM-CI-PERFORMANCE-007.4
system_design:
  - ../../specs/platform/system-design/ci-completion-budget.md
---

# Task 14: Partition frontend and Windows verification

## Outcome and scope

Move frontend unit execution into two built-in shards parallel to static checks; partition whole Windows native steps into compilation, platform, persistence and settings groups. Preserve original commands and gates.

## Exclusions

No application behavior, paid capacity activation, trust expansion, assertion
removal, automatic retries or delegation. Preserve unrelated user changes.

## Acceptance

Exact frontend report union and every prior Windows step remain required once. Failure/cancellation/missing-report paths block aggregates. Hosted candidate reports execution, waiting and occupancy.

## Files and dependencies

The backend/frontend/E2E workflow files, their registered contract tests and
CI performance evidence. Task 13 supplies the preceding measured result.

## Verification

```bash
python3 .github/scripts/frontend-tests-workflow-contract_test.py
python3 .github/scripts/backend-tests-workflow-contract_test.py
python3 .github/scripts/ci-gates_test.py
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Results

Local workflow contracts pass: frontend 11 cases, backend 19, runner placement 5,
required gate failure/skip/cancellation paths 5, action pinning 9. Catalog and
specification lint pass. Comparing every Windows step body against base
`5c6cf5196f886bdb29bcccf6530aa48174efef55` proves all 27 bodies unchanged apart
from their owner condition. The native Vitest sequencer selects all 2,948 files
exactly once across four 737-file shards. `vitest list --filesOnly` alone ignores
sharding and is not partition evidence. Hosted timing and case outcomes remain
pending; local contracts and enumeration do not prove speed.

Public documentation is unchanged: this affects repository CI scheduling only.
The contributor runbook and internal delivery documents describe the new layout.

### First hosted attempt and remediation

Head `6025705717` frontend run 38577314508 failed before tests: pnpm 9 parsed
`test --shard` as pnpm options. Use `run test` explicitly; the real package
script under production NODE_ENV passed the six worker-budget tests with shard
and JSON options forwarded and its pretest generators executed.

Backend run 38577314273 retained passing compile and native groups, but the
new persistence/settings groups hit their original six-minute step budgets
while cold-compiling dependencies with a 512 MiB Go memory limit. The restored
setup-go cache was only 276 MiB and still needed module downloads and race
compilation. Give the existing two-process Go compiler a 2 GiB soft heap budget
on the hosted Windows runner. Preserve every test selector, four-minute test
alarm and six-minute step deadline. These failed attempts are not performance
acceptance; rerun the complete groups before claiming improvement.

The next candidate uses two frontend shards instead of four: the measured first
attempt's startup cost repeats per job, and two partitions should fit within the
full-CI budget while retaining the 10% runner-minute adoption constraint. Four
shards remain only the initial failed-attempt configuration and enumeration
evidence. Same-source hosted one-versus-two comparisons decide adoption.

### Windows process follow-up

The unchanged process job 115897903616 completed successfully in 19m08s, including
17m44s in the two-process package step. Move its existing two native cohorts to
two hosted Windows jobs with the same inventory/selector/completion validator.
Keep the no-argument local mode, make selected-cohort scope explicit in each
report, preserve race/deadlock budgets and join both jobs in the required gate.
Verify with `go test -trimpath -race ./cmd/windows-process-tests` from
`apps/backend`, the backend workflow contracts, and native hosted results.

### Second candidate local verification

The native Vitest sequencer retains the same 2,948 files exactly once across two
1,474-file shards. The selected Windows cohort launcher passes its race-enabled
unit suite (2.036 seconds), retaining the no-argument two-cohort mode and rejecting
invalid selection. Backend workflow contracts pass 22 cases, including executable
sidebar nested-selection coverage and both required Windows process jobs.

### Cold Windows compilation boundary

In the second candidate, persistence job 115914242608 produced no test results
before its six-minute step deadline. Settings job 115914242752 passed the three
small packages (1.155s, 1.255s, 1.391s) then spent the remaining time compiling
the large dependency closure. The 2 GiB heap did not resolve the cold build.

Add a bounded compilation warmup before these two groups: use the exact union
of their existing package selectors, the same trimpath/fts5/race mode and a
zero-match run selector. Allow two compilation packages with GOMAXPROCS=2 and
GOMEMLIMIT=2GiB; no test assertions run during warmup. Its ten-minute step bound
and thirty-second empty-test bound are independent of the existing six-minute
targeted steps and four-minute test alarms, which remain unchanged. This
restores the compilation-before-tests separation previously supplied by the
monolithic native job. Measure the entire job, including warmup; a deadline
change alone is not a performance improvement.

### Four native Windows process cohorts

The two hosted process groups passed in 13m40s and 16m14s. The slower group
alone exceeds the full budget. Extend the same native sorted inventory partition
to four hosted cohorts; retain two as the no-argument local default. Accept only
these supported cohort counts and reject out-of-range selection before work.
Reports retain inventory, selected count/index, exact completion validation and
true process exits. Existing race, 25-minute deadlock and job bounds remain.
Verify default compatibility, four-way exact name coverage (including duplicate
name occurrences in separate packages), invalid counts and full native CI.
