---
status: draft
system: integrations
created: 2026-09-11
owners:
  - kandev
---

# GitHub PR discovery health requirements

## Overview

Users need task PR discovery to work and its failures to remain visible.
Reported quota is not proof that a PR request succeeded.
Integrations owns this contract because it owns GitHub discovery, credentials,
and provider status. Task and UI systems consume its results.

## Requirements

### REQ-INTEGRATIONS-GITHUB-PR-DISCOVERY-001: Discover task pull requests

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-001.1:** When exactly one eligible open
  PR matches a watched branch, a successful discovery shall associate that PR
  with the owning task and expose it to task consumers.
- **AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-001.2:** Discovery shall preserve the
  target repository and head repository identities for same-repository and
  fork PRs. Missing head repository data shall not invent an identity.
- **AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-001.3:** A rejected provider query shall
  not be treated as a successful search with no PR. Existing associations and
  explicit unlink decisions shall remain intact.
- **AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-001.4:** For every unchanged attached
  repository and branch, repeated watch reconciliation shall preserve its
  discovery target independently of sibling branches and task-group ownership.
  A confirmed rename may update that target; missing or ambiguous branch
  information shall not redirect it to a sibling target.

### REQ-INTEGRATIONS-GITHUB-PR-DISCOVERY-HEALTH-001: Explain discovery failures

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-HEALTH-001.1:** When PR discovery fails,
  workspace integration settings shall show the failed operation, failure
  category, and last failure time independently of reported quota.
- **AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-HEALTH-001.2:** Refreshing quota, opening
  its disclosure, or successfully checking authentication shall not clear a
  PR discovery failure. Quota values may update while the failure stays visible.
- **AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-HEALTH-001.3:** A later successful
  discovery of the affected repository and branch shall clear that failure.
  Success that started before the failure shall not clear it. If at least one
  discovery remains failed, settings shall retain a degraded indication.
- **AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-HEALTH-001.4:** Rate-limited discovery
  shall wait until its retry deadline before another automatic or manual
  attempt. Quota refresh shall not bypass this pause. A retry becoming due
  shall not itself report recovery.
- **AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-HEALTH-001.5:** Failures shall remain
  isolated by workspace connection and credential generation. Switching
  workspace or replacing credentials shall not expose stale failures from
  the previous context.
- **AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-HEALTH-001.6:** Desktop and phone users
  shall see the same failure and quota information. The phone details shall
  use a touch-accessible drawer, readable wrapping, and no page overflow.
- **AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-HEALTH-001.7:** An invalid query shall
  be identified separately from a quota failure. It shall not trigger an
  immediate fallback using the same invalid query or an automatic retry more
  often than once per minute for that discovery target.

### REQ-INTEGRATIONS-GITHUB-PR-POLLING-001: Adaptive background discovery

- **AC-INTEGRATIONS-GITHUB-PR-POLLING-001.1:** Searching watches shall use a 1-minute interval while their task runs or has less than 2 hours of inactivity. They shall use 15 minutes from 2 hours to less than 24 hours, then 30 minutes.
- **AC-INTEGRATIONS-GITHUB-PR-POLLING-001.2:** Task execution, task conversation activity, or an observed branch commit or push shall restore fast discovery. A new watch shall receive an initial attempt on the next poll tick.
- **AC-INTEGRATIONS-GITHUB-PR-POLLING-001.3:** Idle discovery shall not stop permanently. A PR opened externally on an unchanged branch shall remain discoverable. Known PRs shall retain their 1-minute background cadence.
- **AC-INTEGRATIONS-GITHUB-PR-POLLING-001.4:** Explicit PR refresh shall bypass the idle schedule but obey existing rate-limit and authentication admission. Passive frontend refreshes shall obey the idle schedule for searching targets.
- **AC-INTEGRATIONS-GITHUB-PR-POLLING-001.5:** Poll attempts and provider status writes shall not count as task activity. Shared targets shall use the fastest eligible member's schedule. Archived and deleted members shall not accelerate it.

### REQ-INTEGRATIONS-GITHUB-PR-POLLING-002: Bounded batch fallback

- **AC-INTEGRATIONS-GITHUB-PR-POLLING-002.1:** Rate-limit, authentication, and invalid-query batch failures shall not trigger per-watch fallback for the affected workspace. Successful workspace batches shall not be repeated because another workspace failed.
- **AC-INTEGRATIONS-GITHUB-PR-POLLING-002.2:** Transient failures shall permit at most 5 fallback target checks per workspace and 10 across one poll cycle. Deferred targets shall retain their state and receive a fair opportunity on later cycles.
- **AC-INTEGRATIONS-GITHUB-PR-POLLING-002.3:** A rate-limit or authentication error during fallback shall stop remaining checks for that workspace. Cancellation shall stop the cycle. Existing retry deadlines remain authoritative.
- **AC-INTEGRATIONS-GITHUB-PR-POLLING-002.4:** Clients without GraphQL support shall retain REST support under the same bounded schedule. Deferral shall not imply a successful provider observation or discard pending status events.

## Exclusions

No new credentials, automatic PR creation, inferred PR links from chat, changed
fork eligibility, unlink behavior, or task/workflow transitions. No promise
that the quota endpoint explains all provider throttling. Failure history is
runtime status, not a permanent incident archive.

## Related contracts

- [Workspace authentication](github-authentication.md)
- [Frontend synchronization coordination](github-task-pr-sync-coordination.md)
- [System design](../system-design/github-pr-discovery-health.md)
- [Implementation package](../../../plans/github-pr-discovery-health/plan.md)
- [Watch reconciliation repair](../../../plans/github-pr-watch-reconciliation/plan.md)

## Polling efficiency package

- [Implementation plan](../../../plans/watch-task-cleanup/plan.md)
