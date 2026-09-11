---
id: "01-valid-pr-discovery"
title: "Restore valid PR discovery"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-DISCOVERY-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-001.1
  - AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-001.2
  - AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-001.3
system_design:
  - ../../specs/integrations/system-design/github-pr-discovery-health.md
---

# Task 01: Restore valid PR discovery

## Summary

Correct the shared GraphQL PR selection and prove discovery writes the expected
association. Keep same-repository, fork, and missing-head identity behavior explicit.

## In scope

- First add `TestPRDiscoveryQueryContract` in the proposed
  `graphql_pr_discovery_contract_test.go`. Validate the generated Repository
  selection against a checked field contract; run it red on `cloneUrl`.
- Replace the selection with `url`; map the decoded URL to the clone identity.
  Reuse shared conversion where applicable without conflating CLI JSON and GraphQL.
- Add `TestPRDiscoveryHeadIdentity` for normal, fork, null, and missing head data.
- Add `TestPRDiscoveryAssociationRecovery` using the real service and store:
  rejected query creates no association; corrected successful query discovers
  one eligible PR, persists it once, and publishes the scoped task PR event.
- Exercise both branch and known-PR batch builders. Keep definitive empty,
  ambiguity, tombstones, and fork-network call-count coverage green.

## Out of scope

Retry policy, health projection, UI, credential selection, and live task repair.

## Acceptance

1. The contract regression fails on the current invalid selection and passes
   with supported fields; no live GitHub credential is required by tests.
2. Head and target identities remain distinct and correct, including null data.
3. Persistence and publication use existing writers and retain unlink protection.

## Verification

Run from repository root. Record red output before implementation, then green results.

```bash
(cd apps/backend && go test ./internal/github -run 'TestPRDiscovery' -count=1)
(cd apps/backend && go test -race ./internal/github -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/github/graphql.go`
- `apps/backend/internal/github/gh_client.go` (shared repository decoding only)
- `apps/backend/internal/github/graphql_pr_discovery_contract_test.go` (new)
- `apps/backend/internal/github/graphql_test.go`
- `apps/backend/internal/github/service_pr_watch_batched_budget_test.go`

## Dependencies

None.

## Risks

Do not obtain a clone URL by using the PR target when the head is a fork.
Validate the schema fixture against the upstream source linked in the design;
do not merely snapshot today's production query.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/github-pr-discovery-health.md), discovery criteria.
- [Design](../../specs/integrations/system-design/github-pr-discovery-health.md), query validation and association.
- `apps/backend/internal/github/AGENTS.md` and existing GraphQL/service test patterns.

## Results

Pending.
