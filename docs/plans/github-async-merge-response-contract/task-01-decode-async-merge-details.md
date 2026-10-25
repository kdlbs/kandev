---
id: "01-decode-async-merge-details"
title: "Decode async merge details"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.3
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.13
system_design:
  - ../../specs/integrations/system-design/github-pr-merge-queue.md
---

# Task 01: Decode Async Merge Details

## Summary

Decode GitHub asynchronous merge details in the shared response model. Use
the nested UUID for polling and the nested message for terminal errors.

## In scope

- Add a failing nested-response regression test for each GitHub client.
- Model the documented `details.uuid` and `details.message` fields.
- Poll nested pending responses to a terminal merged or queued outcome.
- Handle a nested pending response from an existing-request conflict.
- Report the nested message from a terminal failed response.

## Out of scope

- GitHub merge eligibility and queue policy.
- Automation retry cadence and deduplication.
- Frontend or persistence changes.

## Acceptance

- Both clients poll the UUID from the provider `details` object.
- Both clients return the existing normalized terminal outcomes.
- A terminal failed result includes the provider message in the error.

## Verification

```bash
# Run from apps/backend.
go test -tags fts5 ./internal/github -run 'Test(GHClient|PATClient)_MergePR'

# Run from the repository root.
python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `apps/backend/internal/github/client.go`
- `apps/backend/internal/github/gh_client.go`
- `apps/backend/internal/github/pat_client.go`
- `apps/backend/internal/github/gh_client_commands_test.go`
- `apps/backend/internal/github/pat_client_writes_test.go`
- `docs/specs/integrations/system-design/github-pr-merge-queue.md`

## Dependencies

None.

## Risks

- The `gh` conflict response contains JSON and CLI error text.
- The PAT client receives the same response through an HTTP error body.

## Parallelism

`sequential`

## Inputs

- GitHub PR merge-queue requirement and system design.
- Existing asynchronous merge clients and response tests.
- GitHub's documented `status` and nested `details` response object.

## Results

- Added provider-shaped nested pending, conflict, merged, queued, and failed
  response fixtures for both GitHub clients.
- The RED test run failed all six focused cases for the expected missing UUID
  and message behavior.
- Added the shared `details` response model. Both clients now poll
  `details.uuid` and report `details.message`.
- `go test -tags fts5 ./internal/github -run 'Test(GHClient|PATClient)_MergePR'`
  passed 21 tests.
- `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` passed. The targeted `gofmt -l` command returned no
  files.
