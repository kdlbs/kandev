---
id: "01-bind-merge-head"
title: "Bind GitLab merge to source head"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITLAB-INTEGRATION-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.24
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.25
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.26
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.27
system_design:
  - ../../specs/integrations/system-design/gitlab-integration-02.md
---

# Task 01: Bind GitLab merge to source head

## Summary

Restore the existing workspace-scoped merge on GitLab hosts requiring SHA by
binding each provider merge write to the freshly read source head. Keep the
provider's mismatch failure and current action, credential, and policy paths.

## In scope

- Write an independent disposable-server regression through the registered
  workspace route, real config/secret resolution, service and PAT client. Show
  the required-SHA case fails for missing SHA before the production fix and
  demonstrate legacy and exact-SHA provider controls. Never use the private
  qualification fixture.
- Read top-level MR `sha` at the PAT provider boundary, validate nonblank,
  and send it unchanged. Cover divergent diff head/base, fork identity,
  unavailable/empty/whitespace SHA and no write, provider 409 and no retry,
  scope, method, squash and message. Verify inherited GLab behavior and direct
  service/automation coverage; adjust existing PAT tests for the new read.
- Preserve current failure mapping and all existing payload behavior, including
  no requested source-branch deletion. Do not alter general MR projection.

## Out of scope

UI, browser changes, reviewed-head claims, admin, credential changes, source
branch deletion, unrelated CI/filter/pagination behavior and squash defaults.
No live GitLab or external credentials.

## Acceptance

- The independent real-route test shows a meaningful missing-SHA RED and then
  passes with required-SHA, legacy, exact-SHA and identity/options controls.
- Provider-level tests demonstrate top-level source head selection, fail-closed
  reads, inherited GLab use, 409 propagation and exactly one write without
  retry; existing service and automation behavior remains intact.
- Targeted Go, race, changed-Go lint, spec and documentation checks pass and
  the work order and plan record actual results.

## Verification

Run only after ROOT separately releases implementation and the heavy lease.
Run from repository root. Use pinned toolchain PATH and a fresh, unique
`KANDEV_RUN_QUIET_DIR` per run; retain process handles and exit evidence.
Before production edits, record the RED output of the named new route test and
its provider controls (the required case must fail because SHA is absent):

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -p=1 -count=1 -v ./internal/gitlab -run '^TestWorkspaceMergeMRRequiredSHA$')
```

Then run after the fix:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -p=1 -count=1 ./internal/gitlab)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -p=1 -race -count=1 ./internal/gitlab)
GOMAXPROCS=2 GOMEMLIMIT=3GiB bash scripts/lint-go-changed apps/backend/internal/gitlab/pat_client_actions.go apps/backend/internal/gitlab/pat_client_search_actions_test.go apps/backend/internal/gitlab/merge_head_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/gitlab-required-merge-sha
```

Name the new owning test `merge_head_test.go` so the lint list covers it; add
any other changed Go files to the same canonical lint invocation. Run the
repository PR-documentation coverage preflight when available. Install the
workspace's frozen pnpm dependencies once before pnpm or hooks in a fresh
worktree; no install or heavy check is authorized in the design phase.

## Files likely touched

- `apps/backend/internal/gitlab/pat_client_actions.go`
- `apps/backend/internal/gitlab/pat_client_search_actions_test.go`
- `apps/backend/internal/gitlab/merge_head_test.go`
- The linked requirement, system design and plan documents for lifecycle/results.

## Dependencies

None.

## Risks

GitLab may return a stale source head and correctly reject it at write time;
never substitute a diff/base SHA or retry silently. Provider 409 may be mapped
by the existing Kandev route; preserve the provider error and route failure.

## Parallelism

`sequential`

## Inputs

- AC .24-.27 and the linked system design.
- Current provider, registered controller and workspace credential tests.
- GitLab's documented single-MR and merge API contracts.

## Results

- Independent registered-route RED: required-SHA provider rejected the absent
  SHA and the route returned 502; legacy and exact-SHA controls passed.
- Registered-route GREEN passed after the fix; required and legacy cases both
  passed. Full `./internal/gitlab` package and `-race` passed with the command
  and resource limits above. Disposable server tests cover the stated matrix.
- Canonical changed-Go lint passed with `GOMEMLIMIT=3GiB` after correcting two
  test-only lint findings. Catalog, specification lint, documentation coverage
  preflight and diff checks passed as recorded in the plan.
