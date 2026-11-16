---
created: 2026-11-16
status: complete
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-007
system_design:
  - ../../specs/tasks/system-design/workflow-task-start-context.md
legacy_specs: []
---

# Implementation Plan: Current workflow step on ordinary starts

## Overview

Ordinary existing-task starts must compose the current workflow step when the caller omits `workflow_step_id`.
One sequential work order owns the correction, service regressions, and API documentation.
Task 01 is complete.

## Evidence and root cause

Source: [issue #5337](https://github.com/kdlbs/kandev/issues/5337), open with no comments or image attachments during investigation.
Investigation date: 2026-11-16. Checkout: `8b46ceba098d20ddb11de01d6d9d9d947edc71e5`.
The same mismatch exists on remote main at `2e0ad9d5f2d7e145faf7acb8f16f44cb98bb3f17`.
The issue is assigned to the authenticated GitHub user, `carlosflorencio`.

`startTask` resolves an omitted incoming step into `workflowSessionConfigStepID` for session routing and configuration.
Its `applyWorkflowAndPlanMode` call still receives the empty incoming `workflowStepID`.
The composition helper therefore bypasses the step and sends the task description without step instructions.

The issue's original request payload is unavailable.
The reproduction proves this source path, not the exact cause of every event in the reporter's transcript.

### Temporary reproduction

`TestIssue5337StoredStepFirstPrompt` used a Go overlay, leaving repository source and tests untouched.
It created an existing task bound to a review step with `auto_start_agent`, a distinct prompt sentinel, and a conditional configuration rule.
The real start path selected `review-profile` and persisted `step-model` plus `reasoning_effort=max` before the mock launch callback.

The baseline command was:

```bash
(cd apps/backend && go test -trimpath -overlay /tmp/kandev-issue-5337-4tskbvs1/baseline.json ./internal/orchestrator -run '^TestIssue5337StoredStepFirstPrompt$' -count=1 -v)
```

| Case | Baseline result |
| --- | --- |
| Omitted step, ordinary prompt | Fails. Step sentinel is absent. |
| Explicit current step, ordinary prompt | Passes |
| Omitted step, `skip_step_prompt` | Fails. Task-description fallback remains. |
| Explicit current step, `skip_step_prompt` | Passes |

All four baseline profile and runtime-configuration assertions passed.
The candidate harness adds Office and helper prompt controls.
The disposable overlay supplies design evidence. Task 01 creates the permanent regression from these fixture conditions.

## Requirement conformance

Existing requirements cover automatic entry, reset, fallback admission, and creation placement.
They do not explicitly define current-step prompt parity for ordinary omitted-ID starts.
Draft requirement `REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-007` adds that contract in the existing task owner.
Its focused design separates direct starts from the larger reset design, which is near its size limit.

Companion packages include [start ownership](../workflow-step-agent-start-ownership/plan.md),
[asynchronous preservation](../workflow-async-start-prompt-preservation/plan.md),
[creation admission](../task-create-initial-turn-start/plan.md),
[saved workflow prompts](../workflow-entry-saved-prompts/plan.md), and
[prompt handoff integrity](../workflow-prompt-handoff-integrity/plan.md).
Their entry, recovery, and prepared-session contracts remain separate.
This package changes prompt-step selection at ordinary fresh launch. It requires no new ADR.

## Scope

In scope: omitted-ID prompt parity, explicit step/profile precedence, existing template transforms, skip overrides, plan mode, and caller compatibility.
Out of scope: historical transcript repair, recovery replay, missing automatic-start actions, successor handoff, and launch admission changes.
No new UI, provider adapter, persistence, telemetry, or dependency is required.
Task 01 completed the implementation. Commits, push, and PR creation remain outside this work order.

## Technical approach

Task 01 changes the existing composition call in `apps/backend/internal/orchestrator/task_operations.go`.
A local prompt-step value starts with incoming `workflowStepID`.
For an ordinary omitted-ID start, it falls back to existing `workflowSessionConfigStepID`.
Office and `PreserveWorkflowSourceBinding` launches retain their incoming prompt step.
`ProfileExplicit` remains independent from prompt composition.

The original argument continues to serve movement, session creation, events, launch options, and replay serialization.
The existing builder retains all prompt transforms. No new resolver or repository read is needed.

| Launch shape | Required outcome | Evidence |
| --- | --- | --- |
| Structured ordinary start, stored step | Step text and matching configuration | Runtime request capture plus SQLite metadata |
| Explicit step/profile | Explicit step supplies context. Explicit profile retains profile authority. | Distinct-step sentinels and profile assertion |
| Skip override | Step and description suppressed. Applicable workflow/instruction context remains. | Prompt and configuration assertions |
| Office or helper, omitted step | Caller prompt retained | Separate negative controls |
| Ephemeral or unbound task | Existing direct prompt retained | Negative controls |
| Passthrough | Visible template rules retained. Hidden reference context excluded. | Focused composition coverage |

The service seam is provider-independent. Evidence does not include a clean macOS, Homebrew, or Codex transport reproduction.
Existing missing-step and read-failure behavior remains authoritative.

## Tests

New file: `apps/backend/internal/orchestrator/task_start_workflow_prompt_test.go`.
New table-driven test: `TestStartTask_CurrentStepPrompt`.
The abbreviated criterion labels below use prefix `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-`.

| Criteria | Cases and assertions |
| --- | --- |
| `007.1` | Omitted/explicit current step. Step sentinel, selected profile, and persisted model/options at launch. |
| `007.2`, `007.7` | Different explicit step wins. Explicit profile wins independently. Omitted-ID placement stays unchanged without repeated entry actions. |
| `007.3` | Placeholder, replacing template, empty template, and nonempty caller input preserve existing semantics. |
| `007.4` | Skip for both step forms. Workflow instructions, one-time instructions, and configuration remain. |
| `007.5` | Step plan mode and saved references appear once in stored and dispatched content. |
| `007.6` | Office, helper, ephemeral, and unbound-task controls retain caller text. |
| `007.8` | A later omitted-step lookup error retains direct-prompt launch behavior and omits the step template. |

Existing composition, direct-prompt fallback, explicit-profile, and helper-binding tests provide adjacent checks.

## E2E tests

The Go service test provides end-to-end evidence for this backend composition contract.
It covers the stored task step, real launch orchestration, durable configuration, stored initial message, and captured runtime request.
Only the external agent boundary uses a mock.
No rendered interface or browser contract changes. This package therefore adds no Playwright scenario or UI preview.

## Work orders

- [x] [Task 01: Compose the current step on ordinary starts](task-01-compose-current-step.md) (`done`, sequential, no dependencies).

## Verification

Task 01 owns the [exact commands](task-01-compose-current-step.md#verification) for TDD, race detection, compatibility, lint, and documentation.
The primary command is:

```bash
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run '^TestStartTask_CurrentStepPrompt' -count=1)
```

## Verification results

- Baseline: two expected prompt assertion failures and two passing explicit-ID controls. All baseline profile/configuration checks passed.
- Candidate overlay: all six cases passed in package time 0.510 seconds.
  Cases cover omitted/explicit step IDs, both skip variants, helper prompts, and Office prompts.
  The corrected Office fixture binds `KANDEV_TASK_ID` to the fixture task before launch.
  Command: the baseline command with `candidate.json` as its overlay.
- Catalog validation passed: 453 decisions and 2019 specifications.
- Specification-linter tests passed: 36 tests. Full specification lint passed.
- PR-documentation preflight passed with the planned runtime path included as a prospective change. Requirement, design, plan, and work-order references resolve.
- Relative file links and document diff checks passed. Temporary overlays were removed after evidence capture.
- The permanent `TestStartTask_CurrentStepPrompt` regression failed before the production change because omitted-ID ordinary starts omitted the step sentinel and bypassed skip suppression. Explicit-step and caller-owned prompt controls passed.
- The new 14-case `TestStartTask_CurrentStepPrompt` passes after the local prompt-step fallback. It covers stored and explicit steps, profile precedence, template variants, skip overrides, plan mode, saved references, and Office/helper/ephemeral/unbound controls.
- A separate regression injects a failure in the later omitted-step lookup after task classification succeeds. It confirms the accepted degraded behavior: the start logs the failure and launches the matching caller prompt without step-template text.
- Focused regression passed: `(cd apps/backend && go test -trimpath ./internal/orchestrator -run '^TestStartTask_CurrentStepPrompt' -count=1 -v)`.
- Race regression passed: `(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run '^TestStartTask_CurrentStepPrompt' -count=1)`.
- Compatibility tests passed: `(cd apps/backend && go test -trimpath ./internal/orchestrator -run '^(TestApplyWorkflowAndPlanMode_|TestLaunchSession_ProfileExplicit|TestStartTask_PreservesOnlyResolvedWorkflowPromptExpansion|TestLaunchSpawnPreservesWorkflowSourceBinding|TestStartTask_Office)' -count=1)`.
- Focused Go lint passed with `bash scripts/lint-go-changed apps/backend/internal/orchestrator/task_operations.go apps/backend/internal/orchestrator/task_start_workflow_prompt_test.go`.
- `make -C apps/backend build` passed.
- Specification catalog validation, all 36 specification-linter tests, full specification lint, all 63 public-doc validator tests, live public-doc validation, PR documentation coverage preflight, and `git diff --check` passed.

## PR review follow-up

A current-head review raised the later omitted-step lookup failure path. The package keeps the existing warning-and-direct-prompt behavior in that degraded case to preserve launch compatibility; requirement criterion `007.8`, the focused design, and `TestStartTask_CurrentStepPromptRetainsDirectPromptWhenStoredStepLookupFails` now state and verify the exception. The regression confirms that stored and dispatched prompt text still match and that the step template is absent when the fallback read fails.

The fixup reran the focused and race-enabled prompt tests, adjacent workflow/profile/Office tests, changed-package Go lint, specification catalog validation, all specification-linter tests and full lint, all 63 public-doc validator tests, live public-doc validation, PR documentation coverage preflight, and `git diff --check`. All passed.

## Risks

- Unconditional fallback can inject step templates into Office or helper prompts. Explicit caller exclusions and negative controls bound this risk.
- A template without `{{task_prompt}}` replaces ordinary task content. Tests must preserve that behavior.
- Reassigning the request argument can affect movement and replay. The correction must use a local composition value.
- The workspace base differs from remote main. Reconcile IDs, caller contracts, and the launch seam before implementation.

## Documentation impact

Internal docs include the owning requirement, design cross-reference, focused design, and completed delivery package.
Task 01 updates `docs/public/websocket-api.md`, a protocol reference, to describe the omitted-ID prompt behavior.
