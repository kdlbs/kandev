---
id: "01-provider-contribution-resolution"
title: "Provider contribution resolution"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/system-design/remote-contribution-tasks.md"
---

# Task 01: Provider Contribution Resolution

## Acceptance

- A versioned provider-neutral contribution binding validates and round-trips canonical, non-secret
  change/source identity, branches, SHA, and collaboration state without provider title/body content.
- GitHub PAT and `gh` clients resolve same-repository and fork PRs with source repository clone identity
  and `maintainer_can_modify`; GitLab resolves target/source projects and `allow_collaboration` on the
  configured host.
- Both providers reject malformed URLs, inconsistent returned identity, non-open changes, missing heads,
  unsafe refs, and non-editable forks before returning a binding.

## Verification

```bash
cd apps/backend
rtk go test ./internal/task/models ./internal/github ./internal/gitlab -run 'Test.*(RemoteContribution|Contribution|Maintainer|Collaboration|SourceProject)'
```

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- focused task model validation tests
- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/pat_client.go`
- `apps/backend/internal/github/gh_client.go`
- `apps/backend/internal/github/service_pr.go` or a focused contribution service file
- GitHub client/service tests beside those files
- `apps/backend/internal/gitlab/models.go`
- `apps/backend/internal/gitlab/client_helpers.go`
- `apps/backend/internal/gitlab/service_task_mr_link.go` or a focused contribution service file
- GitLab client/service tests beside those files

## Dependencies

None.

## Parallelism

Sequential foundation. Later tasks consume the exact binding and provider resolver contract.

## Inputs

- Spec: **Data model**, **Permissions**, provider rejection scenarios.
- ADR: backend-owned resolution and versioned binding.
- Existing patterns: GitHub `GetPRForWorkspace`, GitLab `AssociateExistingMRByURL`, shared branch-name
  validation, and provider remote canonicalization.

## Risks

- GitHub CLI and PAT conversions must expose identical source/collaboration semantics.
- GitLab IID is project-scoped; never resolve or persist it without the target project path.
- Provider response text and credential-bearing clone URLs must not enter the binding or logs.

## Output contract

Report the contract implemented, files changed, exact test results, blockers/risks, divergence from the
plan, and update this task plus `plan.md` status.

## Completion

Implemented the versioned credential-free binding, strict canonical URL/ref/SHA validation, and
workspace-scoped GitHub PAT/CLI plus configured-host GitLab resolvers. Provider conversions now retain
source/target identity and collaboration eligibility while excluding provider-authored content.

The affected provider/model package suite passed as part of the 17-package backend run: 5,603 tests
passed. The provider tests cover canonical URL rejection, identity mismatch, missing repository data,
non-editable forks, and title/body non-persistence. No real provider credentials or network calls are
used; provider client behavior is exercised through hermetic service/domain fixtures.
