---
id: "05-deterministic-claim-window"
title: "Claim target vanishes mid-write, on purpose"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/office/requirements/participant-seat-provenance-assurance.md"
---

# Task 05: Claim target vanishes mid-write, on purpose

Implements `AC-OFFICE-SEAT-ASSURANCE-002.10`, pinning
`AC-OFFICE-SEAT-PROVENANCE-004.8`.

## Problem

This is the window that protects `attemptClaim`'s zero-rows fallthrough: a seat
selected as claimable by `findClaimableAutoSeat`, removed before `claimAutoSeat`'s
conditional write applies, and the registration falling through to inserting its
own seat rather than completing having written nothing.

The shipped comment on that path says the window "cannot be forced
deterministically". The Postgres case that stands in for it
(`TestPostgresAddTaskParticipant_ConvergesWithConcurrentRemoveOfClaimTarget`)
runs fifteen concurrent iterations whose split it logs but does not assert — so
a run in which the window was never entered passes identically to one in which
it was. A regression in the fallthrough could pass on an unlucky scheduling run.

The premise is wrong, and the reason is visible in the code: the claim search is
a plain read and the claim is a conditional `UPDATE` of one row by identifier. A
plain read is not blocked by a row lock; an `UPDATE` is. An outside session can
stand exactly between them.

## Acceptance

New file in `repository/sqlite`. `participant_claim_postgres_test.go` is at 788
lines against a 800-effective-line limit — do not append to it.

Server dialect, three connections:

1. A holding session opens a transaction and takes a row lock on the seat
   (`SELECT ... FOR UPDATE`). Nothing else is locked.
2. The registration runs in a goroutine. Its advisory lock is uncontended, its
   identity probe misses, its claim search reads the seat *through the row lock*
   and selects it as the candidate. Its conditional write then blocks.
3. The test waits until the registration's backend is observably waiting on a
   lock, through the server's own activity view. **Reaching this wait is the
   proof** — it establishes the candidate was selected and the write not yet
   applied, which is the window itself. This replaces the existing case's
   unasserted iteration split.
4. The holding session deletes the seat and commits.
5. The blocked write re-evaluates, matches no row, reports zero rows affected;
   the registration falls through and inserts.

Assert: the outcome is `Inserted`; exactly one seat exists; it names the
registering agent with provenance `manual`; and its identifier **differs** from
the seat that was removed. An implementation that reported a claim on a zero-row
write would leave the slate empty and the outcome `Claimed`, failing on both
counts.

- **No production code changes.** Choosing an external handle over the failpoint
  idiom is the point: the window is entered on the shipped path, not on a path
  that exists only under test.
- The existing fifteen-iteration case is **kept, not replaced**. It exercises
  genuine cross-connection concurrency, which the deterministic case does not;
  the two answer different questions.
- Reuse the multi-connection Postgres pool builder already defined in that
  package. Do not copy it — a second copy would invite the two to drift.
- The case must skip **visibly** when the server dialect is unavailable, using
  the same gate the existing Postgres cases use, and must never report success
  without having run (`AC-OFFICE-SEAT-ASSURANCE-002.11`).

## Fallback

If the wait probe proves unreliable in practice — a server build where the wait
is not visible, or a probe that flakes — fall back to the failpoint idiom this
repository already establishes for destructive cutover migrations: a hook
between the claim search and the conditional write, nil in production, that the
test uses to perform the removal. **Take this only if the probe fails in
practice**, and record why in the case's comment. Do not take it first.

A flaky case is a defect in the case, not a cost of doing business. The
mechanism is chosen to be deterministic precisely so flakiness is a signal.

## Verification

```bash
cd apps/backend && go test ./internal/office/repository/sqlite/... -run 'Postgres.*Claim|ClaimWindow' -count=5
cd apps/backend && go test ./internal/office/...
make -C apps/backend lint
```

`-count=5` is the flakiness check: a deterministic window passes five for five.
Report whether the server dialect was available on this runner; if it was not,
say so plainly rather than reporting the case as covered.

## Files likely touched

- `apps/backend/internal/office/repository/sqlite/` — new deterministic-window test file
- this task file

## Inputs

- `docs/specs/office/requirements/participant-seat-provenance.md` — `-004.8`
- `docs/specs/office/system-design/participant-seat-provenance-assurance-01.md`
  ("The claim target must vanish mid-write, on purpose")
- `apps/backend/internal/office/repository/sqlite/participants.go` —
  `attemptClaim`, `findClaimableAutoSeat`, `claimAutoSeat`
- `/tdd`

## Output contract

Return a compact handoff capsule with intent/acceptance, base/head SHA, changed
files and entry points, risk tags, exact verification commands and results
(including the `-count=5` run and dialect availability), whether the fallback was
taken and why, uncertainties, and this task status set to `done`. Do not edit
`plan.md`.
