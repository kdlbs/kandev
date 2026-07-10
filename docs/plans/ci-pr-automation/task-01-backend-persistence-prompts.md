---
id: "01-backend-persistence-prompts"
title: "Backend persistence and prompts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/ci-pr-automation.md"
---

# Task 01: Backend Persistence and Prompts

## Acceptance

- `github_task_ci_options` and `github_task_ci_pr_state` are created and migrated idempotently.
- GitHub store methods return disabled defaults, persist partial option updates, and record per-PR fix/merge/error state.
- Built-in prompt `ci-auto-fix` is seeded and resolvable with embedded fallback.

## Verification

```bash
rtk make -C apps/backend test
```

Optionally run focused tests once named:

```bash
cd apps/backend && rtk go test ./internal/github ./internal/prompts/...
```

## Files Likely Touched

- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/store_ci_automation_test.go`
- `apps/backend/config/prompts/ci-auto-fix.md`
- `apps/backend/config/prompts/embed.go`
- `apps/backend/internal/prompts/store/sqlite.go`
- `apps/backend/internal/prompts/service/service.go`
- Prompt tests under `apps/backend/internal/prompts/`

## Dependencies

None.

## Inputs

- Spec sections: Data model, Persistence guarantees, Failure modes.
- Plan sections: Backend > GitHub persistence and models; Backend > Default prompt.
- Existing patterns: `github_task_prs` schema and built-in prompt seeding in `apps/backend/internal/prompts/store/sqlite.go`.

## Output Contract

When complete, update this file's `status` to `done`, update the Wave 1 checkbox in `plan.md`, and report changed files, tests run, blockers, and residual risks.
