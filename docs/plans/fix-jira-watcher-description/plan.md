---
created: 2026-08-28
status: done
requirements:
  - REQ-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001
system_design:
  - ../../specs/integrations/system-design/jira-watcher-task-prompts.md
legacy_specs: []
---

# Implementation Plan: Fix Jira Watcher Description

## Overview

Jira issue #3130 reports an empty `{{issue.description}}` value in watcher-created tasks. The watcher uses a compact search request that omits the description field.

One work order adds a watcher-specific search contract and regression coverage. The correction keeps interactive Jira searches compact and avoids per-issue requests.

## Scope

### In scope

- Request the Jira description in watcher searches for Jira Cloud and Jira Server or Data Center.
- Keep the description through watcher filtering, event publication, prompt interpolation, and task creation.
- Keep the existing compact field list for interactive Jira searches and work-item references.
- Preserve the Atlassian MCP search behavior and the Jira mock behavior.

### Out of scope

- New placeholders or custom Jira fields.
- Jira rich-text style conversion beyond the current readable text conversion.
- Changes to JQL, pagination, deduplication, routing, or automatic start behavior.
- Frontend production changes or new user-facing text.

## Confirmed root cause

`CloudClient.SearchTickets` sends an explicit REST `fields` list. That list omits `description`, so Jira omits the value from each search result.

`Service.CheckIssueWatch` uses that compact search result as the event payload. `interpolateJiraPrompt` receives an empty `JiraTicket.Description` and replaces the placeholder with an empty string.

A temporary regression test used an HTTP test server that honored the requested field list. The test failed with `description = "", want Jira description`.

## Technical approach

### Provider client boundary

- Add `SearchTicketsForWatch` to `internal/jira.Client` in `apps/backend/internal/jira/client.go`.
- Keep `SearchTickets` as the compact browse and work-item reference method.
- Reuse the Cloud and Server or Data Center search paths in `cloud_client.go` with an explicit field set.
- Include `description` only in the watcher field set.
- Delegate the MCP and mock watcher methods to their current search behavior.

### Watcher flow

- Change `Service.CheckIssueWatch` in `service_watch.go` to use `SearchTicketsForWatch`.
- Update test clients that implement `Client`.
- Do not add per-issue `GetTicket` calls. That method also requests transitions and causes request fan-out.

### Regression coverage

- Add a table-driven REST client test in `cloud_client_test.go` for Cloud and Server or Data Center.
- Record the requested REST field list and assert watcher searches request a description while compact searches do not.
- Add a service test in `service_issue_watch_test.go` that distinguishes compact search from watcher search.
- Extend `TestInterpolateJiraPrompt` in `event_handlers_jira_test.go` with the description placeholder.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001.1` | `TestInterpolateJiraPrompt` resolves the supported description placeholder before task creation. |
| `AC-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001.2` | A new Cloud and Server or Data Center client test requests and converts watcher descriptions. |
| `AC-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001.3` | Existing `TestCloudClient_GetTicket_ParsesADFDescription` plus the new watcher search test cover ADF and plain strings. |
| `AC-INTEGRATIONS-JIRA-WATCHER-TASK-PROMPTS-001.4` | `TestInterpolateJiraPrompt` covers an empty description without an unresolved token. |

## E2E tests

The service and real REST client test form the faithful end-to-end provider path for this regression. The in-memory Playwright Jira mock already supplies descriptions.

A new Playwright test passes before the correction because that mock does not enforce Jira REST field selection. This package does not add that false-positive test.

## Work orders

- [x] [Task 01: Preserve Jira watcher descriptions](task-01-preserve-jira-watcher-descriptions.md)

## Verification results

Implemented the watcher-specific search contract and regression coverage.

Verification passed:

- `cd apps/backend && go test ./internal/jira ./internal/orchestrator -count=1`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check -- apps/backend docs/specs docs/plans`

## Risks

- A new `Client` method requires updates to every production and test implementation.
- The Atlassian MCP tool controls its own fields. Its watcher method must preserve the current parser and result contract.
- A shared REST field list can increase payload size for interactive searches. The implementation must keep separate field sets.
