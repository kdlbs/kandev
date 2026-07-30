---
id: "01-store-last-error"
title: "Persist last_error fields on plugin Record"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/plugins/spec.md"
---

# Task 01: Persist last_error fields on plugin Record

## Acceptance

1. `store.Record` has `LastError string` and `LastErrorAt *time.Time` with yaml/json tags `last_error` / `last_error_at` (omitempty).
2. FSStore Save → Get round-trips both fields on a real temp dir.
3. Existing store tests still pass; empty/omitted values decode as zero/nil.

## Verification

```bash
cd apps/backend && go test ./internal/plugins/store/... -count=1
```

## Files likely touched

- `apps/backend/internal/plugins/store/store.go`
- `apps/backend/internal/plugins/store/fs_store_test.go`

## Dependencies

None.

## Parallelism

sequential (wave 1 foundation).

## Inputs

- Spec: Data model runtime fields, Persistence guarantees, Last failure reason.
- Plan: Backend → Store / record.
- Pattern: `RestartCount` on the same struct.

## Implementation notes

- TDD: add round-trip test first (red), then fields (green).
- Do not change StartActivePlugins or UI in this task.
- Registry SetStatus only mutates Status — new fields live on the same Record pointer/copy path; no separate DB migration (YAML).

## Output contract

- Summary of field additions + test names
- Files changed
- Test command + pass/fail
- Update this task `status: done` and plan checkbox when complete
- Blockers / risks
