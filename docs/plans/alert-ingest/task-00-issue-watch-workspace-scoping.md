# T00: Scope ListAllIssueWatches to the caller's workspaces in Sentry and GitLab

## Outcome

`ListAllIssueWatches` filters to the caller's workspaces in the Sentry and
GitLab integrations, matching the behavior Jira, Linear and Slack already have.

This is a pre-existing defect that this initiative surfaced rather than created.
It is listed first because it ships on its own and because T08 must not carry
the gap forward into the shared store.

## In scope

- Apply the same per-user workspace filter used by Jira, Linear and Slack to the
  Sentry and GitLab `ListAllIssueWatches` paths.
- Preserve the identity-less internal caller behavior: a nil authorizer still
  sees all rows, so the pollers keep working.
- Tests proving a caller sees only rows from workspaces it can access, and that
  a nil authorizer is unrestricted.

## Exclusions

- No change to any other integration.
- No change to the watch schema.
- No part of the alert ingest framework.

## Applicable specifications

- `REQ-INTEGRATIONS-ALERT-INGEST-003`, `AC-INTEGRATIONS-ALERT-INGEST-003.6`
- [Alert ingest system design](../../specs/integrations/system-design/alert-ingest.md),
  Security section.

## Implementation acceptance conditions

1. A caller with a user identity listing all issue watches receives only rows
   whose workspace that caller can access, in both Sentry and GitLab.
2. A caller with no identity (the poller) receives all rows, unchanged.
3. A denial surfaces as workspace-not-found and maps to 404, with no existence
   leak, matching the existing convention.

## Verification

    make -C apps/backend test
    make -C apps/backend lint
    cd apps/backend && go test ./internal/sentry/... ./internal/gitlab/... -count=1

## Likely files

- `apps/backend/internal/sentry/service_issue_watch.go`
- `apps/backend/internal/sentry/store_issue_watch.go`
- `apps/backend/internal/gitlab/` watch service and store
- `apps/backend/internal/backendapp/helpers.go` for authorizer wiring
- Reference implementation: the equivalent filter in `internal/jira` and
  `internal/linear`

## Dependencies

None.

## Results

Not started.
