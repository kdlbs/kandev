---
id: "03-frontend-types-form"
title: "Frontend: types, form state, and payload"
status: pending
wave: 2
depends_on: ["02-backend-service-source"]
plan: "plan.md"
spec: "../../specs/linear-watcher-multiple-repositories/spec.md"
---

# Task 03: Frontend — types, form state, and payload

## Acceptance

- `apps/web/lib/types/linear.ts` defines `LinearWatchRepositoryBinding { repositoryId: string; baseBranch: string }`; `LinearIssueWatch`, `CreateLinearIssueWatchInput`, and `UpdateLinearIssueWatchInput` carry `repositories?: LinearWatchRepositoryBinding[]` (legacy `repositoryId`/`baseBranch` stay on `LinearIssueWatch` for read-compat).
- `FormState` in `linear-issue-watch-form.ts` replaces `repositoryId`/`baseBranch` with `repositories: LinearWatchRepositoryBinding[]`; `formStateFromWatch` maps plural-first (`w.repositories`), falling back to the legacy singular fields; `buildWatchPayload` emits `repositories: form.repositories.filter(r => r.repositoryId)` (half-filled rows dropped) and no longer emits the singular pair.
- `clearWorkspaceScopedForm` in `apps/web/lib/watcher-repository-default.ts` clears `repositories` when the shape has it, without breaking the Jira/Sentry singular-only callers (guard the spread on field presence); its doc comment is updated.
- Unit tests cover: plural round-trip, legacy-singular fallback, empty ⇒ `[]`, empty-row filtering, and workspace-switch clearing.

## Verification

```bash
cd apps && pnpm --filter @kandev/web typecheck
cd apps && pnpm --filter @kandev/web test -- components/linear/linear-issue-watch-form.test.ts lib/watcher-repository-default.test.ts
```

## Files likely touched

- `apps/web/lib/types/linear.ts`
- `apps/web/components/linear/linear-issue-watch-form.ts`
- `apps/web/components/linear/linear-issue-watch-form.test.ts`
- `apps/web/lib/watcher-repository-default.ts`
- `apps/web/lib/watcher-repository-default.test.ts`
- `apps/web/lib/state/slices/linear/linear-slice.test.ts` (fixtures, if they construct watches with singular fields)

## Dependencies

Task 02 (wire contract). Consumed by task 04 (picker + dialog wiring).

## Inputs

- Spec: `API surface` (request/response shapes), `Data model`.
- Plan: `Design > Frontend types & API client`, `Frontend form`.
- Existing patterns: `sentry-issue-watch-form.ts` `formStateFromWatch` / `buildWatchPayload`; `watcher-repository-default.ts` sentinels (still used by the branch select in task 04).

## Risks

- The shared `clearWorkspaceScopedForm` helper is used by three dialogs; changing its generic constraint can break Jira/Sentry compile — verify with a full web typecheck, not just the touched tests.
- `formStateFromWatch` must prefer plural so a watch saved by the new UI never round-trips through the legacy fallback.

## Output contract

Report type/form/helper changes and exact test + typecheck results; mark this task `done` and update its checkbox in `plan.md`.
