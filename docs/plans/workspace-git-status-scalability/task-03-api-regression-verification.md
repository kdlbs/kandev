---
id: "03-api-regression-verification"
title: "API regression verification"
status: done
wave: 3
depends_on:
  - "02-coalesce-fresh-status"
plan: "plan.md"
spec: "../../specs/platform/workspace-git-status.md"
---

# Task 03: API regression verification

## Acceptance

- Overlapping fresh multi-repository HTTP requests perform one underlying observation per repository and return consistent repository-scoped snapshots.
- One cancelled HTTP waiter does not prevent peers from receiving the shared completed result.
- An invalid repository remains a local failure while healthy repository entries succeed.
- Focused race tests and the backend format, test, and lint suites pass.
- No frontend, payload-shape, ignore-policy, or public-documentation changes are introduced.

## Verification

```bash
cd apps/backend
rtk go test ./internal/agentctl/server/api -run 'Test.*GitStatus.*Fresh'
rtk go test -race ./internal/agentctl/server/process ./internal/agentctl/server/api
cd ../../
rtk make -C apps/backend fmt
rtk make -C apps/backend test
rtk make -C apps/backend lint
```

## Files likely touched

- `apps/backend/internal/agentctl/server/api/git_status_fresh_concurrency_test.go`
- Process-package files only if handler coverage exposes a correctness defect from Task 02.

## Dependencies

- `02-coalesce-fresh-status`

## Inputs

- Existing Git-status single and multi handlers and response types.
- Existing multi-repository partial-success tests.
- Task 02's deterministic observation-count seam or blocking Git shim.

## Output contract

Report handler scenarios, underlying observation counts, cancellation behavior, multi-repository results, race and full-suite verification, changed files, and remaining risks. Keep assertions synchronization-based rather than elapsed-time-based.
