---
id: "01-sprites-control-plane-retry"
title: "Retry transient preview Sprites control-plane failures"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/preview-sprites-transient-retry.md"
---

# Task 01: Retry transient preview Sprites control-plane failures

- **Acceptance:** `getOrCreateSprite` retries bounded transient get/create
  failures with observable retry output and cancellation-aware backoff.
- **Acceptance:** An ambiguous transient create is reconciled with a named
  sprite lookup before another create request; retryable API `429`/`5xx` and
  network failures are distinct from permanent client errors.
- **Acceptance:** Focused Go tests cover retry success, ambiguous-create
  reconciliation, non-retryable failure, and retry exhaustion.
- **Verification:** `cd apps/backend && go test -v ./cmd/preview`
- **Files likely touched:** `apps/backend/cmd/preview/sprite_ops.go`,
  `apps/backend/cmd/preview/sprite_ops_test.go`.
- **Dependencies:** None.
- **Parallelism:** sequential.
- **Inputs:** `docs/specs/ui/requirements/preview-sprites-transient-retry.md`,
  `docs/plans/preview-sprites-transient-retry/plan.md`, the existing upload
  retry in `sprite_ops.go`, and Sprites SDK `APIError` retry metadata.
- **Output contract:** Report the red/green test evidence, retry semantics,
  changed files, final targeted test result, risks, and task/plan status
  update.
