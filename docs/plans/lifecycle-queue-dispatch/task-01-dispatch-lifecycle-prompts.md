---
id: "01-dispatch-lifecycle-prompts"
title: "Dispatch retained lifecycle prompts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-CI-PR-AUTOMATION-001
acceptance_criteria:
  - AC-UI-CI-PR-AUTOMATION-001.9
  - AC-UI-CI-PR-AUTOMATION-001.10
  - AC-UI-CI-PR-AUTOMATION-001.18
system_design:
  - ../../specs/ui/system-design/ci-pr-automation-01.md
---

# Task 01: Dispatch retained lifecycle prompts

## Summary

Make the real SQLite queued lifecycle consumer reach legacy and advertised v1
transport without requiring an ordinary dispatch claim. Keep the ordinary
claim-to-submission contract and all lifecycle admission/acceptance guards.

## In scope

Independently author the tests and matrix named in [the plan](plan.md#tests-and-end-to-end-evidence),
capture the causal red result, select the existing ordinary updater only on the
non-lifecycle path, then run the exact targeted checks below. Assert SQL queue,
claim and v1 submission state rather than only callback counts. Use current
production CI auto-fix binding rather than manufacturing an accepted attempt.

## Out of scope

No production/permanent test edits until ROOT reviews the whole package and
sends a later explicit implementation interrupt to this same primary. No
delegates, new sessions, model switch, schema changes, generic retained bypass,
plan-comment/managed-input repair, async drain redesign, global claim-error
suppression, durable protocol disablement, or unrelated resources.

## Acceptance

1. The new SQLite lifecycle consumer test fails before correction because an
   absent ordinary claim prevents transport; after correction legacy and v1
   lifecycle cases dispatch once per accepted live reservation, retain at the
   handoff boundary, acknowledge on acceptance, and unblock normal signalled
   completion processing without a queued lifecycle head. Existing real
   completion/ready controls establish unchanged signal semantics separately;
   do not add a duplicate workflow simulation.
2. Failed unaccepted handoff retains one retry; archive/reset/current-token,
   session ownership and durable recovery guards still prevent stale sends.
   Valid ordinary claims still persist associations; missing/stale ordinary
   claims fail before transport. Retained non-lifecycle hook eligibility and
   existing receipt/no-replay behavior are unchanged.
3. Exact checks pass with actual exit/join evidence recorded. Any need for
   broader persistence or retained-kind correction returns to ROOT before code
   expansion. Plan and work-order statuses/results match the implemented scope.

## Verification

Run only after implementation authorization AND an explicit ROOT heavy lease.
Use task-local Go 1.26 without altering global mise. From repository root:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
export GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB
export KANDEV_TEST_POSTGRES_DSN=
go version
export KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/child146-4551-red.XXXXXXXX)"
scripts/run-quiet backend-test -- timeout --kill-after=10s 280s sh -c 'cd apps/backend && go test -race -trimpath -tags fts5 -p=1 ./internal/orchestrator -run "^TestQueuedLifecycleDeliverySQLite$" -count=1 -timeout=120s'
```

Capture the original exit and joined process on red; no blind replay after
compile, control, or timeout failures. Diagnose anything other
than the absent ordinary claim rejection before implementing. After correction,
from the same repository root and task-local environment:

```bash
export KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/child146-4551-green.XXXXXXXX)"
scripts/run-quiet backend-test -- timeout --kill-after=10s 420s sh -c 'cd apps/backend && go test -race -trimpath -tags fts5 -p=1 ./internal/orchestrator -run "TestQueuedLifecycleDelivery|TestQueuedOrdinaryDeliveryClaimControls|TestQueuedDeliveryUpdaterEligibility|TestExecuteQueuedMessage_Lifecycle|TestDrainQueuedMessage_Lifecycle|TestExecuteQueuedMessage_PlanComment|TestStartupReconciliationDoesNotRedispatchAttemptedPlanComment|TestManagedInput|TestDeliveryRetention.*Queued|TestProcessOnTurnComplete_ExplicitSignalGating|TestReviewWatchLifecycle_BootReadyStaysOnReviewUntilTurnComplete" -count=1 -timeout=300s'
export KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/child146-4551-lint.XXXXXXXX)"
scripts/run-quiet backend-lint -- timeout --kill-after=10s 400s bash scripts/lint-go-changed apps/backend/internal/orchestrator/event_handlers_agent.go apps/backend/internal/orchestrator/event_handlers_queue_lifecycle_delivery_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Keep checks sequential; retain each returned session handle and join it before
the next heavy command. If more/new Go files become necessary, update this
reviewable file list and lint those exact changed paths, not full-backend lint.
Run the local `validateCoverage` export from `.github/scripts/pr-docs.cjs` with
the changed work order, manifest, owning requirement/design and production path
as input; record its result. Before any later hooks/pnpm, install once from
`apps/` using `pnpm install --frozen-lockfile` under the granted budget.

## Authorized lint budget adjustment

After the canonical check returned exit 4 with an internal timeout, ROOT granted
one new attempt retaining its owning package, configuration and checked diff
base. From repository root, with the same task-local PATH:

```bash
unset GIT_DIR GIT_WORK_TREE GIT_COMMON_DIR GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES
export GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=3GiB GOFLAGS=-p=2 KANDEV_TEST_POSTGRES_DSN=
lint_base=$(scripts/resolve-go-lint-base --comparison-base)
lint_base_rc=$?
if [ "$lint_base_rc" -ne 0 ] || [ -z "$lint_base" ]; then exit 1; fi
export KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/child146-4551-lint-budget2.XXXXXXXX)"
scripts/run-quiet backend-lint -- timeout --kill-after=10s 620s bash -c 'cd apps/backend && exec golangci-lint run ./internal/orchestrator --new-from-rev="$1" --timeout=10m --concurrency=2' child146-lint "$lint_base"
```

The resolver returned exit 0 and base
`530493cce701430cb9b68387eadd1d0126e78c63`. This changes only the authorized
resource/deadline budget; the canonical script and shared environment are
unchanged. Normal hooks may use the same task-local 3 GiB/GOMAXPROCS2/GOFLAGS
settings after this lint passes. Tests retain their reviewed 512 MiB budget.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- New `apps/backend/internal/orchestrator/event_handlers_queue_lifecycle_delivery_test.go`
- The owning requirement/design pair and this package, for synchronized results.

## Dependencies

None. Recheck remote main and relevant changes before implementation. Respect
foreign concurrent work; do not rebase for drift or change foreign resources.

## Risks

The memory queue cannot prove SQL claim behavior. V1 capability advertisement
must reach actual submission preparation and mocked transport; a legacy mock
with a durable label is insufficient. Keep generation, hash, ownership and
recovery checks enabled. Tests must join bounded async consumers and avoid
uncontrolled retry loops.

## Parallelism

`sequential`; no delegation authorized.

## Inputs

- [Requirement](../../specs/ui/requirements/ci-pr-automation.md), `.9`, `.10`, `.18`.
- [Owning design](../../specs/ui/system-design/ci-pr-automation-01.md), retained lifecycle delivery.
- Read-only supporting context: design part 2 lifecycle/auto-fix flow, Platform
  durable-agent-delivery ordinary claim contract, `messagequeue/types.go`,
  `repository_sqlite.go`, `repository_dispatch_recovery.go`,
  `agent_delivery_submission.go`, lifecycle tests, submission tests and managed
  input receipt/settlement tests. ROOT private fixture is excluded.

## Results

Implementation released by ROOT in a later explicit message and performed in
this same primary session. Production correction is lifecycle-only; targeted
legacy/v1 SQLite consumer, CI attempt binding, ordinary claim controls and the
existing lifecycle/workflow controls passed under the exact GREEN command.

- RED original native 86336: exit 1 at the expected zero-transport assertion
  in all six lifecycle cases; normal compile/test completion, 6.692s runtime.
- Initial GREEN: exit 1 on four nil/empty attachment test-oracle assertions.
  Refined GREEN: exit 1 on two post-attempt log-oracle assertions. Corrected
  those fixture assumptions; no extra production changes.
- Final GREEN original native 28156: exit 0, 3.537s test runtime. All observed
  owned wrapper/timeout processes absent after join.
- Canonical scoped lint original native 99905: exit 4. Log reports `0 issues`
  followed by internal `Timeout exceeded`. This is unknown, not a lint pass.
  No blind replay; ROOT explicitly granted the adjusted attempt documented
  above, which subsequently passed.
- Frozen install original native 87426: exit 0, pnpm 9.15.9, 1.7s. Current
  worktree heavy process candidates were empty after join.
- ROOT-authorized canonical-equivalent 10-minute lint: original native 48964
  joined exit 0, `0 issues`. Observed wrapper absent. No passing test replay
  or shared-script modification.
- Catalog/spec/diff and local PR-docs coverage preflight: passed.

Task-defined local checks are complete. Normal hooks, commit/push/PR and hosted
validation are tracked in the external task plan, alongside exact log paths,
original process handles, joins and lease state.
