---
id: "01-agent-registration"
title: "Observation-only agent registration"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-002
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-003
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-004
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-005
acceptance_criteria:
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-002.1
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-002.2
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-002.3
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-002.4
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-003.1
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-003.2
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-003.3
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-003.4
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-004.1
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-004.2
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-004.3
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-004.4
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-005.1
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-005.2
  - AC-INTEGRATIONS-CHANGE-REGISTRATION-005.3
system_design:
  - ../../specs/integrations/system-design/task-change-registration.md
---

# Task 01: Observation-only agent registration
## Summary

Deliver agent URL registration through the existing neutral tool for GitHub and GitLab.
Persist exact identity, origin, observation policy, and unlink revisions before exposing the capability to injected guidance.

## In scope

- Extend URL resolution, current-task default, schema/backend validation, result DTOs, and authenticated target checks.
- Add provider-row metadata and GitLab soft detach; preserve exact automation/watch cleanup and sibling associations.
- Persist the exact canonical key with immutable target repository ID and connection scope. Cover rename, path reuse, and credential rotation.
- Migrate verified local identity only. Keep unresolved legacy rows removable and prevent automatic writes until authorized identity repair.
- Add the narrow registration-intent table, transaction-aware provider commit seam, and controlled retry/read recovery.
- Guard comparison and execution effects through later sync, discovery, orphan observation, and event consumers.
- Add shared agent guidance for creation, active review/fix/update, stacks, final checks, background-only references, and accurate failure reports.
- Amend existing MCP required-field criteria and stale tool examples. Link the observation exception from the comparison contract; preserve historic plan evidence.

## Out of scope

Composer controls, message/queue admission producers, Azure management support, new automation policies, repository attachment, and production work before authorization.

## Acceptance

1. URL and legacy tuple operations pass identity/authority/compatibility tests, return known committed state, and retain replacement compensation guarantees. Rename preserves identity; path reuse never inherits links or tombstones.
2. New observation links preserve comparison/checkout and disabled execution switches across sync; retries cannot defeat unlink or remove siblings.
3. Effective shared guidance registers relevant work with the existing tools and reports failure/suppression without claiming success.

## Verification

From repository root. Configure an isolated test PostgreSQL DSN through the normal test environment; never use the live Kandev database.
The nonempty-variable check prevents PostgreSQL tests from silently skipping.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/mcp/handlers ./internal/mcp/server ./internal/backendapp ./internal/github ./internal/gitlab ./internal/sysprompt -run 'Registration|TaskChange|AssociateExisting|Unlink|Detach|ComparisonTarget|PostgresStoreSchemaReplay' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator ./internal/task/service -run 'Registration|ComparisonTarget|TaskPRCIAutomation|TaskMRLifecycle' -count=1)
(cd apps/backend && test -n "$KANDEV_TEST_POSTGRES_DSN" && go test -trimpath -race -tags fts5 ./internal/github ./internal/gitlab ./internal/task/repository/sqlite -run 'Postgres.*Registration|Registration.*Postgres|PostgresStoreSchemaReplay' -count=1 -v)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/github ./internal/gitlab ./internal/task/repository/sqlite -run Registration -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Add the matrix's proposed provider/store/coordinator/guidance tests in this work order. PostgreSQL registration cases must run, not skip.
Include `TestRegistrationImmutableIdentity`, `TestRegistrationLegacyIdentityRepair`, and concurrent compare-and-set tests in both database modes.
Use barriers to control identity-read, unlink, and commit order. Assert stored keys and preserved sibling rows, not only returned status.
A controlled fake-agent trace must call link for every created/worked stack URL, then list; it must leave background-only URLs alone.
No browser behavior changes in this work order; existing PR lists consume the same provider events.

## Files likely touched

- `apps/backend/internal/mcp/server/server.go`, `handlers.go`, and argument-validation tests.
- `apps/backend/internal/mcp/handlers/task_change_link.go`, `task_change_request.go`, and tests.
- `apps/backend/internal/backendapp/task_change_link_coordinator.go`, related identity/read adapters, service wiring, and tests.
- `apps/backend/internal/github/models.go`, `store.go`, `service_pr_watch.go`, comparison, detach, sync, and store tests.
- GitHub/GitLab provider response mappings for immutable target repository IDs and the existing authorized repository identity lookup.
- `apps/backend/internal/gitlab/models.go`, `store.go`, `service_task_mr_link.go`, comparison, watch, and store tests.
- `apps/backend/internal/orchestrator/event_handlers_github.go`, `event_handlers_gitlab.go`, automation evaluators, and tests.
- `apps/backend/internal/task/repository/sqlite/` for new registration-intent persistence and migrations.
- `apps/backend/internal/sysprompt/sysprompt.go`, `config/prompts/` under the backend, and guidance tests.
- Existing task-change MCP/comparison specifications and public MCP examples.

## Dependencies

None. The registration consumer and additive schema are necessary before admission producers.

## Risks

Legacy host ambiguity, SQLite/PostgreSQL lock ordering, GitLab query filters, and hidden comparison or automation consumers.
Do not expose new URL guidance until the observation and tombstone tests pass.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/task-change-registration.md), sections 002 through 005.
- [Design](../../specs/integrations/system-design/task-change-registration.md), identity, agent tool, persistence, unlink and downstream sections.
- [Investigation](investigation.md) and [validation matrix](validation-matrix.md).
- Existing `task_change_link_test.go`, coordinator identity tests, provider store tests, and `sysprompt_test.go`.

## Results

Task 01 is complete. The following checks passed:

- Focused MCP, backend coordinator, GitHub, GitLab, and sysprompt tests.
- Focused orchestrator and task-service comparison/automation tests.
- PostgreSQL 16 registration and schema replay tests with `-race`, including provider fences, accepted-message intent actions, admission fences, and repository intent persistence. The test DSN pointed to a disposable local database; each test used an isolated schema.
- Race-enabled registration tests for GitHub, GitLab, and the SQLite task repository.
- Public documentation validators, specification catalog validation, specification lint, and `git diff --check`.

The repository-action fixture now starts from the intent created by accepted message persistence, then simulates its committed unlink state. This keeps the SQLite and PostgreSQL cases aligned with the production admission path.
