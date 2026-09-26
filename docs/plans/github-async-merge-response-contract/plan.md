---
created: 2026-08-28
status: done
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001
system_design:
  - ../../specs/integrations/system-design/github-pr-merge-queue.md
legacy_specs: []
---

# Implementation Plan: GitHub Async Merge Response Contract Fix

## Overview

Correct both GitHub clients to decode the documented asynchronous merge
response. Add provider-boundary tests before the production correction.

## Scope

### In scope

- Decode `details.uuid` from pending asynchronous merge responses.
- Decode `details.message` from failed asynchronous merge responses.
- Poll accepted and existing merge requests through both GitHub clients.
- Replace flat response fixtures with the documented nested response shape.

### Out of scope

- Changes to merge eligibility or automation policy.
- Changes to merge-queue observation or persistence.
- UI, localization, database, and public documentation changes.
- Automatic merge retries after a terminal GitHub failure.

## Technical approach

Update `mergeAsyncResponse` in `apps/backend/internal/github/client.go`. Model
the provider `details` object with its UUID and message fields.

Update `GHClient.MergePR` and `PATClient.MergePR` to read the nested fields.
Keep the existing status normalization and the `gh` conflict-body extraction.

Add real response shapes to `gh_client_commands_test.go` and
`pat_client_writes_test.go`. Each client test first receives a nested pending
response. It then polls the UUID and receives a terminal result.

## Tests

- `AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.3`: Both clients poll
  `details.uuid` and return the final merged or queued outcome.
- `AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.13`: Both clients report
  `details.message` for a terminal failed result.
- The `gh` client keeps its embedded JSON extraction for a `409` response.

## E2E tests

No new Playwright test is required. Existing merge-action and CI automation
tests cover normalized outcomes. This fix changes the external provider
decoder below the mock-provider boundary.

## Work orders

- [x] [Task 01: Decode async merge details](task-01-decode-async-merge-details.md)

## Verification results

- The RED test run failed all six nested-response cases for the expected
  missing UUID and message behavior.
- `go test -tags fts5 ./internal/github -run 'Test(GHClient|PATClient)_MergePR'`
  passed 21 tests.
- `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` passed.
- The targeted `gofmt -l` command returned no files.

## Risks

- The `gh` client receives a `409` body inside CLI error text. The correction
  must preserve the existing JSON extraction.
- Both clients share the response model. A partial correction can leave one
  authentication route broken.
