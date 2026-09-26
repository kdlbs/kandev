---
id: task-04-post-start-gating
title: Office post-start failure gates on profile mode
status: done
wave: 2
depends_on: [task-01-profile-fields]
plan: docs/plans/no-silent-model-fallback/plan.md
spec: docs/specs/agents/requirements/no-silent-model-fallback.md
---

# Task 4 — Post-start failure: strict escalate / fallback one-shot / legacy requeue

## Change

`HandlePostStartFailure` in
`apps/backend/internal/office/scheduler/routing_lifecycle.go` already
receives `agent *models.AgentInstance`. After `routingerr.Classify`:

- `!classified.FallbackAllowed` → unchanged (return false, escalate).
- `agent.AutoFallback` → unchanged (requeue to next candidate via
  `applyPostStartFallback`).
- `agent.FallbackModel != ""` → **one-shot retry with the fallback model**:
  - Persist the override: new nullable column `runs.fallback_model`
    (`models.Run` gains `FallbackModelOverride *string`; repo method
    `SetRunFallbackModelOverride` in
    `apps/backend/internal/office/repository/sqlite/run_routing.go`;
    migration in the runs schema file — check where office run migrations
    live, e.g. `repository/sqlite/` migrate list).
  - Requeue (existing `RequeueForNextCandidate`).
  - Next dispatch (`dispatch_routing.go`): when
    `run.FallbackModelOverride != nil`, build **one** candidate from
    `run.ResolvedExecutionProfileID` + `run.ResolvedProviderID` + the
    override, skipping the resolver (or via a `ResolveOptions` force), and
    **clear the override after the attempt** so a subsequent failure
    escalates to the terminal explicit-failure path. Counts as a normal
    attempt row (`recordAttemptStart`) — respects `MaxAttemptsPerRun`.
  - On success, `handleLaunchSuccess` records `ResolvedModel = fallback`.
- otherwise (strict) → return `(false, nil)` so the caller escalates to
  `HandleAgentFailure` — the run fails explicitly (no requeue).

**Error mapping**: helper (in `routingerr` or the scheduler) that renders an
actionable message for model/auth codes (`model_unavailable`,
`auth_required`, `missing_credentials`, `subscription_required`,
`provider_unavailable`) — e.g. `"Model unavailable: the configured model
<id> is no longer available. Change the model in the agent profile."` Used
when the strict path writes the run failure message (and reused by task 03
for session-start failures).

## Acceptance

1. Test: strict profile + fallback-allowed failure → `HandlePostStartFailure`
   returns `(false, nil)`; run reaches the terminal failure path; failure
   message contains the actionable copy.
2. Test: auto-fallback profile → existing requeue-to-next-candidate behavior
   preserved (regression).
3. Test: fallback-model profile → override persisted + requeued; next
   dispatch launches the single forced candidate (same provider,
   fallback model); the override is cleared after one attempt.
4. Test: forced candidate fails too → run fails explicitly (no second
   requeue, no resolver candidates).
5. Test: forced candidate succeeds → `ResolvedModel` = fallback; health
   scopes marked healthy.
6. Test: `MaxAttemptsPerRun` still enforced with the forced candidate.

## Verification

```sh
make -C apps/backend test ./internal/office/scheduler/...
make -C apps/backend test ./internal/office/repository/...
```

## Risks

- The forced candidate path must not bypass the attempt cap or the
  route-cycle baseline semantics.
- Find the runs-schema migration location and follow its conventions;
  existing runs get NULL override (no behavior change).
- `excludedFromAttempts` / `continuationLaunchContext` must treat the
  forced attempt like any other (exclude-set unchanged — the forced
  candidate is the only candidate, so exclusion is moot).
