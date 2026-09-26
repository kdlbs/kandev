---
id: "10-final-verification"
title: "Final verification and spec reconciliation"
status: done
wave: 9
depends_on: ["09-public-documentation"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/repository-sets.md"
---

# Task 10: Final Verification And Spec Reconciliation

## Acceptance

- Formatting, backend tests and lint, frontend typecheck, tests, lint, and the i18n gates all pass,
  and every command's result is recorded in the task file.
- The spec matches the shipped behavior. Any divergence found while building is reflected in
  `docs/specs/workspaces/requirements/repository-sets.md`, whose status moves from `draft` to `building`, with the same
  status in `docs/specs/INDEX.md`.
- `plan.md` records every task as done, and no task file is left `in_progress`.
- The change is committed with a Conventional Commits message (`feat: ...`), staged with explicit
  paths, after confirming `git diff --cached --name-only | grep '\.go$' | xargs gofmt -l` is empty.

## Verification

```bash
cd apps/backend && make fmt && go test ./internal/task/... ./internal/backendapp/... ./internal/events/... ./internal/gateway/websocket/... && make lint
cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm vitest run components/task-create-dialog-repository-sets.test.ts lib/state/slices/workspace hooks/domains/workspace app/settings/workspace
```

## Files likely touched

- `docs/specs/workspaces/requirements/repository-sets.md`, `docs/specs/INDEX.md`
- `docs/plans/repository-sets/plan.md` and the task files
- `CLAUDE.md` or a scoped `AGENTS.md`, only if a convention documented there changed

## Dependencies

All prior tasks.

## Inputs

- Root `CLAUDE.md`: commit conventions, the 100-character commitlint header cap, and the pre-commit
  prettier/gofmt hooks that fail the commit when they reformat staged files (re-stage and make a new
  commit rather than amending).
- Do not run `/simplify`, `/qa`, `/code-review`, security review, or a broad `/verify` here; the PR AI
  reviewers are the semantic-review gate.

## Risks

- The i18n key checker fails on a missing key, an extra key, a dropped placeholder, or a value left
  identical to English in any of the five locales, so it is the most likely late failure.
- A pre-commit hook reformatting staged files fails the commit; expect one re-stage cycle.

## Output contract

Summary, files changed, every command and its result, blockers, risks, divergence from the plan, and
final spec/plan/task status updates.
