---
id: "08-frontend-state-and-types"
title: "Frontend review types, API client, store slice, WS handlers, helpers"
status: done
wave: 5
depends_on: ["06-ws-actions-and-dtos"]
plan: "plan.md"
spec: "../../specs/agents/requirements/native-code-review.md"
---

# Task 08: Frontend review types, API client, store slice, WS handlers, helpers

All non-visual frontend plumbing, so the UI tasks are pure rendering.

## Inputs

- Spec **API surface** and **Data model** for field names; the DTOs from task 06 are authoritative.
- `apps/web/AGENTS.md` → Data Flow Pattern (never fetch in components), store slice layout, code-quality limits.
- Patterns: `lib/state/slices/session/session-slice.ts` walkthrough handling, `lib/ws/handlers/walkthroughs.ts`, `lib/api/domains/walkthrough-api.ts`.
- `components/review/types.ts` `reviewFileKey` / `splitReviewFileKey` and `lib/utils/hash.ts` `djb2Hash` — findings must key and hash identically.

## Work

1. `lib/types/http.ts` — `TaskReviewRun`, `TaskReviewFinding`, `ReviewSeverity`, `ReviewFindingStatus`, `ReviewRunStatus`.
2. `lib/api/domains/review-api.ts` — `runTaskReview`, `cancelTaskReview`, `getTaskReview`, `updateReviewFindingStatus`, `clearTaskReview`.
3. `lib/state/slices/review/{review-slice.ts,types.ts,index.ts}` — state `{ runsByTaskId, findingsByTaskId, activeRunIdByTaskId }`; actions `setTaskReview`, `upsertReviewRun`, `addReviewFindings` (replacing by id), `updateReviewFinding`, `clearTaskReviewState`. Register in `lib/state/store.ts` and `lib/state/default-state.ts`.
4. `lib/ws/handlers/review.ts` + `lib/ws/router.ts` — handlers for `task.review.run_updated`, `task.review.findings_published`, `task.review.finding_updated`, `task.review.cleared`.
5. `lib/review/findings.ts` — pure helpers:
   - `findingFileKey(finding)` → `reviewFileKey({ path: finding.file_path, repository_name: finding.repository_name })`.
   - `isFindingStale(finding, currentDiffHash)` → `!!finding.file_diff_hash && !!currentDiffHash && finding.file_diff_hash !== currentDiffHash`.
   - `reanchorFinding(finding, diff)` — best-effort: when stale and `anchor_text` is non-empty, locate it among the diff's added lines and return the new line range; otherwise `null`.
   - `groupFindingsByFile`, `groupFindingsByRepository`, `openFindingCount`, `severityRank` (`blocker < major < minor < nit`), `sortFindings` (severity then file then line).
6. `hooks/domains/review/use-task-review.ts` — subscribes to the review WS actions and backfills via `getTaskReview` on mount, exactly as the walkthrough store is backfilled.

## Acceptance

- Slice reducers are pure and idempotent; re-publishing the same finding id does not duplicate.
- `isFindingStale` returns false when either hash is missing (an agent-published finding without a hash is never spuriously stale).
- `reanchorFinding` returns the relocated range for a moved block and `null` when the anchor text is gone.

## Verification

```
cd apps/web && pnpm vitest run lib/review/findings.test.ts lib/state/slices/review/review-slice.test.ts lib/ws/handlers/review.test.ts
cd apps/web && pnpm run typecheck
```

## Files likely touched

`lib/types/http.ts`, `lib/api/domains/review-api.ts`, `lib/state/slices/review/*`, `lib/state/{store.ts,default-state.ts}`, `lib/ws/{router.ts,handlers/review.ts}`, `lib/review/findings.ts`, `hooks/domains/review/use-task-review.ts`, plus `*.test.ts` for each logic module.

## Output contract

Summary, files changed, tests run with results, blockers, risks, `status: done`, plan checkbox.
