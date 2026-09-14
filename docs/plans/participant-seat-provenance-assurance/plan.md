---
spec: docs/specs/office/requirements/participant-seat-provenance-assurance.md
created: 2026-09-09
status: done
---

# Implementation Plan: Participant Seat Provenance Assurance

## Overview

`participant-seat-provenance.md` is shipped and correct. Three review legs and
an outside-voice pass found no production defect. This initiative closes the two
gaps they did find, and nothing else.

The first is one guard. `Repository.AddTaskParticipant` will refuse a
registration naming an empty agent profile identifier before it begins a
transaction. Today that call is unreachable — both production callers are HTTP
routes that reject it first — so this changes no observable behaviour. It exists
because the store's write is what makes the seat, and a future internal caller
reaching it directly would otherwise write a seat naming nobody: admitted by the
natural key, counted by the quorum guard, never wakeable, never decidable.

The second is verification. Nine criteria of the shipped contract hold by
inspection and are held in place by nothing. Each work order below names the
behaviour that must become falsifiable and the mechanism that makes its window
reachable on purpose. Three of those mechanisms are the substance of this plan,
because the shipped code already answered them wrongly once: a window its own
comment calls impossible to force deterministically, a race left to the
scheduler across fifteen unasserted iterations, and an end-to-end assertion that
watches the wrong field.

`participant-seat-provenance.md` is frozen. Nothing here edits it, and a case
that cannot be made to pass against the shipped implementation is evidence the
assurance spec is wrong — it routes back to the spec step and is never licence
to weaken the case or the code under test (`AC-OFFICE-SEAT-ASSURANCE-002.12`).

## Backend

### The store-boundary guard

- Add an exported sentinel `ErrEmptyAgentProfileID` to
  `apps/backend/internal/office/repository/sqlite/`, so two callers cannot
  compare against two identities.
- Return it as the first statement of `AddTaskParticipant`, before `BeginTxx`
  and therefore before `lockParticipantRoleSeat` acquires the shared exclusion.
  A rejected call contends with nobody and has nothing to roll back.
- Compare against the empty string only. No trimming, no normalisation — a
  guard that trimmed would disagree with the surface guard about what it is
  guarding, and a whitespace identifier is already governed by
  `AC-OFFICE-SEAT-PROVENANCE-005.8`.
- Do not place it in the service layer beside the existing role lookup, and do
  not remove the surface check. Two layers is the intent: the surface returns a
  client error to an operator, the store refuses to write a row.
- Map it to no HTTP status. No shipped route can reach it.

### Test-only support

- A logger-accepting variant of the dashboard test harness, so the
  cancellation-failure case can read the warning the service already emits.
  Existing callers of the harness keep their default logger.

No other production change. No schema change, no migration, no new metric, log
line or counter.

## Frontend

No production frontend change. The E2E work widens one local response type in a
spec file to read a field the API already returns.

## Tests

Grouped by the mechanism that makes each window reachable.

Ordinary sequential cases against a seeded slate:

- Identity probe precedes claim search: a slate holding agent A's `manual` seat
  and agent B's sole undecided `auto` seat, with A registered again, reports no
  change and leaves B's seat naming B with provenance `auto`
  (`AC-OFFICE-SEAT-ASSURANCE-002.1`).
- A displaced agent registered again writes a second `manual` seat: exactly two
  seats, both `manual`, naming both agents (`-002.2`).
- A claim leaves the seat's identifier, decision-required flag, position and
  creation time unchanged alongside the agent and provenance it changes
  (`-002.9`). Creation time is a real column the office projection does not
  carry, so this reads the column directly.
- The claim's activity entry is asserted by content — task, step, role,
  displaced agent, claiming agent, each with its correct value — replacing the
  current count-only assertion (`-002.8`).

Divergent handles, no concurrency:

- The task's current step must be resolved on the writing handle. Build the
  repository with its writer on one store and its reader on a second whose task
  row names a different step; assert the seat lands at the writer's step
  (`-002.3`). Swapping `stepIDForTaskTx` for `stepIDForTask` currently passes the
  whole suite; it must fail this.

Surface refusals:

- An empty identifier over HTTP is a client error and writes no seat (`-002.4`).
- A role outside `reviewer`/`approver` is refused and writes nothing (`-002.5`).
  This is not reachable over HTTP — the routes are role-fixed — so it is a
  white-box case in the service's own package.

Deliberate mid-write window, server dialect:

- A seat selected as claimable and removed before the conditional write applies:
  a holding session takes a row lock, the registration blocks on it, the test
  waits until the block is observable through the server's activity view, the
  holder deletes and commits, and the registration falls through to inserting
  (`-002.10`). Reaching the wait is the proof the window was entered — it
  replaces the existing case's unasserted iteration split. The existing
  fifteen-iteration case is kept, not replaced: it answers a different question.

Contracted residual:

- A claim whose cancellation of the displaced run fails still returns success,
  leaves the claimed slate exactly as a successful claim does, records the
  failure, and leaves the displaced run runnable (`-002.7`).

## E2E Tests

- The existing quorum case already walks an automatic cast followed by a manual
  registration naming a different agent. Widen its local response type to carry
  the required-decision count and assert it is one, alongside the existing role
  assertion rather than instead of it (`-002.6`). A regression to two seats
  leaves the role correct and shows up only here.

## Implementation Waves

Wave 1 (parallel — disjoint files and packages):

- [x] [task-01-store-boundary-guard](task-01-store-boundary-guard.md)
- [x] [task-02-repository-provenance-cases](task-02-repository-provenance-cases.md)
- [x] [task-05-deterministic-claim-window](task-05-deterministic-claim-window.md)
- [x] [task-06-e2e-quorum-required-count](task-06-e2e-quorum-required-count.md)

Wave 2:

- [x] [task-03-surface-refusals-and-claim-activity](task-03-surface-refusals-and-claim-activity.md)

Wave 3:

- [x] [task-04-cancellation-failure-residual](task-04-cancellation-failure-residual.md)

Tasks 03 and 04 both add files to the `dashboard` package and both touch its
shared test helpers, so they are sequenced rather than run together.

## Risks

- **File-size limit.** Revive's 800-effective-line limit applies to test files,
  and new tests go in a new file rather than being appended to a large one.
  `participant_claim_postgres_test.go` is at 788 lines and
  `dashboard/participants_test.go` at 766, so the deterministic-window case, the
  cancellation-failure case and the white-box role case each begin a new file.
  Measure before appending anywhere; a case that crosses the limit fails lint and
  moving it afterwards costs a round.
- **The wait probe.** `-002.10` depends on the registration's block being visible
  through the server's activity view. If that proves unreliable, the named
  fallback is the failpoint idiom this repository already uses for destructive
  cutover migrations — a hook between the claim search and the conditional write,
  nil in production. Take it only if the probe fails in practice, and record why
  in the case's comment. Do not take it first.
- **Divergent-handle setup.** Schema initialisation runs against the writer, so
  the reader store needs its own `tasks` table and row. Every assertion must read
  through the writer handle, never the repository's reader accessor, which by
  construction now points at the wrong database.
- **Skips must stay visible.** A dialect-gated case that silently passed without
  running would report coverage this plan then relies on
  (`AC-OFFICE-SEAT-ASSURANCE-002.11`).
- **Helper drift.** The multi-connection Postgres pool builder is reused, not
  copied. A second copy would invite the two to drift.
- **Scope.** No case may change production behaviour, weaken an existing
  assertion, or relax a criterion of the frozen contract.

## Results

All six work orders landed. Production change is 18 lines in one file
(`participants.go`: the sentinel, the guard, and the doc comment recording
its placement); everything else is test-side.

Each retroactive case was checked for decisiveness by regressing the
implementation and confirming the case went red, then reverting:

| Criterion | Regression applied | Result |
| --- | --- | --- |
| `-002.1` | claim search moved before the identity probe | RED |
| `-002.3` | `stepIDForTaskTx` → `stepIDForTask` (read-only pool) | RED |
| `-002.8` | claim activity payload emptied | RED |
| `-002.9` | claim also bumps `position` | RED |
| `-002.7` | cancellation failure swallowed without a log | RED |
| `REQ-001` | guard absent (sentinel undefined) | RED, then GREEN with the guard |

`-002.10` is server-dialect-only and skips on a runner without
`KANDEV_TEST_POSTGRES_DSN`; it is verified by CI, whose backend-tests
workflow supplies a Postgres service. Its `-count=5` flakiness check and its
wait-probe reliability are therefore unverified locally — if the probe flakes
in CI, the named fallback is the failpoint idiom, per task 05.

`-002.5` gained a positive control (reviewer and approver must reach the
store) so that a body refusing every role could not satisfy it.

`-002.9`'s seed asserts its own distinctive values before the claim, so a
silently-failed seed cannot make the invariance comparison vacuous.

`AC-001.7` (both dialects) is structural rather than tested twice: the guard
compares its own argument against the empty string and issues no query, so
there is no dialect-dependent behaviour for a second case to observe.

## Verification

Targeted commands are owned by each task. After all tasks:

```bash
make -C apps/backend fmt
make -C apps/backend test lint
cd apps/web && pnpm e2e:run tests/office/workflow-quorum-transitions.spec.ts
python3 scripts/lint-spec-files.py --all
```

Commit through active hooks, then push and open a PR against `main`.
