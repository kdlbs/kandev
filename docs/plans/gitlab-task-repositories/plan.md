---
created: 2026-10-27
status: implemented
requirements:
  - REQ-INTEGRATIONS-GITLAB-INTEGRATION-001
system_design:
  - ../../specs/integrations/system-design/gitlab-integration-02.md
legacy_specs: []
---

# GitLab task repository repair

Task creation rejects configured self-managed GitLab origins, while branch
listing for saved GitLab repositories incorrectly requires a provider plugin.
Reuse the existing workspace origin verifier and GitLab client resolver.

## Scope and risks

Restore the documented GitLab behavior without new settings, migrations,
credentials, UI changes, or plugin contracts. Preserve exact origin checks,
workspace isolation, trusted contributions, and other provider routes.

## Work orders

- [Task 01: Restore native GitLab task repository paths](task-01-restore-gitlab-task-repositories.md)

## Verification

SQLite-backed task creation must accept configured HTTP(S) origins and reject
unconfigured, mismatched, or unavailable origins without persisted rows.
The branch adapter must read the selected workspace's seeded branches and
reject missing or mismatched origins and unconfigured workspaces.
Run affected package tests with the race detector, changed-package lint, and
documentation validators. Hosted CI and manual browser verification remain
separate from these backend integration checks.

## Results

Both regressions failed before their corresponding corrections. Targeted
race tests and changed-package lint pass. Specification validation, spec lint,
and PR documentation coverage pass. The broader race suite reported a data
race in the existing `TestTaskStatusSummaryBatchRepair`, between replacement of
`svc.tasks` and its cleanup worker; this repair does not change those paths.
The full backendapp package passes. An isolated 200-run check on the unchanged
base did not reproduce the intermittent race.
Production deployment and manual browser verification were not performed.
