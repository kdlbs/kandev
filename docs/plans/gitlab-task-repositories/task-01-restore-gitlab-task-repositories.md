---
id: "01-restore-gitlab-task-repositories"
title: "Restore native GitLab task repository paths"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITLAB-INTEGRATION-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.28
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.29
system_design:
  - ../../specs/integrations/system-design/gitlab-integration-02.md
---

# Task 01: Restore native GitLab task repository paths

## Scope

Reuse the workspace origin verifier in remote task repository resolution.
Route saved GitLab repository branch reads to the first-party workspace
client, preserving origin checks and the full subgroup path. No UI, schema,
new authentication mechanism, or plugin changes are required.

## Acceptance

1. Task creation admits configured self-managed origins and retains the
   requested branch; denied or unavailable configurations leave no persisted
   task or repository. Public GitLab and trusted contributions remain valid.
2. Saved GitLab branch listing works without a plugin, returns only the
   selected workspace's branches, and rejects missing or mismatched origins.

## Verification

Run from `apps/backend`:

```bash
go test -trimpath -race ./internal/task/service ./internal/backendapp -run '^(TestCreateTaskGitLabRepositoryOrigin|TestCodeHostBranchListerGitLab|TestGitLabRepositoryOriginVerifier|TestRegisterRemoteRepositorySelfManagedGitLab|TestResolveRepositoryRef_RejectsUntrustedGitLabOrigins)$' -count=1
# Broader check; status-summary test race reported locally:
go test -trimpath -race ./internal/task/service ./internal/backendapp
```

Run from the repository root:

```bash
bash scripts/lint-go-changed apps/backend/internal/task/service/service_tasks.go apps/backend/internal/task/service/service_tasks_gitlab_origin_test.go apps/backend/internal/backendapp/services.go apps/backend/internal/backendapp/repository_gitlab_branches_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Results

Both focused regressions failed before their corresponding corrections.
Targeted race checks for the two new regressions, the existing origin verifier,
self-managed registration, and untrusted-origin rejection pass. Changed-package
lint reports zero issues. Specification catalog validation, spec lint, PR
documentation coverage, and whitespace checks pass. The full race suite reports
a race in the existing status-summary repair test, outside the corrected paths;
it is not claimed as passing. The full backendapp race suite passes. An
isolated 200-run check on the unchanged base did not reproduce the race. No
production or manual browser verification was performed.
