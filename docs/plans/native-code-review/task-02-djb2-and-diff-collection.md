---
id: "02-djb2-and-diff-collection"
title: "Go djb2 hash and task changed-file collection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/native-code-review.md"
---

# Task 02: Go djb2 hash and task changed-file collection

Give the backend the two primitives a review run needs: the frontend-compatible diff hash, and the task's changed-file set.

## Inputs

- Spec **Data model** (`file_diff_hash` must match `apps/web/lib/utils/hash.ts`).
- `apps/web/components/review/review-dialog.tsx` `buildAllFiles` — the dedup/priority contract the backend must mirror (uncommitted wins over committed).
- `apps/backend/internal/agent/handlers/git_handlers.go` `computeCumulativeDiff` — how to reach agentctl for a session's diff.
- `apps/web/components/review/types.ts` `normalizeDiffContent`, `sanitizeReviewRepositoryName`.

## Work

1. `internal/utility/hash/djb2.go` — `DJB2(s string) string`. Iterate UTF-16 code units to match `String.prototype.charCodeAt`; accumulate in `int32` with wraparound; emit `strconv.FormatUint(uint64(uint32(h)), 16)`.
2. `internal/review/diff.go` (new package `internal/review`):
   - `type ChangedFile struct { RepositoryID, RepositoryName, Path, Status, Diff, DiffHash string; Additions, Deletions int }`
   - `type ChangeSource interface { GitStatus(ctx, sessionID) (*agentctl.GitStatusResult, error); CumulativeDiff(ctx, sessionID) (*process.CumulativeDiffResult, error) }` — satisfied by an adapter over the existing execution lookup so the runner is testable with a fake.
   - `CollectChanges(ctx, src, sessionID, repositoryID string) ([]ChangedFile, error)` — merge uncommitted then committed, keyed by `repositoryName + "\x00" + path`, uncommitted winning; normalize the diff the same way the frontend does before hashing; filter to `repositoryID` when non-empty; skip entries with an empty diff.

## Acceptance

- `DJB2` agrees with `djb2Hash` on the shared fixture set (ASCII, multi-byte UTF-8, empty string, a realistic diff body).
- `CollectChanges` returns uncommitted content for a path present in both sources, and scopes correctly for a multi-repository payload.

## Verification

```
cd apps/backend && go test ./internal/utility/hash/... ./internal/review/...
cd apps/web && pnpm vitest run lib/utils/hash.test.ts
```

## Files likely touched

`internal/utility/hash/{djb2.go,djb2_test.go}`, `internal/review/{diff.go,diff_test.go}`, `apps/web/lib/utils/hash.test.ts` (add the shared fixtures).

## Output contract

Summary, files changed, tests run with results, blockers, risks, `status: done`, plan checkbox.
