---
created: 2026-09-11
status: done
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-DISCOVERY-001
  - REQ-INTEGRATIONS-GITHUB-PR-DISCOVERY-HEALTH-001
system_design:
  - ../../specs/integrations/system-design/github-pr-discovery-health.md
legacy_specs: []
---

# Implementation plan: GitHub PR discovery health

## Overview

Restore valid discovery first, then add truthful failure reporting and bounded
retry admission. Execute two work orders sequentially. Both work orders are
implemented and verified.

## Evidence and requirement conformance

Task `b88d95ee-d11d-4024-aa62-a4f818b750be` reported PR #3601. A read-only REST
lookup confirmed an open PR on `feature/default-agent-handof-775`, with head
`632da8de5773569a2ce34d30d28aaccc93b1ceff`, created at 10:31:03 UTC on September 11.
Backend build `407ed4f1a5` logged repeated failures for that exact branch:

```text
Field 'cloneUrl' doesn't exist on type 'Repository'
```

The failure continued through 12:33 Lisbon time, after PR creation. From 12:34,
the same watches reported rate-limit errors. On-demand sync at 12:41 logged
the task ID and `pr_number: 0`. `graphql.go:439` contains the invalid field;
`poller.go:detectPRForWatch` returns before association persistence on error.

The quota endpoint separately reported 5,000/5,000 while GraphQL failed.
`recordRateLimitResources` overwrites tracker snapshots. Opening the existing
quota disclosure calls `refresh`, allowing the quota response to clear the
exhausted indication. This state replacement is source-confirmed. The user's
brief red text was not captured; its exact content and cause remain unconfirmed.
The evidence does not prove which caller exhausted quota or why the quota
endpoint disagreed. Do not present those unknowns as established causes.

The backend diagnostic ZIP was partial because of its byte limit. Task-detail
MCP access was denied and not bypassed. Evidence is sufficient for the invalid
query and contradictory-state regressions without live mutations.

Existing frontend sync-coordination requirements explicitly exclude backend
rate policy and persistence. Authentication requirements own identity, not
successful discovery. The new integration-owned capability fills that gap;
it does not rewrite either adjacent contract. No new ADR is needed: the scoped
design records the failure-state choice and rationale without a new system
ownership or persistence boundary.

## Scope

### In scope

- Correct generated GraphQL fields and head identity mapping.
- Prove watch discovery persists and publishes the expected association.
- Separate reported quota from scoped discovery failure and recovery.
- Coalesce attempts and honor retry deadlines in background and on-demand paths.
- Project failure state through status and events into existing desktop/phone UI.
- Localized copy, focused E2E, and public integration troubleshooting guidance.

### Out of scope

- Live PR linking, credential changes, deployment, and automatic task transitions.
- PR parsing from chat, new providers, global GitHub client redesign, and SQL migrations.
- Changes to fork matching, attribution, unlink tombstones, or frontend consumer leases.
- Broad review, QA, or unrelated polling cleanup.

## Technical approach

Task 01 corrects `prFieldsBlock`, GraphQL decoding, and clone URL mapping, with
query-contract and real service-path regression tests. Task 02 adds service-owned
discovery health and admission shared by the poller and on-demand synchronization.
It extends workspace status and scoped events, keeps quota observation independent,
and updates the existing GitHub settings summary and limits disclosure. The
completed remediation also shares duplicate target attempts, carries immutable
completion tokens through fork rebinding, reconciles health consumers with watch
lifecycle changes, orders projections by runtime epoch, and preserves provider
retry deadlines.

The [design](../../specs/integrations/system-design/github-pr-discovery-health.md)
defines keys, generation/revision ordering, retry timing, mixed-target aggregation,
retention, and recovery rules. Existing companion packages
`github-task-pr-sync-coordination` and `github-pr-sync-context-regression` retain
their delivered scope; implementation must not reset their historical results.

## ASCII UI preview

UI-01: Workspace > Integrations > GitHub, failed discovery with full reported quota.
Before: the quota response can replace an exhausted indication with full counters.
After: a persistent inline warning and separate quota details remain visible.

```text
Desktop settings
  carlosflorencio [GitHub CLI] [limits]     [refresh]
  ! PR discovery failed: rate limited. Last failure: 12:34

  Limits disclosure (hover or keyboard focus)
  +----------------------------------------------------+
  | PR discovery paused. Retry after 12:35.              |
  | Reported GraphQL quota: 5,000 / 5,000                |
  | Reported API quota:     5,000 / 5,000                |
  | Reported Search quota:    30 / 30                   |
  +----------------------------------------------------+

Phone settings
  carlosflorencio [GitHub CLI]
  ! PR discovery failed: rate limited.
    Last failure: 12:34
  [limits] [refresh]

  Tap limits -> bottom drawer
  +----------------------------------+
  | GitHub API limits                |
  | PR discovery paused.             |
  | Retry after 12:35.               |
  | Reported GraphQL: 5,000 / 5,000   |
  | Reported API:     5,000 / 5,000   |
  | Reported Search:     30 / 30     |
  +----------------------------------+
```

Required structure: operation failure above numerical quota; persistent inline
warning; shared state across viewports; touch drawer and focus return. Exact copy,
spacing, and times are illustrative and must be localized. Pending refresh keeps
the warning and counters visible. Recovery removes the warning only after newer
success. Missing health shows unknown; invalid query uses its own category and
does not claim quota exhaustion. Long phone content has one internal scroll
region, safe-area clearance, no horizontal overflow, and 44px touch controls.
UI-01 covers health ACs .1, .2, .3, .5, and .6.

## Tests

- Discovery ACs .1-.3: new `graphql_pr_discovery_contract_test.go`,
  `TestPRDiscoveryQueryContract`, `TestPRDiscoveryHeadIdentity`, and
  `TestPRDiscoveryAssociationRecovery`. Validate generated fields, nullable/fork
  identity, persistence, event identity, and error-versus-empty outcomes.
- Health ACs .1-.7: new `service_pr_discovery_health_test.go`,
  `TestPRDiscoveryHealth`, covering full quota after failure, duplicate and
  mixed targets, fork rebinding, watch lifecycle pruning, monotonic recovery,
  credential replacement, HTTP-200 GraphQL errors, runtime epochs, and exact
  transport counts before/after provider deadlines.
- Health projection: extend `github-slice.test.ts` and `github.test.ts` for
  workspace/generation isolation and stale response rejection. Add
  `github-rate-limit.test.tsx` for retained warning with refreshed counters.

## E2E tests

Extend `github-workspace-settings.spec.ts` (chromium) and
`mobile-github-workspace-settings.spec.ts` (mobile-chrome) with a failed discovery,
full quota refresh, and explicit newer recovery fixture sequence. Assert the
warning before and after refresh, actual recovery, drawer semantics, wrapping,
and touch reachability. Backend integration evidence supplies the real provider
query-to-association proof; browser fixtures must not be described as real GitHub
schema validation.

## Work orders

- [x] [Task 01: Restore valid PR discovery](task-01-valid-pr-discovery.md) (done)
- [x] [Task 02: Preserve discovery failure evidence](task-02-discovery-health.md) (done)

## Verification results

Implementation checks passed:

- `go test ./internal/github -count=1`: 1,769 passed.
- `go test -race ./internal/github -count=1`: 1,769 passed.
- Backend `make lint` and frontend `pnpm run lint`: passed.
- Frontend typecheck, focused Vitest suite (33 tests), i18n check, and i18n
  ratchet: passed.
- Backend and Vite production builds: passed.
- Chromium GitHub settings E2E: 5 passed. Mobile Chrome GitHub settings E2E:
  3 passed.
- Public-doc validators, specification lint, and `git diff --check`: passed.

## Risks

- GraphQL schema and GH CLI JSON differ; accepting arbitrary test queries hides regressions.
- Quota reports may conflict with actual failures; no reset time is inferred from this incident.
- A successful unrelated target must not erase another target's failure.
- HTTP-200 GraphQL errors and partial batches require explicit classification.
- Retry timing changes must preserve user refresh semantics without bypassing admission.
- Existing integrations index is migrating; retain adjacent sources and historical plan results.
