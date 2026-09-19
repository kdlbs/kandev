---
id: "04-run-orchestration"
title: "Review run orchestration: resolver, batching, prompt, parser"
status: done
wave: 3
depends_on: ["02-djb2-and-diff-collection", "03-review-service-and-events"]
plan: "plan.md"
spec: "../../specs/agents/requirements/native-code-review.md"
---

# Task 04: Review run orchestration

Turn a task's changed files into findings through the utility-agent inference substrate.

## Inputs

- Spec **Failure modes** (every row), **State machine** (run transitions), **What** (agent/model independence).
- `internal/utility/handlers/handlers.go` — `InferenceExecutor.ExecuteInferencePrompt` (session-bound) and `HostUtilityExecutor.ExecutePrompt` (sessionless), and how `PreparePromptRequest` pairs agent + model.
- `internal/utility/template/engine.go` — `Context` fields available to the prompt.
- `internal/utility/store/builtins.go` + `config/utilityagents/` — how a builtin utility agent and its embedded prompt are declared.
- Agent profile resolution: `internal/agent/runtime/lifecycle/profile_resolver.go`; passthrough detection wherever the existing code distinguishes CLI-passthrough profiles.

## Work

1. `config/utilityagents/code-review.md` — the review prompt. Must contain the literal sentinel line `KANDEV_CODE_REVIEW_REQUEST`, the `{{ChangedFiles}}` and `{{GitDiff}}` placeholders, and a strict output contract: a single fenced ```json block containing `{"summary": string, "findings": [{"repo","file","line","line_end","severity","category","title","body","suggestion"}]}`, an empty array when there is nothing to report, and no prose outside the block.
2. `internal/utility/store/builtins.go` — add `{"builtin-code-review", "code-review", "Review the task's changed files and return anchored findings", "code-review"}`.
3. `internal/review/resolver.go` — `Resolve(ctx, ResolveRequest{AgentProfileID}) (agentID, model string, err error)` with the precedence in the plan; `ErrReviewAgentUnavailable` for no-agent, no-model, and passthrough-profile cases, each with a distinct message.
4. `internal/review/batch.go` — `PlanBatches(files []ChangedFile, budgetBytes int) (batches [][]ChangedFile, skipped []ChangedFile)`; never splits a file; `reviewPromptBudgetBytes = 120_000`.
5. `internal/review/parse.go` — `ParseFindings(response string) (ParseResult, error)`; accepts a fenced block or a bare object, tolerates surrounding prose, counts and drops individually-malformed entries, returns `ErrUnparseableResponse` when no array is recoverable.
6. `internal/review/runner.go` — `Runner.Run(ctx, RunRequest{TaskID, SessionID, RepositoryID, AgentProfileID, Trigger, WorkflowStepID}) (*models.TaskReviewRun, error)`:
   - in-memory `map[string]string` in-flight guard keyed by task id; a second request returns the in-flight run untouched.
   - `review_no_changes` before any run row is created.
   - create run → `MarkRunRunning` → per batch resolve the template and execute (session executor when the task has a live session, else host executor) → accumulate findings, tokens, duration → one `PublishFindings` → `CompleteRun` with `finding_count`, `file_count`, `repository_count`, and a summary that names skipped files and rejected entries.
   - every failure path calls `FailRun` with the spec's error code and returns it.
   - `Run` is safe to call from a goroutine; the WS handler returns the `pending` run immediately and the run proceeds detached with its own cancellable context owned by the `Runner` (`Start`/`Stop`, `sync.WaitGroup`, goleak-clean per `apps/backend/AGENTS.md`).
7. `internal/backendapp/` — provide the `Runner` and wire its dependencies.

## Acceptance

- Happy path: fake executor returning a fenced JSON block produces the expected findings and a `completed` run.
- `review_no_changes`, `review_agent_unavailable` (missing agent, missing model, passthrough profile), `review_workspace_unavailable`, `review_unparseable_response` each produce the documented outcome.
- A response with one malformed entry still completes and reports the rejected count.
- A second concurrent run request for the same task returns the first run.
- `go test -race` is goroutine-leak clean for the package.

## Verification

```
cd apps/backend && go test -race ./internal/review/...
cd apps/backend && go test ./internal/utility/...
```

## Files likely touched

`config/utilityagents/code-review.md`, `internal/utility/store/builtins.go`, `internal/review/{resolver.go,batch.go,parse.go,runner.go}` plus tests, `internal/backendapp/services.go`.

## Output contract

Summary, files changed, tests run with results, blockers, risks, `status: done`, plan checkbox.
