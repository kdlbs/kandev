---
id: "01-preserve-jira-watcher-descriptions"
title: "Preserve Jira watcher descriptions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001
acceptance_criteria:
  - AC-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001.1
  - AC-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001.2
  - AC-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001.3
  - AC-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001.4
system_design:
  - ../../specs/integrations/system-design/jira-watcher-task-prompts.md
---

# Task 01: Preserve Jira Watcher Descriptions

## Summary

Add a watcher-specific Jira search contract that requests descriptions in the batched REST response. Keep compact searches unchanged and preserve descriptions through prompt interpolation.

## In scope

- Add and implement `Client.SearchTicketsForWatch` for REST, MCP, mock, and test clients.
- Include `description` only in REST watcher field sets.
- Route `Service.CheckIssueWatch` through the watcher method.
- Add regression tests at the REST client, service, and interpolation boundaries.

## Out of scope

- Frontend production changes.
- Per-issue hydration through `GetTicket`.
- Changes to the current description text converter.
- Changes to watcher persistence, polling frequency, or dispatch order.

## Acceptance

- The new regression test fails before the correction because the watcher REST request omits `description`.
- Jira Cloud and Server or Data Center watcher searches return readable descriptions in one search request.
- Compact Jira searches keep their current field set, and watcher task prompts contain the resolved description.

## Verification

```bash
cd apps/backend
go test ./internal/jira ./internal/orchestrator -count=1
cd ../..
python3 scripts/lint-spec-files.py --all
git diff --check -- apps/backend docs/specs docs/plans
```

## Files likely touched

- `apps/backend/internal/jira/client.go`
- `apps/backend/internal/jira/cloud_client.go`
- `apps/backend/internal/jira/cloud_client_test.go`
- `apps/backend/internal/jira/mcp_client.go`
- `apps/backend/internal/jira/mock_client.go`
- `apps/backend/internal/jira/service_watch.go`
- `apps/backend/internal/jira/service_issue_watch_test.go`
- `apps/backend/internal/jira/service_test.go`
- `apps/backend/internal/orchestrator/event_handlers_jira_test.go`
- `docs/plans/fix-jira-watcher-description/plan.md`
- `docs/plans/fix-jira-watcher-description/task-01-preserve-jira-watcher-descriptions.md`

## Dependencies

None.

## Risks

- A missing implementation of the new interface method causes a compile error.
- Shared search helpers can accidentally add descriptions to interactive search payloads.

## Parallelism

`sequential`

## Inputs

- `REQ-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001`
- `docs/specs/integrations/system-design/jira-watcher-task-prompts.md`
- GitHub issue #3130 and its two screenshots.
- Atlassian Jira Cloud and Jira Data Center search field contracts.

## Results

Implemented `Client.SearchTicketsForWatch` for REST, MCP, mock, and test
clients. Jira REST watcher searches request `description` for Cloud and Server
or Data Center while compact searches retain their existing field list.

Verification passed:

- RED: `TestService_CheckIssueWatch_UsesWatcherSearch` failed with an empty
  description before the production change.
- `cd apps/backend && go test ./internal/jira ./internal/orchestrator -count=1`
  (2,383 tests passed)
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check -- apps/backend docs/specs docs/plans`
