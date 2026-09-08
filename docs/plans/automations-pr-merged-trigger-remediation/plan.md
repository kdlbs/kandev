---
spec: docs/specs/office/requirements/automations-pr-merged-trigger.md
created: 2026-08-09
status: complete
---

# Implementation Plan: Merged-PR Automation Remediation

## Overview

Finish PR #2462 by repairing the merged-PR event path across both supported buses, making start-up
recovery real across the complete consumer chain, binding the archive mutation to the event-selected
task, and covering the recovery, mobile, accessibility, tooling, and documentation gaps found in
review. The trigger remains an ordinary automation that creates and starts a run task; no native
archive action or new persistence table is introduced.

## Confirmed root causes

- `githubPRMergedSubscriber.handle` accepts only `*github.TaskPR`, while the NATS bus decodes
  `Event.Data` as `map[string]any`. The NATS configuration therefore drops every merged-PR event.
- `Start` marks the subscriber started and returns without subscribing when its lookup is absent.
  This contradicts the service's live-lookup design because later wiring can never recover.
- The GitHub poller can run its immediate sweep before `orchestratorSvc.Start` subscribes to
  `automation.triggered`. Starting only the merged-PR subscriber earlier still leaves a downstream
  event gap.
- The default prompt asks the agent to archive one task, but the generic archive tool accepts any
  owner-reachable task. Prompt text is not an enforcement boundary.
- The trigger-specific keyless skip/failure writes are not protected by focused regressions.
- The PR adds a handwritten glob fallback for unsupported Node versions even though the workspace
  requires Node 24, and the new editor surface lacks mobile E2E, label association, and public docs.

## Architecture

### Event transport and lifecycle

- Add one normalization helper at the merged-PR subscriber boundary. It accepts the typed in-memory
  payload and the JSON-decoded NATS object form, returning one validated `github.TaskPR` shape.
- Keep malformed payloads fail-closed and local to this subscriber. Do not change the generic bus
  envelope or make every event consumer depend on reflection-based decoding.
- Always attach the bus subscription during `Start`; lookup availability is checked live per event.
- Compose start-up in dependency order after construction and wiring:
  `orchestrator automation consumer -> merged-PR subscriber -> GitHub poller`.
  Register cleanup so producers stop before consumers.

### Bound archive target

- Persist the validated event `task_id` on the newly created automation-run task under the stable
  task-metadata key `automation_target_task_id`.
- Have the in-session MCP server inject its current run-task id as `caller_task_id` into the backend
  archive request.
  Do not expose that caller id in the MCP tool schema.
- At `handleArchiveTask`, load the caller. When it is an `automation_run` created by a
  `github_pr_merged` firing, require the requested id to equal the persisted target before invoking
  `ArchiveTask`. Missing/malformed metadata and mismatches fail closed.
- Preserve existing owner authorization and generic archive behavior for ordinary task sessions and
  automation runs from other trigger types.

This follows
[ADR-2026-08-09](../../decisions/2026-08-09-bind-automation-mutations-to-event-targets.md).

## Frontend and mobile design contract

- **Entry point:** Settings -> workspace Automations -> create/edit automation -> select
  "Pull request merged".
- **Presentation:** keep the short configuration inline in the existing automation editor card; no
  new overlay, drawer, or mobile-only state model.
- **Nearest mobile exemplar:** `apps/web/e2e/tests/mobile-automations-scroll.spec.ts` for the settings
  scroll owner and touch interactions.
- **Shared behavior:** desktop and mobile use the same trigger config component, draft state, save
  contract, and translations.
- **Mobile proof:** select the trigger, exercise repository and base-branch controls with touch,
  save/reopen, verify the dead-configuration warning and persistence, assert no horizontal overflow,
  and keep controls within the viewport.
- Associate the base-branch `Label` and `Input` with a stable id so the field has an accessible name.

## Public documentation

The primary doc is `docs/public/automation-and-mcp.md` (how-to/explanation). It must describe the
trigger's linked-task requirement, repository/base-branch filters, poll latency, first-observation
semantics, bound archive target, and retry/manual-run limitations. Update the reference matrix in
`docs/public/feature-status.md` in the same task.

## Implementation tasks

- [x] [Task 01: Repair event delivery and startup](task-01-event-delivery-and-startup.md)
- [x] [Task 02: Bind the archive target](task-02-bind-archive-target.md)
- [x] [Task 03: Lock dedup recovery semantics](task-03-dedup-recovery-tests.md)
- [x] [Task 04: Finish mobile and tooling details](task-04-mobile-accessibility-and-tooling.md)
- [x] [Task 05: Document and verify the feature](task-05-docs-and-verification.md)

Execution is sequential in the primary conversation. No subagent delegation is planned or
authorized. Tasks 01 and 02 establish the behavioral boundaries; the remaining work builds on them.

## Validation

Run from the repository root unless noted otherwise. A fresh worktree must first run
`cd apps && pnpm install --frozen-lockfile` if dependencies are absent.

- `cd apps/backend && go test ./internal/automation ./internal/backendapp`
- `cd apps/backend && go test ./internal/orchestrator ./internal/mcp/handlers ./internal/mcp/server`
- `cd apps && pnpm --filter @kandev/web test -- scripts/lib/guard-allowlist.test.ts`
- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm e2e:run --project chromium tests/automations-pr-merged-trigger.spec.ts`
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/mobile-automations-pr-merged-trigger.spec.ts`
- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`

Do not replace the behavioral start-up or NATS tests with source-order assertions. The implementation
turn should record RED/GREEN evidence in each task file and update task/plan statuses as work lands.

## Risks

- Trusting a caller-supplied run-task id would defeat the archive binding; it must come from the
  current MCP server instance and stay out of the public arguments.
- A broad change to generic event decoding could alter unrelated subscribers. Normalize only this
  payload unless a separately reviewed bus contract is introduced.
- Moving component starts without preserving reverse cleanup can allow a producer to publish into a
  stopped consumer during shutdown.
- Automation task metadata is untyped. Centralize key names and parse defensively so legacy or
  malformed rows fail closed without panics.
- Mobile automation fixtures mutate persistent workspace state; the E2E must use its own automation
  and delete it in cleanup.

## Completed implementation

All five tasks are complete. The final implementation covers both event transports and startup
ordering, binds merged-PR archive mutations to the event-selected task, locks the trigger-specific
dedup recovery paths, adds mobile/accessibility/tooling coverage, and documents the shipped limits.
