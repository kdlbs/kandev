---
id: "01-github-review-watch-response"
title: "Return the updated GitHub review watch"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-manual-save.md"
---

# Task 01: Return the updated GitHub review watch

## Acceptance

- `PUT /api/v1/github/watches/review/:id` returns the complete updated
  `ReviewWatch`, including its `id` and new `enabled` value.
- The route keeps its existing workspace authorization and error behavior.
- A controller regression fails on the acknowledgement-only response and passes
  on the authoritative response.

## Verification

```bash
make -C apps/backend test
```

## Files likely touched

- `apps/backend/internal/github/controller.go`
- `apps/backend/internal/github/controller_test.go`

## Dependencies

None.

## Parallelism

Sequential.

## Inputs

- `docs/specs/ui/requirements/settings-manual-save.md`
- `docs/plans/integration-watcher-save-state/plan.md`
- Existing `httpUpdateIssueWatch` response pattern

## Risks

The test must exercise the real handler/service/store path so the frontend
contract cannot drift back to an acknowledgement shape.

## Output contract

Report the red and green command results, files changed, blockers, risks, and
update this task plus `plan.md` status in the primary conversation.
