---
status: active
system: integrations
created: 2026-10-06
owners:
  - kandev
---

# GitHub Browse Status Batches Requirements

## Overview

Users browsing GitHub pull requests need review, check and mergeability summaries
that belong to the workspace and result batch currently requested by the list.
Two workspaces or search pages can contain the same pull request; a shared PR
identity alone does not make a previous batch's summaries current.

Integrations owns this provider browse-status contract and its desktop and phone
outcomes. Query selection and mobile presentation remain with the
[GitHub dashboard](github-dashboard-mobile.md). The separate
[PR-to-task association read](github-pr-task-association-reads.md) does not own
these provider summaries.

## Terminology

- **Requested batch:** The requested workspace and ordered collection of
  owner/repository/PR-number identities. Rebuilding objects or arrays without
  changing those values preserves batch identity.
- **Eligible summaries:** A completed result whose batch identity equals the
  current nonempty requested batch.
- **Current acknowledgement:** Successful completion of the uncancelled read
  for the requested batch. This does not establish that GitHub data is live or
  newer than a provider cache.

## Requirements

### REQ-INTEGRATIONS-GITHUB-BROWSE-STATUS-001: Requested-batch status eligibility

**Intent:** Browse rows show provider summaries only from an eligible batch,
independently of when a replacement read completes.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-BROWSE-STATUS-001.1:** When the workspace or ordered
  PR membership changes, the first render for the new batch shall exclude every
  summary from a different batch, including summaries for a PR present in both
  batches. A pending replacement shall not make those summaries eligible.
- **AC-INTEGRATIONS-GITHUB-BROWSE-STATUS-001.2:** When the requested workspace is
  absent or the PR collection is empty, the status read shall expose an empty
  result immediately and shall make no batch request for those inputs.
- **AC-INTEGRATIONS-GITHUB-BROWSE-STATUS-001.3:** Rebuilt equal-content inputs
  shall retain the eligible result collection and its status object identities
  without an additional request or cancellation of a pending equal-content read.
  Changes outside the existing batch identity shall not introduce refreshes.
- **AC-INTEGRATIONS-GITHUB-BROWSE-STATUS-001.4:** A current acknowledgement shall
  make its summaries available; an acknowledged empty collection shall expose
  no summaries. A failed replacement shall not restore an ineligible batch.
  This contract shall preserve existing failure and request scheduling behavior,
  without guaranteeing an immediate new request or automatic retry.
- **AC-INTEGRATIONS-GITHUB-BROWSE-STATUS-001.5:** Responses and failures from
  cancelled reads shall not replace or clear the current result. This includes
  unmount, StrictMode effect cleanup, and A-to-B-to-A transitions where an
  earlier pending A read completes after a later A read.
- **AC-INTEGRATIONS-GITHUB-BROWSE-STATUS-001.6:** Independent browse consumers
  shall retain independent results even for equal PR identities. Returning to
  A while a completed A result remains retained and B is pending shall preserve
  existing equal-batch reuse; this does not require a new navigation generation
  or promise reuse after the retained result has been replaced or cleared.
- **AC-INTEGRATIONS-GITHUB-BROWSE-STATUS-001.7:** Desktop and phone rows shall
  apply the same eligibility. A newly requested row shall omit old status badges
  while preserving its current PR title and existing task action; after a current
  acknowledgement, its current badges shall render through the existing row.

## Out of scope

- Provider cache freshness, HEAD or updated-time refresh, polling and retries.
- New batching, coalescing, request scheduling, backend or persistence behavior.
- Task-linked PR status and reverse task associations.
- Provider permissions or a new authorization guarantee.
- Changes to row layout, labels, navigation, scrolling, touch or breakpoints.
- Other providers and issue results.

## Related documents

- [System design](../system-design/github-browse-status-batches.md).
- [Implementation plan](../../../plans/github-pr-status-batch-scope/plan.md).
