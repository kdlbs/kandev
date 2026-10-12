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
