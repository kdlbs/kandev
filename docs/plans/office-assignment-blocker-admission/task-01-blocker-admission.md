---
id: "01-blocker-admission"
title: "Gate Office assignment admission on pending blockers"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-SCHEDULER-002
acceptance_criteria:
  - AC-OFFICE-SCHEDULER-002.1
  - AC-OFFICE-SCHEDULER-002.2
  - AC-OFFICE-SCHEDULER-002.3
  - AC-OFFICE-SCHEDULER-002.4
  - AC-OFFICE-SCHEDULER-002.5
  - AC-OFFICE-SCHEDULER-002.6
  - AC-OFFICE-SCHEDULER-002.7
  - AC-OFFICE-SCHEDULER-002.8
  - AC-OFFICE-SCHEDULER-002.9
system_design:
  - ../../specs/office/system-design/scheduler-01.md
---

# Task 01: Gate Office assignment admission on pending blockers

## Summary

Extend Office's shared assignment eligibility predicate with authoritative persisted
blocker reads. Independently reproduce the unwanted run admission, then prove the
three affected producers hold pending work and the existing resolution path releases it.

## In scope

- `shared.IsAssignmentWakeEligible` and its structural lookup contract.
- TaskCreated/TaskUpdated subscriber, recovery, and assignee mutation integrations;
  preserve their existing calling boundaries and use the single shared check.
- Minimal onboarding `Repository` interface compatibility for its existing helper
  call; preserve its no-blocker creation and step/keyless policy.
- Independent permanent real-schema/writer regressions and targeted policy controls
  described in the [plan's test matrix](plan.md#tests).

## Out of scope

- FAILED/CANCELLED/archive/missing policy redesign, #4548, #4553/PR #4559,
  global/claim-time scheduling, generic Kanban changes, schema/flag/metric work,
  provider execution, frontend/copy, and unrelated lifecycle or preservation work.
- ROOT's private candidate source/runner and any replay of its qualification.
- Additional sessions, native delegates, model switches, or protected-resource cleanup.

## Acceptance

1. All three existing real producers admit zero assignment runs with any known
   pending prerequisite, including mixed states; positive no-blocker/completed
   controls remain admitted and Backlog remains held. Producer tests must fail
   behaviorally before the fix and pass afterward with real schemas and run admission.
2. Partial completion remains held; final completion reaches the existing actual
   blocker-resolution producer and persists a legitimate resolution run. Recovery
   progresses beyond held candidates and can reconsider eligible held work.
3. The complete compatibility matrix passes: step fail-open does not bypass a known
   pending blocker; previous-runner interruption, assignment writes/identity,
   rate/pause/causation, Office terminal/archive/missing policy, and onboarding remain
   intact. No run placeholder or alternate launch path is introduced.

## Dependencies

None. ROOT explicitly released implementation to this same primary session on
2026-10-18 after accepting the corrected design and granted the local-heavy lease.

## Inputs

- [REQ-OFFICE-SCHEDULER-002 and criteria](../../specs/office/requirements/scheduler.md).
- [Assignment eligibility design](../../specs/office/system-design/scheduler-01.md#assignment-wake-step-eligibility).
- [Global scheduler ownership](../../decisions/2026-08-01-global-run-scheduler-ownership.md).
- Existing `assignment_step_eligibility_test.go`, `scheduler_recovery_step_eligibility_test.go`,
  `reactivity_step_eligibility_test.go`, `reactivity_blockers_resolved_test.go`, and
  `blockers_resolved_key_convergence_test.go` are patterns, not substitutes for the
  new actual-producer regressions. Some use reduced task schemas; do not copy that
  limitation into this repair's integration fixtures.
- Task dependency policy is independently owned by
  [the task design](../../specs/tasks/system-design/task-dependencies.md). Its
  successful-only terminal policy is not being imported into Office here.

## Files likely touched

Production responsibility:

- `apps/backend/internal/office/shared/assignment_eligibility.go`
- `apps/backend/internal/office/onboarding/service.go` (structural interface only)
- `apps/backend/internal/office/service/event_subscribers.go`
- `apps/backend/internal/office/service/scheduler_recovery.go`
- `apps/backend/internal/office/scheduler/reactivity.go`

The last three should need no new independent blocker logic; change only necessary
call/interface wiring and accurate gate comments. Do not modify existing resolution
logic or repository state policy. The repository already supplies the required reads.

Permanent test responsibility:

- `apps/backend/internal/office/service/assignment_blocker_admission_test.go`
- `apps/backend/internal/office/service/assignment_blocker_recovery_test.go`
- `apps/backend/internal/office/service/assignment_blocker_resolution_test.go`
- `apps/backend/internal/office/service/assignment_blocker_office_scope_test.go`
- `apps/backend/internal/office/shared/assignment_blocker_eligibility_test.go`
- Existing shared lookup fakes if the structural contract widens.

Delivery records: this work order and `plan.md`; update pending/in_progress/done and
actual command results together. Keep requirements/design truthful if implementation
evidence requires a correction within scope.

## Implementation sequence

1. On ROOT release, preserve this primary identity and any existing long-command
   handles across interrupts. Recheck relevant base/dependency changes before editing.
   Mark this work order in_progress. No proof replay is needed if dependencies remain
   unchanged. Run serially only while the heavy lease is held.
2. Independently construct isolated real settings/task/workflow/Office repositories
   (`settings/store.Provide`, `task/repository/sqlite.NewWithDB`,
   `workflow/repository.NewWithDB`, `office/repository/sqlite.NewWithDB`). Use a shared
   per-test database with foreign keys enabled, real CLI parent/explicit AgentID,
   workspace, workflow/step, runner, tasks and blocker writers. Verify FK enforcement
   and seed validity before asserting the producer. No hand-created substitute task
   schema. Use deterministic bus completion and cleanup barriers.
3. Add producer regressions named in the plan; run the focused RED command below
   against unchanged production. Require a causal zero-versus-one assignment failure
   with positive controls passing. A compile/setup/FK failure is not RED evidence.
   Retain raw output and every returned execution/join handle.
4. Extend the shared lookup and predicate with `ListTaskBlockers` and
   `IsTaskInTerminalStep`. Preserve step behavior and logging; scan past unknown
   blocker-state reads so any known pending blocker wins. Keep old-assignee interrupt
   before gating. Update onboarding's lookup interface as needed, without changing its
   task creation. Add error/terminal/archive/missing policy controls to the shared test.
5. Prove real final-resolution release from both service completion dispatch and
   assignee-reactivity completion; no spy-only claim of durable wake admission. Prove
   partial resolution remains held. Preserve existing keys and normal admission gates.
6. Run focused GREEN, then one race pass across all four affected helper/consumer
   packages, then changed-Go lint over actual paths and their complete owning packages.
   Rerun only affected checks after a real change/failure. Run the light docs/ref gates.
7. Record actual exits, package results, log paths, joins, and current-owned process
   cleanup. Return the heavy lease with completed joins to ROOT and perform its
   authorized hosted validation directly in that same execution turn. Keep publication
   and merge instructions from ROOT externally authoritative. Merge requires its
   separate later expected-head grant; never use admin/bypass/delete-branch.

## Verification

Run from repository root in Bash after the explicit implementation/heavy release.
Existing binary directories are pinned below. The pnpm installation directory has
no `/bin` suffix. Do not replace/delete dependency trees or shared caches. Before
any later pnpm command or hook in this fresh worktree, perform the one required
`(cd apps && pnpm install --frozen-lockfile)` under the released resource budget;
no install is part of this design turn.

Set up once per execution shell:

```bash
set -euo pipefail
export PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/bin:/usr/bin:/bin:$PATH"
export GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB
export GOFLAGS='-trimpath -tags=fts5 -p=1'
office_root=$(git rev-parse --show-toplevel)
cd "$office_root"

office_test() {
  local office_tag=$1
  local office_go_timeout=$2
  local office_outer_timeout=$3
  shift 3
  local office_logs
  office_logs=$(mktemp -d /tmp/kandev-d16574cc-"$office_tag".XXXXXX)
  KANDEV_RUN_QUIET_DIR="$office_logs" scripts/run-quiet "$office_tag" -- \
    /usr/bin/timeout --kill-after=10s "$office_outer_timeout" \
    bash -c 'cd apps/backend && exec /home/jcfs/.local/share/mise/installs/go/1.26.0/bin/go test -race -trimpath -tags=fts5 -p=1 -count=1 "$@"' _ \
    "-timeout=$office_go_timeout" "$@"
}
```

Focused RED/GREEN use an internal Go deadline of five minutes and an outer GNU
deadline of six minutes, with kill-after 10 seconds. The one complete four-package
race pass uses an internal Go deadline of ten minutes and an outer GNU deadline of
eleven minutes, with kill-after 10 seconds. The helper requires both explicit budgets.

RED, before production changes (expected Go exit 1 from causal assertions only):

```bash
set +e
office_test office-blocker-red 5m 6m \
  ./internal/office/service ./internal/office/scheduler \
  -run '^TestOfficeAssignmentBlocker(Admission|Recovery|Mutation|Resolution)$' -v
office_red_exit=$?
set -e
[ "$office_red_exit" -eq 1 ]
```

Read the recorded log: the wrapper's expected nonzero is not sufficient. Check
pending cases fail for the unwanted persisted assignment while unblocked/backlog
controls pass. Report the actual wrapper, Go, and join results without inferring
one from another.

GREEN and full affected-package race coverage after the fix:

```bash
office_test office-blocker-green 5m 6m \
  ./internal/office/service ./internal/office/scheduler ./internal/office/shared \
  -run '^(TestOfficeAssignmentBlocker(Admission|AdmissionKanban|Recovery|Mutation|Resolution)|TestAssignmentBlockerEligibility)$' -v
office_test office-blocker-packages 10m 11m \
  ./internal/office/shared ./internal/office/service \
  ./internal/office/scheduler ./internal/office/onboarding
```

Lint from root after formatting changed Go files. This collects actual tracked and
untracked Go changes, then the repo helper lints their complete owning packages.
The helper owns its internal five-minute deadline; the outer GNU deadline is six
minutes with kill-after 10 seconds. Lint memory target is capped at 3GiB and Go
build parallelism remains one.

```bash
office_lint_logs=$(mktemp -d /tmp/kandev-d16574cc-office-lint.XXXXXX)
KANDEV_RUN_QUIET_DIR="$office_lint_logs" GOMEMLIMIT=3GiB \
  scripts/run-quiet office-blocker-lint -- \
  /usr/bin/timeout --kill-after=10s 360s python3 - <<'PY'
import subprocess
tracked = subprocess.check_output(
    ['git', 'diff', '--name-only', 'HEAD', '-z']).decode().split('\0')
untracked = subprocess.check_output(
    ['git', 'ls-files', '--others', '--exclude-standard', '-z']).decode().split('\0')
paths = sorted({p for p in tracked + untracked
                if p.startswith('apps/backend/') and p.endswith('.go')})
if not paths:
    raise SystemExit('No actual changed Go paths; do not claim lint coverage')
print('Changed Go paths:', *paths, sep='\n', flush=True)
subprocess.run(['gofmt', '-w', *paths], check=True)
subprocess.run(['bash', 'scripts/lint-go-changed', *paths], check=True)
PY
```

No full-backend, task-wide, Node package test/lint/build, or overlapping broad replay
is scheduled. A newly changed owning package requires its own bounded race coverage
and changed-Go lint; report a material scope change to ROOT.

Light design/documentation gates (allowed in the design turn):

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/office docs/plans/office-assignment-blocker-admission
git status --short -- docs/plans/office-assignment-blocker-admission
```

Also validate the actual changed file set and its four-document package with
`.github/scripts/pr-docs.cjs`'s exported `validateCoverage`, using tracked diffs plus
untracked files and actual filesystem content. Report its actual status (docs-only
can be `exempt` with `errors: []`). Independently check work-order REQ/AC existence,
requirement membership in the referenced design, design inclusion in the manifest,
manifest backlink, and local Markdown references. An exempt coverage result alone
does not validate references. Do not invent runtime changed paths to obtain covered.

## Risks

See the [plan](plan.md#risks). In particular, blocker reads are admission-time
observations; the fix makes no claim-time atomicity guarantee. Unknown reads preserve
prior assignment admission, with known-pending precedence. Final resolution must
be proven through actual run admission to rule out a silent strand.

## Parallelism

`sequential`

## Results

Design checkpoint completed on 2026-10-18 (historical docs-only results):

- `python3 scripts/list-docs.py validate`: exit 0; 386 decisions and 1638
  specifications validated (completed before ROOT's correction interrupt; no rerun).
- `python3 scripts/lint-spec-files.py --all`: exit 0 initially and after the
  requirement/work-order correction; all specification files passed. No linter unit
  tests ran.
- Local Node inline invocation of `.github/scripts/pr-docs.cjs` `validateCoverage`
  against the actual four changed documents: exit 0, status `exempt`, `ok: true`,
  `errors: []`. No runtime paths were fabricated to force `covered`.
- Independent Node reference/budget checks: exit 0; one requirement, all nine ACs,
  one work order, manifest/design membership and backlink, and 30 local links/anchors
  passed. Explicit 5m/6m focused and 10m/11m package budgets were verified.
- `git diff --check -- docs/specs/office docs/plans/office-assignment-blocker-admission`:
  exit 0. `git status --short -- docs/plans/office-assignment-blocker-admission`:
  exit 0, new plan directory present and untracked as intended.

All light command calls returned terminal exit 0; no background session handles,
current-owned long-running process groups, temporary fixture source, or cleanup work
remain from this design phase. ROOT's historical qualification joins are evidence,
not child execution. No permanent tests, production edits, install, package checks,
hooks, staging, commit, push, or PR have run in this design phase.


### Implementation evidence

ROOT's later explicit release was followed in this same primary session; no delegates
or extra sessions were used. Producer tests share one real-schema fixture in the
external service test package and invoke the actual scheduler public mutation API.
No production test-only export or alternate launch path was introduced.

- Frozen apps install: original handle 14074, joined 3a8815, exit 0. Log:
  `/tmp/kandev-d16574cc-install.DLYOAr/kandev-run.office-install.YZCVaGb7.log`.
- Fixture setup history retained separately: handle 61932 / join 23fa45 failed to
  compile because Task has no generation field; 60653 / join 436a53 exposed the
  real create generation and recovery's config-loader dependency. Both exited 1
  and their owned groups were absent. These are not causal RED evidence.
- Independent causal RED: handle 34019 / join 41f98c, exit 1, service 4.484s.
  TaskCreated explicit/fallback and TaskUpdated pending/mixed cases, reassignment,
  and recovery fail on actual unwanted persisted runs. All nine bus no-blocker,
  completed and Backlog controls plus two unblocked mutation controls pass.
  Both completion producers reach actual partial/final resolution admission.
  Log: `/tmp/kandev-d16574cc-red-causal.6f07wh/kandev-run.office-blocker-red.tzDdTIXS.log`.
- Focused GREEN: handle 79703 / join e7ac30, exit 0; six top-level tests and 51
  subtests pass. Includes real terminal/archive/missing/Kanban policy controls,
  both actual resolution producers, recovery quota/reconsideration, generation/
  key/actor/runner preservation, and shared unknown/pending precedence.
  Log: `/tmp/kandev-d16574cc-green.8modqo/kandev-run.office-blocker-green.LtMWYFBC.log`.
- One complete four-package race: handle 54221 / join 75070f, exit 0. Shared
  2.639s, service 18.247s, scheduler 2.299s, onboarding 1.292s. Log:
  `/tmp/kandev-d16574cc-packages.zAoWmr/kandev-run.office-blocker-packages.DzpiQuTf.log`.
- Actual runtime documentation coverage: exit 0, `covered`, `errors: []`, one
  work order mapping REQ-OFFICE-SCHEDULER-002 through the owning design. No paths
  were fabricated. Final lint/docs/hook outcomes remain to be recorded.

Each recorded Go invocation used local Go 1.26.0, GOTOOLCHAIN=local, GOMAXPROCS=2,
GOMEMLIMIT=512MiB, race/trimpath/fts5/p1/count1 and the reviewed internal/outer budgets.
The causal RED, GREEN and package execution receipts under
`/tmp/kandev-d16574cc-evidence/` record actual waits and owned-group absence. Tests
prove durable run admission; no actual agent/provider was launched.


Scoped lint initially rejected two deprecated Office Run aliases in the new test
fixture (handle 5607 / join c57cd4, exit 1, owned group absent). The fixture now
uses the canonical `internal/runs/models.Run`. Only the affected producer test
selection was rerun: handle 22899 / join 325873, exit 0, owned group absent, log
`/tmp/kandev-d16574cc-alias-check.ST1Wzd/kandev-run.office-blocker-alias.wanDUuzQ.log`.
No production logic changed after the completed package pass; that broad pass was
not repeated. Catalog/all-spec/whitespace checks also exit 0 (ca88d1).


Final scoped actual-path lint passed: handle 21049 / join 53c5c3, exit 0, zero
issues across the complete owning packages; GOMEMLIMIT=3GiB, internal five-minute
and outer six-minute/kill-after-10-second deadlines. Log:
`/tmp/kandev-d16574cc-lint-fixed.brWT3n/kandev-run.office-blocker-lint.QR0CtNkH.log`.
All local test/lint commands are terminally joined and their current-owned process
groups are absent. Production and permanent test work is complete. Normal active
hooks, commit/push and hosted PR validation follow as delivery, with their actual
receipts kept in the external task record.
