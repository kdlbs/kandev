---
id: "01-race-safe-description-writes"
title: "Make PR description writes race-safe"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-PR-WALKTHROUGH-001
acceptance_criteria:
  - AC-UI-PR-WALKTHROUGH-001.9
  - AC-UI-PR-WALKTHROUGH-001.10
system_design:
  - ../../specs/ui/system-design/pr-walkthrough.md
---

# Task 01: Make PR description writes race-safe

## Summary

Harden the walkthrough and preview PR-description writers against stale
full-body updates. Each writer will merge its marker-owned section from a
fresh body, retry when another writer changes the body, and verify its result
after the PATCH.

## In scope

- Keep the walkthrough link job bound to the publication job's validated
  12-character URL.
- Add the shared per-PR description concurrency group to the walkthrough link
  and short trusted preview-description writers.
- Add fresh-snapshot comparison, bounded retry, marker preservation, and
  post-write readback to the Python and Go write paths.
- Add canonical URL and concurrent body-update regression tests.

## Out of scope

- The edited-event reconciliation job.
- Changing the walkthrough generation agent, renderer, or R2 lifecycle.
- Renaming existing full-SHA objects or changing external GitHub Apps.

## Acceptance

- A current walkthrough writer can only produce the validated URL using the
  first 12 lowercase SHA characters; a legacy full-SHA URL is replaced inside
  the owned marker block.
- Concurrent walkthrough and preview body updates preserve contributor text
  and both marker-owned sections, retry from a fresh body, and do not report
  success until the writer's owned result is readable.
- A writer that cannot converge after the bounded retry limit fails without
  sending a stale merged body.

## Verification

```bash
python3 scripts/pr-walkthrough-pr-body.test.py
cd apps/backend && go test ./cmd/preview
cd ../.. && python3 .github/scripts/pr-walkthrough-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
python3 .github/scripts/lint-action-pinning.py
actionlint .github/workflows/pr-walkthrough.yml .github/workflows/preview-env.yml
git diff --check
```

## Files likely touched

- `.github/workflows/pr-walkthrough.yml`
- `.github/workflows/preview-env.yml`
- `.github/scripts/pr-walkthrough-workflow-contract_test.py`
- `scripts/pr-walkthrough-pr-body`
- `scripts/pr-walkthrough-pr-body.test.py`
- `apps/backend/cmd/preview/github.go`
- `apps/backend/cmd/preview/github_test.go` or a focused sibling test

## Dependencies

None.

## Risks

- The GitHub REST API does not provide a relied-upon conditional body write,
  so a writer can still lose a race with an external integration after its
  final read. Readback and retry must make this visible and recoverable.
- The preview command is built and run from the preview checkout, so its
  implementation must preserve the existing fork authorization boundary.

## Parallelism

`sequential`

## Inputs

- [PR walkthrough requirements](../../specs/ui/requirements/pr-walkthrough.md)
- [PR walkthrough system design](../../specs/ui/system-design/pr-walkthrough.md)
- [Description integrity ADR](../../decisions/2026-09-05-pr-walkthrough-description-integrity.md)
- Existing `scripts/pr-walkthrough-pr-body` and
  `apps/backend/cmd/preview/github.go` tests.

## Results

Implemented the fresh-snapshot, bounded retry, and post-write readback
protocol for preview description updates. Walkthrough link updates now use
the same per-PR description concurrency group and no longer retry a stale
payload. Added Go regression coverage for pre-PATCH body changes, readback
loss, bounded non-convergence, marker preservation, cleanup no-ops, and orphan
end markers.

Verification passed:

- `python3 scripts/pr-walkthrough-pr-body.test.py` (9 tests)
- `go test -race ./cmd/preview` (28 tests)
- `python3 .github/scripts/pr-walkthrough-workflow-contract_test.py` (27 tests)
- `python3 .github/scripts/preview-env-workflow-contract_test.py` (3 tests)
- `python3 .github/scripts/lint-action-pinning_test.py` (9 tests)
- `python3 .github/scripts/lint-action-pinning.py` (22 workflows)
- `mise x actionlint@1.7.12 -- actionlint .github/workflows/pr-walkthrough.yml .github/workflows/preview-env.yml`
- `git diff --check`
