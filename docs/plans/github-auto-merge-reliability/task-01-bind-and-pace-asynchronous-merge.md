---
id: "01-bind-and-pace-asynchronous-merge"
title: "Bind and pace asynchronous merge"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.1
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.7
system_design:
  - ../../specs/integrations/system-design/github-pr-merge-queue.md
---
# Task 01: Bind and Pace Asynchronous Merge

## Summary

Carry the reviewed head SHA through the automatic merge provider call. Pace
pending status requests and keep the operation bounded by context.

## In scope

- Replace positional merge parameters with a typed request value.
- Require an expected head on the automation service path.
- Send GitHub's `sha` field through the CLI and PAT clients.
- Decode the provider expected-head diagnostic.
- Wait at least one second between pending reads.
- Add deterministic provider, service, mismatch, timeout, and cancellation tests.

## Out of scope

- Attempt persistence or retry authorization.
- Changes to manual merge eligibility or presentation.

## Acceptance

- An automatic request cannot merge a head that differs from the refreshed head.
- Pending reads occur no more than once per second.
- Cancellation and the existing deadline stop polling promptly.
- Manual merge behavior remains unchanged.

## Verification

```bash
go test -tags fts5 ./internal/github -run 'Test(GHClient|PATClient).*MergePR|TestMergePRForAutomation'
```

Run the command from `apps/backend`.

## Files likely touched

- `apps/backend/internal/github/client.go`
- `apps/backend/internal/github/gh_client.go`
- `apps/backend/internal/github/pat_client.go`
- `apps/backend/internal/github/service_pr.go`
- `apps/backend/internal/github/gh_client_commands_test.go`
- `apps/backend/internal/github/pat_client_writes_test.go`
- `apps/backend/internal/github/service_pr_auth_test.go`

## Dependencies

None.

## Risks

- Interface changes affect provider mocks and non-automation callers.
- Real waits can make tests slow. Tests need an injected wait boundary.

## Parallelism

`sequential`

## Inputs

- Integration acceptance criteria 002.1 and 002.7.
- GitHub asynchronous merge response and expected-head contracts.

## Results

- Added a typed merge request with an optional method and expected head SHA.
- Required a non-empty expected head on the automatic merge service path.
- Sent GitHub's `sha` field through the CLI and token clients.
- Added context-aware one-second pacing before pending status reads.
- Included the provider's expected head in failed-request diagnostics.
- Verification: focused GitHub tests passed, 26 tests.
- Verification: orchestrator head-forwarding regression passed, 1 test.
