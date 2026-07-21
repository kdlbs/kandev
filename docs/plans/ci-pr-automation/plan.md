---
spec: docs/specs/ui/ci-pr-automation.md
created: 2026-06-18
status: draft
---

# Implementation Plan: CI PR Automation Controls

## Overview

Add task-level PR automation options to the existing GitHub PR CI popover. The backend owns durable options, per-PR dedupe/checkpoints, default prompt resolution, and automation execution from existing PR watch events. The frontend adds API/state/hooks and renders controls plus a task-specific prompt editor in the existing desktop popover and mobile drawer. E2E verifies the visible controls and the automation behavior against mocked PR states.

---

## Backend

### GitHub persistence and models

Files:

- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/store_test.go` or focused `store_ci_automation_test.go`

Add durable CI automation option and per-PR state models:

- `TaskCIAutomationOptions`
- `TaskCIAutomationOptionsPatch`
- `TaskCIPRAutomationState`
- `TaskCIPRAutomationOptionsResponse`

Add tables:

- `github_task_ci_options`
- `github_task_ci_pr_state`

Store methods:

- `GetTaskCIOptions(ctx, taskID string) (*TaskCIAutomationOptions, error)`
- `UpdateTaskCIOptions(ctx, taskID string, patch TaskCIAutomationOptionsPatch) (*TaskCIAutomationOptions, error)`
- `ListTaskCIPRStates(ctx, taskID string) ([]*TaskCIPRAutomationState, error)`
- `GetTaskCIPRState(ctx, taskID, repositoryID string, prNumber int) (*TaskCIPRAutomationState, error)`
- `RecordTaskCIFixAttempt(ctx, state TaskCIFixAttempt) error`
- `RecordTaskCIMergeAttempt(ctx, state TaskCIMergeAttempt) error`
- `RecordTaskCIError(ctx, taskID, repositoryID string, prNumber int, message string) error`
- `ClearTaskCIError(ctx, taskID, repositoryID string, prNumber int) error`

The store returns default disabled options when no options row exists.

### Default prompt

Files:

- `apps/backend/config/prompts/ci-auto-fix.md`
- `apps/backend/config/prompts/embed.go`
- `apps/backend/internal/prompts/store/sqlite.go`
- `apps/backend/internal/prompts/service/service.go`
- Prompt store/service tests under `apps/backend/internal/prompts/...`

Seed a built-in prompt:

- `id = "builtin-ci-auto-fix"`
- `name = "ci-auto-fix"`
- `builtin = true`

Add prompt service resolution by name with embedded fallback so automation can resolve the current default prompt even if the database row is missing.

### GitHub API

Files:

- `apps/backend/internal/github/controller.go`
- `apps/backend/internal/github/handlers.go` if a websocket action is added
- `apps/backend/pkg/websocket/actions.go` if a websocket action/event is added
- `apps/backend/internal/github/controller_test.go`

Add HTTP routes under `/api/v1/github`:

- `GET /tasks/:taskId/ci-options`
- `PATCH /tasks/:taskId/ci-options`

Response shape follows the spec. The response includes the effective prompt and current per-PR automation state for linked PRs.

Optionally add websocket push:

- `github.task_ci_options.updated`

### Automation execution

Files:

- `apps/backend/internal/orchestrator/event_handlers_github.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/messagequeue/types.go` if metadata constants are needed
- `apps/backend/internal/github/service_pr.go`
- `apps/backend/internal/github/service_pr_watch.go`
- `apps/backend/internal/orchestrator/event_handlers_github_test.go`
- New focused orchestrator test file if needed, e.g. `event_handlers_github_ci_automation_test.go`

Subscribe the orchestrator to PR state events that can drive automation:

- `events.GitHubTaskPRUpdated`
- existing `events.GitHubPRFeedback`

Keep the 1-minute poller lightweight. Full `GetPRFeedback` should happen only after a candidate status/review state is observed.

Add pure helpers for:

- auto-fix candidate detection
- merge-readiness detection
- fix checkpoint/signature creation
- merge readiness signature creation
- feedback delta extraction against the last checkpoint
- prompt rendering variables

Auto-fix sends the rendered prompt through `PromptTask` when possible or `QueueMessageWithMetadata` when the session is busy. Auto-merge calls `MergePR(ctx, owner, repo, number, "")` and records attempt/error state.

---

## Frontend

### Types and API client

Files:

- `apps/web/lib/types/github.ts`
- `apps/web/lib/api/domains/github-api.ts`
- `apps/web/lib/api/domains/github-api.test.ts`

Add types:

- `TaskCIAutomationOptions`
- `TaskCIAutomationOptionsPatch`
- `TaskCIPRAutomationState`

Add API functions:

- `getTaskCIAutomationOptions(taskId, options?)`
- `updateTaskCIAutomationOptions(taskId, patch, options?)`

### State and hook

Files:

- `apps/web/lib/state/slices/github/types.ts`
- `apps/web/lib/state/slices/github/github-slice.ts`
- `apps/web/lib/state/slices/github/github-slice.test.ts`
- `apps/web/hooks/domains/github/use-task-ci-options.ts`
- `apps/web/hooks/domains/github/use-task-ci-options.test.tsx`
- `apps/web/lib/ws/handlers/github.ts` if websocket updates are added

Add task-keyed automation option state with loading/saving/error information. Add a hook that loads the options when a PR popover needs them and provides update/reset helpers.

### Popover controls

Files:

- `apps/web/components/github/pr-ci-popover.tsx`
- `apps/web/components/github/multi-pr-ci-popover.tsx`
- `apps/web/components/github/pr-status-chip.tsx`
- New `apps/web/components/github/pr-automation-controls.tsx`
- New `apps/web/components/github/pr-automation-controls.test.tsx`
- Existing `apps/web/components/github/pr-ci-popover.test.tsx`
- Existing `apps/web/components/github/pr-status-chip.test.tsx`

Render automation controls in `PRCIPopover` after status/review rows and before the existing manual merge button. The prompt editor must work in the desktop hover card and the mobile drawer, with reset-to-default support and disabled/error states.

The automation section includes:

- An info icon/help affordance explaining both toggles, the 1-minute lightweight PR watch cadence, candidate-only full feedback fetches, feedback snapshots/dedupe, and auto-merge readiness gates.
- An edit button for the task-specific auto-fix prompt.
- A prompt editor dialog/drawer that links to Settings > Prompts so users can edit the default `ci-auto-fix` prompt.

No separate changes should be needed in `apps/web/components/task/chat/chat-input-area.tsx` or `apps/web/components/task/passthrough-toolbar.tsx` if controls live inside the shared `PRCIPopover`.

### Prompt settings

Files:

- `apps/web/components/settings/prompts-settings.tsx`
- `apps/web/app/settings/prompts/page.tsx`
- Existing prompt settings tests if present

The settings page should list the seeded built-in `ci-auto-fix` prompt through the existing prompt list API. Add tests only if the UI needs explicit handling for the new built-in prompt.

---

## Tests

- **What:** default options read disabled and persist partial updates.
  **File:** `apps/backend/internal/github/store_ci_automation_test.go`
  **How:** real SQLite store tests.

- **What:** built-in `ci-auto-fix` prompt is seeded and resolvable with fallback.
  **File:** prompt store/service tests under `apps/backend/internal/prompts/`
  **How:** real SQLite prompt repository plus service resolver test.

- **What:** GET/PATCH CI options API returns defaults, effective prompt, per-PR states, and reset-to-default behavior.
  **File:** `apps/backend/internal/github/controller_test.go`
  **How:** HTTP controller test with service/store test fixture.

- **What:** auto-fix no-ops when disabled and queues one prompt when enabled for a failing PR.
  **File:** `apps/backend/internal/orchestrator/event_handlers_github_ci_automation_test.go`
  **How:** orchestrator unit tests with mock GitHub service and fake queue/prompt behavior.

- **What:** repeated same feedback snapshot does not duplicate prompts.
  **File:** `apps/backend/internal/orchestrator/event_handlers_github_ci_automation_test.go`
  **How:** table-driven checkpoint/signature tests plus event handler test.

- **What:** new or materially changed feedback after a checkpoint produces a new prompt containing only the new/changed items.
  **File:** `apps/backend/internal/orchestrator/event_handlers_github_ci_automation_test.go`
  **How:** pure delta helper tests and one handler test.

- **What:** auto-merge calls merge only for ready PRs and throttles repeated failed attempts.
  **File:** `apps/backend/internal/orchestrator/event_handlers_github_ci_automation_test.go`
  **How:** table-driven readiness tests plus mock merge call assertions.

- **What:** frontend API functions call the expected endpoints and serialize reset-to-default payloads.
  **File:** `apps/web/lib/api/domains/github-api.test.ts`
  **How:** fetch mock tests.

- **What:** frontend hook loads/saves options and handles errors.
  **File:** `apps/web/hooks/domains/github/use-task-ci-options.test.tsx`
  **How:** React hook tests with mocked API.

- **What:** popover renders controls, toggles options, opens prompt editor, and resets override.
  **File:** `apps/web/components/github/pr-automation-controls.test.tsx`
  **How:** component tests with mocked hook/API state.

- **What:** automation help affordance explains cadence, watched states, snapshots, and merge gates.
  **File:** `apps/web/components/github/pr-automation-controls.test.tsx`
  **How:** component test activates the info icon and asserts the explanatory content is visible.

- **What:** task prompt editor links to default prompt settings.
  **File:** `apps/web/components/github/pr-automation-controls.test.tsx`
  **How:** component test opens the editor and asserts the Settings > Prompts link points at the prompt settings page.

- **What:** mobile drawer renders automation controls without overflow.
  **File:** `apps/web/components/github/pr-status-chip.test.tsx` or E2E.
  **How:** mobile viewport render test or Playwright screenshot/assertions.

---

## E2E Tests

- **Scenario:** GIVEN a task with a linked open PR, WHEN the user opens the CI popover, THEN the two automation controls are visible.
  **File:** `apps/web/e2e/tests/pr/ci-automation-options.spec.ts`
  **What to verify:** desktop hover/popover and mobile drawer both expose controls.

- **Scenario:** GIVEN a task with a linked open PR, WHEN the user enables auto-fix and auto-merge, THEN the settings persist after reload.
  **File:** `apps/web/e2e/tests/pr/ci-automation-options.spec.ts`
  **What to verify:** checkbox/switch state after reload.

- **Scenario:** GIVEN a task uses the default prompt, WHEN the user customizes and then resets the task prompt, THEN the UI shows override state and returns to default state.
  **File:** `apps/web/e2e/tests/pr/ci-automation-options.spec.ts`
  **What to verify:** prompt editor save/reset behavior.

- **Scenario:** GIVEN the CI automation section is visible, WHEN the user opens the help affordance, THEN the popover/drawer explains watch cadence, candidate feedback fetches, snapshots, and merge gates.
  **File:** `apps/web/e2e/tests/pr/ci-automation-options.spec.ts`
  **What to verify:** help content is visible on desktop and does not overflow on mobile.

- **Scenario:** GIVEN the task auto-fix prompt editor is open, WHEN the user clicks the default prompt settings link, THEN the app navigates to Settings > Prompts.
  **File:** `apps/web/e2e/tests/pr/ci-automation-options.spec.ts`
  **What to verify:** link target/navigation for the default prompt settings page.

- **Scenario:** GIVEN auto-fix is enabled and the mocked PR gets a failing check, WHEN the backend automation handler runs, THEN one auto-fix message is sent or queued and duplicate events do not create duplicates.
  **File:** `apps/web/e2e/tests/pr/ci-automation-options.spec.ts` or backend integration test if E2E cannot observe queue state cleanly.
  **What to verify:** visible queued/chat message or backend mock assertion.

- **Scenario:** GIVEN auto-merge is enabled and the mocked PR becomes ready, WHEN the backend automation handler runs, THEN the merge endpoint is called and UI updates to merged.
  **File:** `apps/web/e2e/tests/pr/ci-automation-options.spec.ts`
  **What to verify:** mock merge call and merged banner/state.

---

## Implementation Waves

Wave 1 (sequential foundations):

- [x] [task-01-backend-persistence-prompts](task-01-backend-persistence-prompts.md)

Wave 2 (backend contracts and behavior):

- [x] [task-02-backend-ci-options-api](task-02-backend-ci-options-api.md)
- [x] [task-03-backend-automation-execution](task-03-backend-automation-execution.md)

Wave 3 (frontend integration):

- [x] [task-04-frontend-api-state-hook](task-04-frontend-api-state-hook.md)
- [x] [task-05-frontend-popover-controls](task-05-frontend-popover-controls.md)

Wave 4 (end-to-end and final validation):

- [x] [task-06-e2e-ci-automation](task-06-e2e-ci-automation.md)
- [ ] [task-07-qa-verify-and-docs](task-07-qa-verify-and-docs.md)

---

## Verification Commands

Targeted backend:

```bash
rtk make -C apps/backend test
```

Targeted frontend:

```bash
cd apps && rtk pnpm --filter @kandev/web test -- pr-automation-controls
cd apps && rtk pnpm --filter @kandev/web test -- use-task-ci-options
cd apps && rtk pnpm --filter @kandev/web typecheck
```

Targeted E2E:

```bash
cd apps/web && rtk pnpm e2e -- tests/pr/ci-automation-options.spec.ts
```

Final verification:

```bash
rtk make fmt
rtk make typecheck test lint
```

---

## Risks

- The auto-fix path must not fetch full PR feedback for every watched PR every minute. Candidate detection must stay lightweight until full details are necessary.
- Prompt dedupe must be durable in the database, not dependent on in-memory caches.
- Auto-merge must fail closed. The backend readiness predicate should be stricter than the frontend display predicate when data is unknown.
- Hover-card UI must remain usable with interactive controls; if hover dismissal fights editing, the controls may need a nested dialog or click-pinned state.

## Open Questions

- None.
