---
id: "02-reconcile-stale-walkthrough-links"
title: "Reconcile stale walkthrough links"
status: done
wave: 2
depends_on:
  - "01-race-safe-description-writes"
plan: "plan.md"
requirements:
  - REQ-UI-PR-WALKTHROUGH-001
acceptance_criteria:
  - AC-UI-PR-WALKTHROUGH-001.9
  - AC-UI-PR-WALKTHROUGH-001.10
  - AC-UI-PR-WALKTHROUGH-001.11
system_design:
  - ../../specs/ui/system-design/pr-walkthrough.md
---

# Task 02: Reconcile stale walkthrough links

## Summary

Add a trusted, non-generating reconciliation path for pull request description
edits. When an existing walkthrough callout contains a legacy or stale URL,
the path will repair only that marker block after confirming that the current
canonical short-SHA object is publicly available.

## In scope

- Handle `pull_request_target` `edited` events without starting the walkthrough
  generation agent.
- Reuse the trusted walkthrough helper and the race-safe description-write
  protocol from Task 01.
- Require a public HTML response for the current canonical URL before writing.
- Treat missing markers as a no-op and malformed or duplicate markers as
  fail-closed conditions.
- Add focused helper and workflow-contract tests for repair and no-op paths.

## Out of scope

- Re-running walkthrough generation because a description was edited.
- Recreating a callout that a user deliberately removed.
- Changing contributor authorization, preview deployment, or R2 retention.
- Adding a redirect or alias at the walkthrough hosting layer.

## Acceptance

- An edited covered pull request with an existing legacy full-SHA walkthrough
  block and an available current short-SHA object is repaired to the canonical
  URL while preserving all other body content.
- An edit with no walkthrough block or with an unavailable current object does
  not issue a body PATCH; malformed ownership markers fail closed.
- A repair that changes the body is read back and verified, and a canonical
  body produces a no-op without a repair loop.

## Verification

```bash
python3 scripts/pr-walkthrough-pr-body.test.py
python3 .github/scripts/pr-walkthrough-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
python3 .github/scripts/lint-action-pinning.py
actionlint .github/workflows/pr-walkthrough.yml .github/workflows/pr-walkthrough-reconcile.yml
git diff --check
```

## Files likely touched

- `.github/workflows/pr-walkthrough.yml` or a dedicated
  `.github/workflows/pr-walkthrough-reconcile.yml`
- `.github/scripts/pr-walkthrough-workflow-contract_test.py`
- `scripts/pr-walkthrough-pr-body`
- `scripts/pr-walkthrough-pr-body.test.py`
- `.github/workflows/lint-action-pinning.yml` if a new contract test command is
  required

## Dependencies

Task 01 must establish the description-write protocol and shared concurrency
group before reconciliation is enabled.

## Risks

- GitHub can emit an `edited` event for more than a body-only edit. The
  reconciliation path must remain a no-op unless it finds an existing owned
  block that needs repair.
- A newly pushed head may not have a published object yet. The path must skip
  that state instead of linking an unvalidated URL.

## Parallelism

`sequential`

## Inputs

- [PR walkthrough requirements](../../specs/ui/requirements/pr-walkthrough.md)
- [PR walkthrough system design](../../specs/ui/system-design/pr-walkthrough.md)
- [Description integrity ADR](../../decisions/2026-09-05-pr-walkthrough-description-integrity.md)
- Task 01's completed body-write implementation and tests.

## Results

Implemented the dedicated `pull_request_target` `edited` reconciliation
workflow. It checks the current canonical short-SHA URL as non-empty HTML,
uses the trusted marker helper in required-existing mode, and applies the
bounded fresh-read, compare, PATCH, and readback protocol. It skips missing
objects and missing callouts, and fails closed on malformed markers without a
body PATCH.

Verification passed:

- `python3 scripts/pr-walkthrough-pr-body.test.py` (9 tests)
- `python3 .github/scripts/pr-walkthrough-workflow-contract_test.py` (27 tests)
- `python3 .github/scripts/lint-action-pinning_test.py` (9 tests)
- `python3 .github/scripts/lint-action-pinning.py` (22 workflows)
- `mise x actionlint@1.7.12 -- actionlint .github/workflows/pr-walkthrough.yml .github/workflows/preview-env.yml .github/workflows/pr-walkthrough-reconcile.yml`
- `git diff --check`
