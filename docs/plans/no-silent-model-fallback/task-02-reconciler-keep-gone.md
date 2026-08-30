---
id: task-02-reconciler-keep-gone
title: Reconciler keeps gone start/fallback models
status: done
wave: 1
depends_on: [task-01-profile-fields]
plan: docs/plans/no-silent-model-fallback/plan.md
spec: docs/specs/agents/requirements/no-silent-model-fallback.md
---

# Task 2 — Reconciler: never heal away a gone model

## Change

`healProfile` in `apps/backend/internal/agent/settings/controller/reconciler.go`
currently replaces a gone `p.Model` with `caps.CurrentModelID` (the boot-time
silent fallback). Change:

- When `p.Model != ""` and the model is not in `caps.Models`: **keep it**.
  Log `"profile model no longer available, keeping (no silent fallback)"`
  with profile id + model; do NOT write.
- The `p.Model == "" && caps.CurrentModelID != ""` seed-default branch is
  unchanged.
- Apply the same keep-when-gone rule to the new `p.FallbackModel` (when the
  fallback model itself is gone, keep it; the UI surfaces it red).
- Mode healing is unchanged.

`modelExists` / `modeExists` helpers stay as-is.

## Acceptance

1. Test: profile with a model absent from the probe list is **not**
   overwritten after `Run()` (model unchanged; no update written).
2. Test: profile with empty model still gets seeded to `CurrentModelID`.
3. Test: profile whose `fallback_model` is absent from the probe list keeps
   it.
4. Test: probe status not "ok" still skips reconciliation entirely
   (existing behavior, guard it with a regression test).

## Verification

```sh
make -C apps/backend test ./internal/agent/settings/controller/...
```

## Risks

- The reconciler runs on every boot; the keep path must not flip-flop (it
  must not write at all, so no churn).
- Do not touch `healProfileName` (unrelated naming heuristic).
