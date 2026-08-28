---
id: "01-public-provider-resolution"
title: "Public provider resolution"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/multi-branch.md"
---

# Task 01: Public provider resolution

## Acceptance

- Public GitHub repository branches plus PR/issue metadata resolve without a configured GitHub client.
- Public `gitlab.com` branches resolve for an unconfigured workspace without enabling anonymous browse/write routes.
- Authenticated-client and upstream 404/403 failures remain authoritative and preserve their HTTP status.

## Verification

- `cd apps/backend && go test -run 'Test(ListRepoBranches|GetPR|GetIssue|HttpListProjectBranches|PATClient)' ./internal/github ./internal/gitlab`

## Files likely touched

- `apps/backend/internal/github/service_pr.go`
- `apps/backend/internal/github/service_reviews.go` or a focused anonymous-read sibling
- `apps/backend/internal/github/service_test.go`
- `apps/backend/internal/gitlab/controller_watches.go`
- `apps/backend/internal/gitlab/pat_client.go`
- `apps/backend/internal/gitlab/controller_test.go`
- `apps/backend/internal/gitlab/pat_client_test.go`

## Dependencies

None.

## Inputs

- `docs/specs/tasks/requirements/multi-branch.md` — Remote task-creation scenarios.
- `docs/specs/integrations/requirements/gitlab-integration.md` — task-creation anonymous exception.
- Existing `listRepoBranchesAnonymous` GitHub implementation and GitLab workspace-client resolution.

## Output contract

Report the changed entry points, exact tests/results, risk tags, and uncertainties; update this task to `done` only after targeted verification passes.

## Completion evidence

- **Entry points:** GitHub anonymous branch, PR, and issue reads; GitLab public `gitlab.com` branch resolution.
- **Result:** backend focused verification passed: 75 tests.
- **Risks/uncertainties:** anonymous access remains limited to public read paths; authenticated clients and upstream 403/404 responses remain authoritative.
