---
id: "05-catalog-cutover"
title: "Catalog and caller cutover"
status: done
wave: 5
depends_on: ['04-bound-outcome']
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-002
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-003
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-004
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-005
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-006
acceptance_criteria:
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.1
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.2
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.3
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.4
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.5
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-001.6
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-002.1
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-002.2
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-002.3
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-002.4
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-003.1
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-003.2
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-003.3
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-004.1
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-004.2
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-004.3
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-004.4
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-004.5
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-005.1
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-005.2
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-005.3
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-006.1
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-006.2
system_design:
  - ../../specs/integrations/system-design/task-change-link-mcp.md
---

# Task 05: Catalog and caller cutover

## Summary

Remove superseded discovery names, migrate callers, and prove the serialized MCP workflow.
Document upgrade behavior and the internal transport retirement boundary.

## In scope

- Remove all eight legacy registrations; update catalog counts and dynamic profile tests.
- Migrate built-in workflow and immutable outcome protocol references with runtime-catalog-aware delivery.
- Use current-execution `SessionMetaKeyMCPAttachmentState` through `LoadMCPAttachmentHistory` to choose old versus neutral outcome name for old/new processes; do not infer binary version from provider presence.
- For missing current-execution evidence, leave dispatch retryable without a new round or delivered-attempt binding. Preserve existing error visibility and retry scheduling.
- Cover queued trusted blocks, unchanged historical transcripts, edited prompts, and current-runtime resume.
- Keep original scoped backend WS actions for one transition release, with principal enforcement.
- Add protocol fixtures described in plan.md for both MCP eras and all agent examples.
- Update public reference pages, coverage metadata, and superseded discovery criteria at cutover.
- Reconcile the ADR and system design against implementation; preserve historical plan results.

## Out of scope

Provider-store redesign, new UI, cross-provider replacement, and GitLab outcome tracking.
Do not change unrelated provider algorithms or historical transcripts.

## Acceptance

- Both protocol eras expose the same new names; all old calls to a new runtime fail without mutation.
- Serialized workflows prove both providers, mixed targeting, authorization, replacement failures, and exact bound reporting.
- Caller inventory has no active old-name use except tested old-runtime transport compatibility; upgrade and retirement instructions are explicit.

## Verification

Run from the repository root. Add failing tests first, then implement and run these checks.
Use existing package fixtures; keep all new tests inside this work order's listed suites.

```bash
(cd apps/backend && go test ./internal/mcp/server ./internal/mcp/handlers ./internal/backendapp ./config/workflows -count=1)
(cd apps/backend && go test ./internal/orchestrator ./internal/github ./internal/gitlab -run 'AutoFix|CIAuto|Outcome|TaskMR|TaskPRDetach|DetachTaskPR' -count=1)
(cd apps/backend && go test -race ./internal/mcp/server -run 'ChangeRequestProtocol|ChangeRequestCatalog|SetProviders' -count=1)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/server_test.go`
- `apps/backend/internal/mcp/server/handlers_test.go`
- `apps/backend/internal/mcp/server/task_change_request_catalog_test.go (new)`
- `apps/backend/internal/mcp/server/task_change_request_protocol_test.go (new)`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation.go`
- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation_test.go`
- `apps/backend/config/workflows/pr-review.yml`
- `apps/backend/config/workflows/loader_test.go`
- `docs/public/automation-and-mcp.md`
- `docs/public/integrations.md`
- `docs/public/coverage.json`
- `docs/specs/integrations/requirements/provider-aware-review-automation.md`
- `docs/specs/integrations/requirements/task-change-link-mcp.md`
- `docs/specs/integrations/system-design/task-change-link-mcp.md`
- `docs/decisions/2026-09-14-provider-neutral-change-request-mcp.md`

## Dependencies

Task 04. Use its DTOs and validated backend boundary.

## Risks

Old binaries cannot learn new handlers through catalog refresh. The SDK cannot provide callable hidden aliases with ToolFilter.

## Parallelism

`sequential`. Shared schemas and registration files prevent parallel ownership.

## Inputs

- [Requirements](../../specs/integrations/requirements/task-change-link-mcp.md), frontmatter IDs.
- [System design](../../specs/integrations/system-design/task-change-link-mcp.md), relevant contract sections.
- [Plan](plan.md), baseline, test matrix, and agent requests.
- Existing adjacent tests named in the plan and `apps/backend/AGENTS.md`.
- `.agents/skills/tdd/SKILL.md` and its backend testing reference before implementation.

## Results

Removed the eight legacy MCP registrations, migrated workflow and public
references, added catalog-aware outcome protocol delivery, and retained the
legacy backend WS actions for the one-release transition. Both MCP protocol
eras support the neutral catalog, while superseded calls fail without reaching
the backend.

Validation passed:

- `go test ./internal/mcp/server ./internal/mcp/handlers ./internal/backendapp ./config/workflows -count=1`
- `go test ./internal/orchestrator ./internal/github ./internal/gitlab -run 'AutoFix|CIAuto|Outcome|TaskMR|TaskPRDetach|DetachTaskPR' -count=1`
- `go test -race ./internal/mcp/server -run 'ChangeRequestProtocol|ChangeRequestCatalog|SetProviders' -count=1`
- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.test.py`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`
- `go test ./internal/orchestrator -run 'TestReplaceCIAutoFixOutcomeProtocol' -count=1`
  passed for wrapped and passthrough prompts using the literal pre-cutover
  protocol.
- `go test ./internal/backendapp -run 'TestTaskChangeCoordinator(Unlinks|Replaces)ReadResolvedLegacyGitLabAssociation' -count=1`
  passed with canonical identity resolution and stale-association cleanup.
- A production-shaped CI automation regression clears the session catalog and
  verifies that the missing-catalog error is recorded before dispatch, with no
  fix attempt or merge operation.
