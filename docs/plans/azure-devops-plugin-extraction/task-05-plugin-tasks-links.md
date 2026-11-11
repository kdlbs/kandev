---
id: "05-plugin-tasks-links"
title: "Tasks, links, PR status, Create PR, agent tool"
status: in_progress
wave: 3
depends_on: 
  - "02-plugin-connection"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-005
acceptance_criteria:
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-005.1
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-005.2
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-005.3
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-005.4
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-005.5
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-005.6
system_design:
  - ../../specs/integrations/system-design/azure-devops-plugin.md
---

# Task 05: Tasks, links, PR status, Create PR, agent tool

## Summary

Launch tasks from work items and pull requests, add the Link entries, store links in plugin state, report PR status through the review provider, create PRs from the native dialog and from the `create_pull_request` agent tool, and provide `#` references.

## In scope

- `TaskCreateDialog` launch with linking; links in `task.<taskId>.pullRequests` and `task.<taskId>.workItems`; `task.deleted` reconciliation.
- `registerTaskAction` Link entries with `openTaskLinkDialog`.
- `registerReviewProvider` snapshots and review detail.
- `createChangeRequest`; agent tool; `EntityReferenceHandler` (port of `mentions/provider_azure.go`).

## Out of scope

- Watches (task 06).

## Acceptance

- A linked PR shows state, checks, and reviews in the task indicators and refreshes.
- An agent creates and links a PR through the tool without `az`.

## Verification

Commands run from the Kandev repository root; the plugin worktree is its sibling.

```bash
(cd ../JnManso-kandev-plugin-azure-devops-main && go test -race ./... -count=1)
(cd ../JnManso-kandev-plugin-azure-devops-main && npm test && npm run e2e)
```

## Files likely touched

- `../JnManso-kandev-plugin-azure-devops-main/internal/plugin/{links,review,agent_tools,references}.go`
- `../JnManso-kandev-plugin-azure-devops-main/ui/src/tasks/`

## Dependencies

Task 02-plugin-connection

## Risks

- None

## Parallelism

`parallel-safe`

## Inputs

- [System design](../../specs/integrations/system-design/azure-devops-plugin.md)
- `apps/backend/AGENTS.md`, `apps/web/AGENTS.md`, `docs/public/plugins-authoring.md`, and the `/create-kandev-plugin`, `/e2e`, and `/mobile-parity` skills where relevant

## Results

- 2026-11-06: implemented on plugin branch `feat/bootstrap` in commits `498e5cb` (feature) and `769fc26` (fixes from the host smoke test).
  - Links: workspace state keys `task.<taskId>.pullRequests` and `task.<taskId>.workItems`, at most 50 links of each kind per task. `task.deleted` removes them, and a startup pass drops links of tasks the host reports as not found.
  - Actions:
    - Workspace scope: `task_links.workspace`, which also returns linked task titles for the work item detail.
    - Task scope: `task_links.get` (links with live pull request status), `pull_requests.link`, `pull_requests.unlink`, `work_items.link`, `work_items.unlink`, and `pull_requests.create`.
    - References accept a URL of the connected organization (`dev.azure.com` or `<org>.visualstudio.com`), `Project/Repository!42`, `!42` (searched in the task's Azure repositories), `Project#42`, and `#42`.
  - Review provider: the server maps policies, reviewer votes, and active threads to the host `ReviewTaskStatus`. The review panel renders the host `ChangeRequestDetail` from `pull_requests.feedback`. Link entries **Azure DevOps Pull Request** and **Azure DevOps Work Item** open `openTaskLinkDialog`, with errors localized from the action status.
  - Create PR: `createChangeRequest` (`supportsDraft: true`) calls `pull_requests.create` with the host-verified session branch. It reuses an active pull request for the same branch pair, attaches the task's linked work items, and links the result.
  - Task launch: the browse page's Task menu and quick actions open the host `TaskCreateDialog`. The repository is preset to the workspace repository resolved by `resolveRepositoryId`, otherwise to the Azure Repos URL. The created task is linked to the work item or pull request.
  - References: `reference_sources` `azure_work_items` and `azure_pull_requests` (provider `azure_devops`). Keys are `Project#42` and `Project/Repository!42`; authorization re-reads the item.
  - Agent tool `create_pull_request` (`kanban-task`): arguments `title`, `description`, `sourceBranch`, `targetBranch`, `repository`, and `draft`.
  - Deviation: the agent passes `sourceBranch`, because plugins cannot read a session's worktree branch. The tool checks that the branch exists on the remote and explains how to push otherwise.
- Verification:
  - `go test -race ./...` passes, including `task_links_test.go`, `agent_tools_test.go`, `mentions_test.go`, `references_test.go`, and `review_test.go`.
  - Vitest 79 passes.
  - The manifest passes the host's `manifest.Validate`.
  - Packaged E2E covers task launch from a work item, the Link menu, and the `#42` indicator (task 07).
  - Smoke on a disposable Kandev from the removal branch in mock mode:
    - Topbar and sidebar indicators, and the review panel with reviews and checks.
    - Link submenu entries, and a localized not-found error.
    - Task launch from a quick action creates a linked task.
- Open:
  - Native Create PR and the agent tool were not exercised on a host. Both need a pushed branch on a real Azure Repos remote; they are covered by unit tests only.
  - Owner validation on `contoso`.
