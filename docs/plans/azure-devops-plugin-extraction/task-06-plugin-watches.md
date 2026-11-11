---
id: "06-plugin-watches"
title: "Watches"
status: in_progress
wave: 3
depends_on: 
  - "02-plugin-connection"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-007
acceptance_criteria:
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-007.1
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-007.2
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-007.3
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-007.4
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-007.5
system_design:
  - ../../specs/integrations/system-design/azure-devops-plugin.md
---

# Task 06: Watches

## Summary

Port the work-item and pull-request watches: settings, poller, reservations, trigger, reset preview and reset, cleanup. Tasks are created with `Tasks().Create` on shared profiles; a rejected launch disables the watch with a visible reason.

## In scope

- `internal/watches` port of `watch_service.go`, `watch_store.go`, `store_watch_reset.go`, `watch_cleanup.go`, `poller.go` onto Host state.
- Watch settings UI listing workflows, steps, and the profiles the plugin can launch with; disabled-reason row (UI-03).
- Metadata `azure_devops_watch_id` and `azure_devops_item_key` on created tasks.

## Out of scope

- Sidebar watch classification and launches on owned executors (known differences).

## Acceptance

- A watch creates exactly one task per matching item, including across a plugin restart.
- A watch whose executor profile becomes unavailable is disabled with the localized reason on desktop and phone.

## ASCII UI preview

See UI-03 in the [plan](plan.md#ascii-ui-preview). This work order owns the rendered result of those views.

## Verification

Commands run from the Kandev repository root; the plugin worktree is its sibling.

```bash
(cd ../JnManso-kandev-plugin-azure-devops-main && go test -race ./internal/watches/... -count=1)
(cd ../JnManso-kandev-plugin-azure-devops-main && npm test && npm run e2e)
```

## Files likely touched

- `../JnManso-kandev-plugin-azure-devops-main/internal/watches/`
- `../JnManso-kandev-plugin-azure-devops-main/ui/src/settings/watches/`

## Dependencies

Task 02-plugin-connection

## Risks

- Poll loop goroutine ownership: start in `SetHost`, stop on context cancellation.

## Parallelism

`parallel-safe`

## Inputs

- [System design](../../specs/integrations/system-design/azure-devops-plugin.md)
- `apps/backend/AGENTS.md`, `apps/web/AGENTS.md`, `docs/public/plugins-authoring.md`, and the `/create-kandev-plugin`, `/e2e`, and `/mobile-parity` skills where relevant

## Results

- 2026-11-06: implemented on plugin branch `feat/bootstrap` in commits `b3f9410` (feature) and `769fc26` (fixes from the host smoke test).
  - `internal/watches`: watches in workspace state `watch.<id>`, reservations in `watch.<id>.item.<generation>.<itemKey>`.
    - Item keys are `wi-<id>` and `pr-<repositoryGuid>-<id>`.
    - A reservation is written before `Tasks().Create` and released when creation fails, so an item creates at most one task per generation, including across restarts.
  - Poller: every minute from `SetHost`, stopped with the runtime context. Watches poll at their own interval, at least 60 seconds.
  - Tasks are created with the watch's workflow, step, repository, base branch, interpolated prompt, `StartAgent`, and `Launch{AgentProfileID, ExecutorProfileID, Prompt}`. The metadata carries `azure_devops_watch_id`, `azure_devops_item_key`, and `azure_devops_item_url`.
  - The open-task limit counts the watch's tasks that are not COMPLETED, FAILED, CANCELLED, or archived.
  - When Kandev rejects a launch with `launch.agent_profile_id is not available` or `launch.executor_profile_id is not available`, the watch is disabled. It stores `agent_profile_unavailable` or `executor_profile_unavailable`, and settings show a localized reason naming the profile (UI-03).
  - Actions (workspace scope): `watches.list`, `watches.options` (workflows with steps, agent and executor profiles, and repositories from the host read APIs), `watches.create`, `watches.update`, `watches.delete`, `watches.trigger`, `watches.preview_reset`, and `watches.reset`.
  - Settings UI: work-item and pull-request sections with cards, an editor (dialog on desktop, full-height drawer on phones) using selects for project, Azure repository, workflow, step, profiles, and repository, run now, enable/disable, reset, and delete confirmations.
  - Deviations:
    - Kandev offers plugins no task deletion (`Tasks()` has no Delete, `PluginOwnedTaskTrees.Delete` is refused, and exact archive needs management approval). The cleanup policy is therefore dropped. Reset starts a new generation and delete removes the watch; both keep the created tasks. The requirements and design record this.
    - Watch projects are stored by name, which the Azure DevOps REST API accepts like an ID.
- Verification:
  - `go test -race ./internal/watches/...` passes. It covers one task per item across polls and a restart, the in-flight limit, reset, disabling on an unavailable profile, retry after other failures, `@me` resolution, and interval handling.
  - Plugin action tests pass, and Vitest covers the editor model and the disabled reason.
  - Packaged E2E creates a watch in settings, runs it twice (one task), and shows the disabled reason on desktop and phone (task 07).
  - Smoke on the host: the options list the host's workflows and shared profiles; a work-item watch created one auto-started task with the metadata; an unknown executor disabled the watch with its reason.
- Open: owner validation on `contoso`.
- 2026-11-10: owner testing on `contoso` found that **Run now** created no task: watch titles (`[project#id] title`) exceeded Kandev's 60-character task title limit. Plugin commit `10921fe` shortens them with an ellipsis and keeps the full title in the prompt; the watch fake and `hosttest` now reject titles over 60 characters like Kandev. `TestTaskTitleFitsKandevLimit` and `TestWatchRunCreatesTaskForLongWorkItemTitle` failed before the change and pass after it.
