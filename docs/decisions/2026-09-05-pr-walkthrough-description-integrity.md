# ADR-2026-09-05-pr-walkthrough-description-integrity: Keep PR walkthrough description updates canonical and race-safe

**Status:** accepted
**Date:** 2026-09-05
**Area:** workflow, infra, GitHub

## Context

PR walkthrough HTML is published under a 12-character head-SHA URL, but pull
request descriptions are shared mutable documents. The walkthrough workflow,
preview workflow, users, and external integrations can update the same body.
GitHub pull request body updates replace the complete body, so a stale writer
can restore an obsolete full-SHA walkthrough URL or remove another automation's
marker-owned section. The result is a successful workflow with a reviewer link
that returns 404.

## Decision

Use one canonical public URL contract: the first 12 lowercase hexadecimal
characters of the published head SHA. The publication job passes its validated
URL to the walkthrough link job, and current Kandev writers must not construct
full-SHA public URLs.

All Kandev-owned pull request body writers use a bounded optimistic update
protocol: read the live body, merge only the caller's marker-owned section,
read again immediately before PATCH, retry when the body changed, and read back
the result before reporting success. Walkthrough and preview write jobs share a
per-pull-request concurrency group, but that group does not replace the
compare-and-readback checks because users and external integrations can write
outside GitHub Actions.

The walkthrough workflow handles pull request `edited` events in a separate,
non-generating reconciliation path. It repairs an existing walkthrough marker
only after the current canonical object is publicly available. Missing objects,
missing callouts, and malformed markers fail closed without a destructive body
write.

## Consequences

- New walkthrough links cannot point at the retired full-SHA object format.
- A later body edit can self-heal a stale walkthrough callout when its current
  object is available.
- Preview and walkthrough sections preserve each other and contributor content
  across concurrent Kandev writes.
- A write can fail after bounded retries instead of silently overwriting a
  changing description. The job summary and logs expose that failure.
- Existing full-SHA objects are not renamed, and links outside the owned
  walkthrough block are not changed.
- The public host may still add a legacy-path redirect as separate defense in
  depth, but hosting compatibility is not the source-of-truth repair.

## Alternatives Considered

- **Rely only on the existing workflow concurrency group:** It serializes
  walkthrough runs but not preview jobs, users, or external integrations.
- **Use GitHub conditional PATCH requests alone:** GitHub's REST guidance does
  not provide a reliable conditional-write contract for this body mutation.
  The client therefore uses fresh snapshots and post-write verification.
- **Publish both full and short URL objects:** This masks stale writers but
  keeps the obsolete public contract alive and does not protect other body
  sections from full-body races.
- **Rewrite the complete description from a template:** This simplifies one
  writer but violates the existing ownership boundary and can destroy
  contributor or other automation content.
