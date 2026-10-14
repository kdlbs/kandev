# Portable adoption review handoff

Base: 00b5d599968a713a7c24318a17128a6b4b1dc431.
MAIN owns source, fixes and routing. Reviewer session
34c401ff-c5c1-4345-90bc-156eac6b5e7c and QA session
16e162e7-dce3-4341-8968-fc7368b03559 are independent same-task sessions.
Final candidate and verdicts are recorded in task-plan metadata.

## Fixed during review

- R1: Git replacement refs fabricated source ancestry. Adoption now uses raw
  Git object identity/history and rejects legacy graft files. The grafted-orphan
  HTTP regression fails before the fix and refuses after it.
- R2: Escaped branch ref files/directories passed preview. Adoption now checks
  ref/object storage with bounded no-follow traversal before and after Git reads.
  Preview, apply, receipt/replay and reuse regressions refuse escaped metadata.
  Redirected object storage refuses; legitimate packed refs still pass.

Ordinary healthy-checkout admission remains unchanged. The independently reproduced
failures were fixed within the existing no-foreign/no-escaped metadata contract.

## QA fixes worth reviewer attention

Before independent review, MAIN reproduced and fixed stale adopted-metadata
rematerialization, forged-origin unrelated history, orphan-claim replay finalization
and borrowed-claim persisted-consumer checks. Regression coverage includes SQLite
and PostgreSQL transactions, rollback, schema replay, CAS and claim contention.

## Follow-up tasks created

None. Existing terminal retention and routing work was reused without duplication.

## Action required by author

None for this source slice. Deployment guard/helper migration, release runtime
acceptance and overlay retirement remain separate and unclaimed.

## Assumptions made during QA

Source-level synthetic acceptance uses existing Git/SQLite test factories and a
task-owned PostgreSQL 18.6 dependency. No production data, real agent launch,
actual backend kill/restart or live deployment was exercised. No UI changed.

## Pre-existing issues found

None required to complete this slice. Ordinary admission #4057 is superseded.

## UI screenshots for PR/MR

Not applicable: operator HTTP API only. No PR was created in this phase.

## PR repair verification

The Git repository-selection finding is fixed by sanitizing inherited Git
variables and specifying the inspected metadata directory on every raw proof
read. A real HTTP regression failed before the fix and passes for inherited
GIT_DIR and GIT_OBJECT_DIRECTORY at preview/apply/reuse. Operator rejection
coverage now includes all three routes. The adoption table is included in the
task store readiness descriptor.

The trusted PR documentation evaluator reproduced invalid work-order metadata.
Work orders and the manifest now link requirement IDs, acceptance criteria and
one task-owned system design. The evaluator reports covered with no errors.
Public prerequisites are a list; the static decision index no longer includes a
manual catalog entry. CodeRabbit's duplicate-traversal suggestion is declined:
the two passes bracket Git subprocess reads and detect intervening mutations.

Stale-receipt replacement is a valid open lifecycle issue. The original API
accepts a different clone only when the selected original path has a broken Git
pointer; it cannot re-attest the already adopted path, and the receipt is unique
per slot. Weakening validation would violate authority requirements. MAIN sent
explicit re-attestation, automatic refresh and retained-refusal options to the
Coordinator; no new public/schema contract is implemented without that scope
decision. Ordinary reviewer notification remains gated on findings and required
checks. Error-status differentiation remains an optional follow-up requiring
consistent inspector/service error classification; the existing 409 behavior is
not claimed to distinguish infrastructure failures.

Original Human test artifacts and the PostgreSQL fixture remain intact. New
repair candidate SHA, exact tests and independent targeted Review/QA receipts
are recorded in task metadata; the original verification does not establish
acceptance of a new candidate. No deployment or integration occurred.


### Hosted retirement regression

Exact 5b74 native authenticated adoption, resume, write, file API, provenance and
receipt assertions passed through the third backend restart. Its explicit reset
then failed because archive capture classified the owned independent clone as a
project primary. Retirement now retains validated receipts in the durable cleanup
snapshot, checks the running job attempt and pinned owner/path/Git proof, and
preserves the project primary. Real handler/Git/SQLite and environment-gated
PostgreSQL tests cover archive/delete after foreign-key cascade and manager
restart; negative cases cover primary, foreign owner/claim, replaced paths,
symlinks and missing proof. Final hosted verification remains pending.

The separate terminal mobile parent-default failure showed a correct parent
session response arriving after form mount, while one-shot seeding left the
repository and profile empty. Default seeding now accepts delayed parent context
once, preserving edits. The existing mobile repository assertion and all native
reset assertions remain unchanged. Hook tests cover delayed context and edited
values. Final full frontend/mobile verification remains pending.
