---
spec: docs/specs/agents/requirements/native-code-review.md
created: 2026-07-24
status: building
---

# Implementation Plan: Native Code Review

## Overview

Findings are a new task-scoped persisted entity that reuses the existing anchored-annotation rendering surface in the Changes/Review diff. Persistence and the publish contract land first, then the run orchestrator that turns a task's working diff into findings through the utility-agent inference substrate, then the two triggers (WS action, workflow `on_enter` action) and the MCP publish tool, then the frontend store/annotations/toolbar, then mobile parity and E2E. The walkthrough feature (`task_walkthroughs` → `WalkthroughService` → bus event → WS → `walkthroughs` store slice → `walkthrough-step` diff annotation) is the reference pattern end to end; findings differ by being many-per-task, repository-scoped, and status-mutable.

---

## Backend

### Schema & models (`internal/task/`)

- `internal/task/models/models.go` — add `TaskReviewRun`, `TaskReviewFinding`, the `ReviewRunStatus`/`ReviewRunTrigger`/`ReviewSeverity`/`ReviewFindingStatus` string enums, `ErrTaskReviewRunNotFound`, `ErrTaskReviewFindingNotFound`.
- `internal/task/repository/sqlite/base_schema.go` — `initTaskReviewSchema()` creating `task_review_runs` and `task_review_findings` exactly as specified in the spec's Data model, both with `CREATE TABLE IF NOT EXISTS`.
- `internal/task/repository/sqlite/base_migrations.go` — idempotent `CREATE INDEX IF NOT EXISTS` for `task_review_findings(task_id, status)`, `task_review_findings(task_id, repository_name, file_path)`, `task_review_findings(run_id)`, `task_review_runs(task_id)`. Indexes live in migrations, never in the schema-init block (see `apps/backend/AGENTS.md`).
- `internal/task/repository/sqlite/review.go` — `CreateTaskReviewRun`, `UpdateTaskReviewRun`, `GetTaskReviewRun`, `ListTaskReviewRuns(taskID, limit)`, `CancelInFlightTaskReviewRuns()` (restart sweep), `CreateTaskReviewFindings(ctx, []*TaskReviewFinding)` (single transaction), `ListTaskReviewFindings(taskID)`, `GetTaskReviewFinding`, `UpdateTaskReviewFindingStatus`, `DeleteTaskReviewFindingsByTask`, `DeleteSupersededTaskReviewFindings(taskID, runID, keys)`.
- `internal/utility/hash/djb2.go` (new small package) — `DJB2(string) string`, byte-for-byte equal to `apps/web/lib/utils/hash.ts` (`hash = 5381`, `((hash<<5)+hash+ch) | 0` as int32, output `uint32` lowercase hex).

### Review service (`internal/task/service/review_service.go`)

- `ReviewService` with the same shape as `WalkthroughService`: minimal local `reviewRepo` interface, `bus.EventBus`, logger.
- `PublishFindings(ctx, PublishFindingsRequest) (*TaskReviewRun, []*TaskReviewFinding, error)` — normalizes + validates every finding (file required, `start_line > 0`, `end_line >= start_line`, known severity, title/body non-empty, title ≤ 120 chars and newline-stripped, category slug ≤ 40 chars, `anchor_text` truncated at 2000 chars), rejects the whole batch on any invalid entry, deletes superseded rows, inserts, publishes `events.TaskReviewFindingsPublished`.
- `UpdateFindingStatus`, `ClearTaskReview`, `GetTaskReview(taskID)`, `CreateRun`, `MarkRunRunning`, `CompleteRun`, `FailRun`, `CancelRun` — each publishing `events.TaskReviewRunUpdated` / `TaskReviewFindingUpdated` / `TaskReviewCleared`.
- `events.go` (`internal/events/types.go`) — `TaskReviewRunUpdated`, `TaskReviewFindingsPublished`, `TaskReviewFindingUpdated`, `TaskReviewCleared`.
- `internal/gateway/websocket/task_notifications.go` + `pkg/websocket/actions.go` — forward those bus events to the client actions listed in the spec, following the `task.walkthrough.*` precedent.

### Run orchestration (`internal/review/`, new package)

- `resolver.go` — `AgentResolver` resolving `(agentID, model)` for a run: explicit `agent_profile_id` → agent profile's agent/model/mode; otherwise the enabled `code-review` builtin utility agent; otherwise the user's default utility agent/model. Returns `ErrReviewAgentUnavailable` when nothing resolves or the resolved agent profile is CLI-passthrough.
- `diff.go` — `CollectChanges(ctx, sessionID, repositoryID)` builds `[]ChangedFile{RepositoryID, RepositoryName, Path, Status, Diff, DiffHash}` by merging agentctl git-status (uncommitted) and cumulative-diff (committed) results, uncommitted winning dedup — mirroring `buildAllFiles` in `apps/web/components/review/review-dialog.tsx` so both halves agree on the file set. `DiffHash` uses `hash.DJB2` over the same normalized diff text the frontend hashes.
- `batch.go` — `PlanBatches(files, budget)` groups files into prompt batches under a byte budget (`reviewPromptBudgetBytes = 120_000`), never splitting a single file; a file whose own diff exceeds the budget is returned in `Skipped`.
- `runner.go` — `Runner.Run(ctx, RunRequest)`: create run → mark running → collect changes → plan batches → for each batch resolve the `code-review` prompt template with `template.Context{GitDiff, ChangedFiles, TaskTitle, TaskDescription, BranchName, BaseBranch}` and execute through the inference substrate (session-bound `ExecuteInferencePrompt` when the task has a live session, else the sessionless host-utility executor) → parse → accumulate → publish once → complete run. A run is tracked in an in-memory `map[taskID]runID` guard so a second request returns the in-flight run.
- `parse.go` — `ParseFindings(response string) (summary string, findings []Finding, rejected int, err error)`. Accepts a fenced ```json block or a bare JSON object with a `findings` array; tolerates prose before/after. Returns `ErrUnparseableResponse` when no array is recoverable.
- `config/utilityagents/code-review.md` + `internal/utility/store/builtins.go` — new `builtin-code-review` entry named `code-review`. The prompt instructs a strict JSON contract and carries the sentinel line `KANDEV_CODE_REVIEW_REQUEST` used by the mock agent.

### Triggers

- `internal/mcp/server/server.go` + `internal/mcp/server/handlers.go` + `internal/mcp/handlers/handlers.go` + `pkg/websocket/actions.go` — register `publish_review_findings_kandev` with the schema in the spec, routed through `ws.ActionMCPPublishReviewFindings` to `ReviewService.PublishFindings` with `trigger = agent`. Mirrors `show_walkthrough_kandev`.
- `internal/workflow/models/models.go` — `OnEnterRunCodeReview OnEnterActionType = "run_code_review"`, `GenericActionRunCodeReview`, and a `RunCodeReview *RunCodeReviewAction` payload carrying `AgentProfileID`.
- `internal/workflow/engine/types.go` — `ActionRunCodeReview ActionKind = "run_code_review"` plus the `RunCodeReview` field on the action struct; `internal/workflow/engine/adapters.go` maps the model action to the engine action.
- `internal/orchestrator/workflow_callbacks.go` — `runCodeReviewCallback` registered when `svc.reviewRunner != nil`, calling `Runner.Run` with `trigger = workflow_step`. A failed run does not fail step entry: the callback logs and returns nil after the run row records the failure.
- `internal/workflow/models/export.go` + `internal/workflow/service/` — export/import and validation for the new action, referencing the agent profile portably like existing step profiles.
- `internal/task/controller` / WS handlers — `task.review.run`, `task.review.cancel`, `task.review.get`, `task.review.finding.update`, `task.review.clear` with the DTOs from the spec, plus the boot-payload/`e2e_reset.go` additions (`DeleteTaskReviewByWorkspace` cascade) required for worker-scoped E2E resets.

---

## Frontend

### Types, API client, store

- `apps/web/lib/types/http.ts` — `TaskReviewRun`, `TaskReviewFinding`, `ReviewSeverity`, `ReviewFindingStatus`.
- `apps/web/lib/api/domains/review-api.ts` — `runTaskReview`, `cancelTaskReview`, `getTaskReview`, `updateReviewFindingStatus`, `clearTaskReview` over the WS client (same shape as `walkthrough-api.ts`).
- `apps/web/lib/state/slices/review/` — `review-slice.ts` with `runsByTaskId`, `findingsByTaskId`, `activeRunByTaskId`; actions `setTaskReview`, `upsertReviewRun`, `addReviewFindings`, `updateReviewFinding`, `clearTaskReview`. Registered in `lib/state/store.ts` and `lib/state/default-state.ts`.
- `apps/web/lib/ws/handlers/review.ts` + `lib/ws/router.ts` — handlers for the four server events.
- `apps/web/lib/review/findings.ts` — pure helpers: `findingFileKey(finding)` (reuses the `reviewFileKey` NUL composite), `isFindingStale(finding, currentDiffHash)`, `groupFindingsByFile(findings)`, `groupFindingsByRepository(findings)`, `openFindingCount`, `severityRank`, `reanchorFinding(finding, diff)` best-effort `anchor_text` relocation.

### Diff annotations

- `apps/web/components/diff/review-finding-card.tsx` — severity chip, category, title, Markdown body, optional suggestion block, and the Resolve / Dismiss / Send-to-agent actions; a muted variant for `resolved`/`dismissed`.
- `apps/web/components/diff/use-diff-annotation-renderer.tsx` — add `"review-finding"` to `AnnotationMetadata.type` and render `ReviewFindingCard`.
- `apps/web/components/diff/use-diff-viewer-state.ts` — `useReviewFindingAnnotations(filePath, repo, diffHash)` alongside `useWalkthroughSelection`; non-stale open/resolved findings anchor at `end_line`, stale findings are excluded here and surfaced by the file header instead.
- `apps/web/components/review/review-file-findings-banner.tsx` — per-file stale-findings group rendered above the diff so a stale finding is visible without being mis-anchored.

### Review surface

- `apps/web/components/review/review-run-button.tsx` — **Review changes** control in `review-top-bar.tsx` with idle / running / failed states, a Cancel action while running, and the `review_agent_unavailable` message linking to Settings → Utility Agents.
- `apps/web/components/review/review-findings-overview.tsx` — findings list grouped by repository then file, severity-sorted, with counts; mirrors `review-comments-overview.tsx`.
- `apps/web/components/review/review-diff-list.tsx`, `review-file-tree.tsx` — per-file open-finding badge next to the existing comment badge.
- `apps/web/components/task/changes-panel-header.tsx` / `changes-top-bar.tsx` — the same **Review changes** control on the Changes panel.
- `apps/web/hooks/domains/review/use-task-review.ts` — WS subscription + backfill via `getTaskReview` on mount; `use-send-finding-to-agent.ts` converting a finding into agent context through the existing pending-comment/context-item path.
- `apps/web/components/settings/workflows/` — step editor checkbox + agent-profile picker for `run_code_review`, with the self-documenting copy the frontend conventions require.

### Mobile

- `apps/web/components/task/mobile/mobile-changes-panel.tsx` — findings entry point opening a `Drawer` bottom sheet listing findings with the full desktop action set; run control in the mobile changes toolbar.

---

## Tests

| What | File | How |
|---|---|---|
| djb2 parity with the TS implementation | `internal/utility/hash/djb2_test.go` | table-driven against fixtures also asserted in `apps/web/lib/utils/hash.test.ts` |
| Run/finding CRUD, cascade delete, supersede | `internal/task/repository/sqlite/review_test.go` | real in-memory SQLite, fresh-DB + replay |
| Finding validation & whole-batch rejection | `internal/task/service/review_service_test.go` | table-driven over malformed entries |
| Event publication on publish/update/clear | `internal/task/service/review_service_test.go` | fake `bus.EventBus`, assert event types + payloads |
| Restart sweep cancels in-flight runs | `internal/task/repository/sqlite/review_test.go` | seed `running` rows, call `CancelInFlightTaskReviewRuns` |
| Agent/model resolution incl. passthrough rejection and no-agent case | `internal/review/resolver_test.go` | table-driven with fake profile + utility stores |
| Batch planning and oversized-file skip | `internal/review/batch_test.go` | table-driven |
| Response parsing: fenced JSON, bare JSON, prose-wrapped, malformed entry counted, unparseable | `internal/review/parse_test.go` | table-driven |
| Full run happy path + `review_no_changes` + `review_agent_unavailable` + in-flight dedup | `internal/review/runner_test.go` | fake executor + fake diff source |
| MCP publish tool accepts valid, rejects malformed | `internal/mcp/server/server_test.go` | existing MCP test harness |
| `run_code_review` action evaluation and failure isolation | `internal/workflow/engine/engine_test.go`, `internal/orchestrator/workflow_callbacks_test.go` | engine trigger with a fake runner |
| Findings helpers: file key, staleness, grouping, severity sort, re-anchor | `apps/web/lib/review/findings.test.ts` | vitest table-driven |
| Review slice reducers | `apps/web/lib/state/slices/review/review-slice.test.ts` | vitest |
| WS handlers update the slice | `apps/web/lib/ws/handlers/review.test.ts` | vitest |
| Finding annotation placement, stale exclusion | `apps/web/components/diff/use-diff-viewer-state.test.ts` | vitest |
| Run button states incl. unavailable message | `apps/web/components/review/review-run-button.test.tsx` | RTL |
| Findings overview grouping per repository | `apps/web/components/review/review-findings-overview.test.tsx` | RTL |
| Send-to-agent builds correct context | `apps/web/hooks/domains/review/use-send-finding-to-agent.test.ts` | vitest |

---

## E2E Tests

`apps/backend/cmd/mock-agent/handler.go` gains `isCodeReviewRequest(prompt)` (matching the `KANDEV_CODE_REVIEW_REQUEST` sentinel) returning a deterministic fenced-JSON findings payload, so both paths are hermetic. Rebuild with `make -C apps/backend build-mock-agent` before running.

| Scenario | File | Verify |
|---|---|---|
| GIVEN changed files, WHEN **Review changes** is used, THEN findings render at their anchored lines | `apps/web/e2e/review/code-review-on-demand.spec.ts` | run chip reaches completed; finding card visible in the diff at the expected line; overview count |
| GIVEN a finding, WHEN resolved, THEN it shows resolved and survives reload | same file | card state + count after `page.reload()` |
| GIVEN a finding, WHEN **Send to agent**, THEN the chat receives the finding context | same file | context chip / message content |
| GIVEN no capable agent, WHEN **Review changes**, THEN the unavailable message appears and no run is created | same file | inline message with the Settings link |
| GIVEN a workflow step with `run_code_review`, WHEN a task enters it, THEN findings appear in Review | `apps/web/e2e/review/code-review-workflow-step.spec.ts` | move task into step; findings visible; run trigger reported as workflow step |
| GIVEN a phone viewport with findings, WHEN Changes is opened, THEN the findings sheet exposes every action | `apps/web/e2e/mobile/code-review-findings.spec.ts` | bottom sheet visible; resolve works from it |

---

## Implementation Waves

```
Wave 1 (parallel):
- [x] [task-01-schema-and-repository](task-01-schema-and-repository.md)
- [x] [task-02-djb2-and-diff-collection](task-02-djb2-and-diff-collection.md)

Wave 2:
- [x] [task-03-review-service-and-events](task-03-review-service-and-events.md)

Wave 3 (parallel):
- [x] [task-04-run-orchestration](task-04-run-orchestration.md)
- [x] [task-05-mcp-publish-tool](task-05-mcp-publish-tool.md)

Wave 4 (parallel):
- [x] [task-06-ws-actions-and-dtos](task-06-ws-actions-and-dtos.md)
- [x] [task-07-workflow-step-action](task-07-workflow-step-action.md)

Wave 5:
- [x] [task-08-frontend-state-and-types](task-08-frontend-state-and-types.md)

Wave 6:
- [x] [task-09-diff-finding-annotations](task-09-diff-finding-annotations.md)

Wave 7:
- [ ] [task-10-review-surface-controls](task-10-review-surface-controls.md)

Wave 8:
- [ ] [task-11-mobile-parity](task-11-mobile-parity.md)

Wave 9:
- [ ] [task-12-e2e-and-mock-agent](task-12-e2e-and-mock-agent.md)

Wave 10:
- [ ] [task-13-public-docs](task-13-public-docs.md)
```
