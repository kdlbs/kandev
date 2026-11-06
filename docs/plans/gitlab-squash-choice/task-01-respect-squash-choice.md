---
id: "01-respect-squash-choice"
title: "Respect GitLab squash choice"
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
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.28
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.29
  - AC-INTEGRATIONS-GITLAB-INTEGRATION-001.30
system_design:
  - ../../specs/integrations/system-design/gitlab-integration-02.md
---

# Task 01: Respect GitLab squash choice

## Summary

Preserve GitLab's persisted MR squash choice for default interactive and
automated merges, while retaining explicit method validation and the existing
workspace and source-head safeguards. Independently establish causal RED and
controls against a local HTTP provider, then make the minimal service change.

## In scope

- Add real service/PAT HTTP request and modeled outcome coverage described in
  the plan, including registered interactive route and automation wiring.
- Correct the empty-method decision and misleading automation comment and test;
  keep existing SHA compatibility code and tests intact.
- Update `docs/public/integrations.md` review/automation guidance to explain
  that default merge respects the MR choice subject to project policy.

## Out of scope

Frontend changes, new configuration or API, live provider writes, adjacent
GitLab repairs, retries, and changing the independent source-head repair.
Do not read, copy or run the protected qualification candidate or its logs.

## Acceptance

- The independently authored regression fails before the fix on both unchecked
  default-policy cases for the actual outgoing forced squash, while controls
  pass; after the fix all listed choice and validation cases pass.
- Request and flow tests retain workspace, host, credential, full project/IID,
  message, source-branch and same-head SHA semantics; mismatch fails with one
  PUT and no retry. Existing missing/blank-head safeguards remain covered.
- Update public guidance and synchronize this order and manifest with actual
  results. No UI or browser proof is claimed.

## Verification

Run only after explicit implementation direction and exclusive HEAVY grant;
retain and join each actual command handle before starting the next. From repo
root, pin pnpm before Node and Go, and install once in this fresh worktree:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH"
pnpm --version
node --version
go version
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/loop-evidence/61dadc35-4a42-4581-8c94-62309c160fd9/child029/install scripts/run-quiet install -- bash -c 'cd apps && exec pnpm install --frozen-lockfile'
```

After writing the independent test and before production edits, run RED and
record the two causal failures and passing controls from its log. After the fix,
run GREEN and the owning package sequentially:

```bash
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/loop-evidence/61dadc35-4a42-4581-8c94-62309c160fd9/child029/red GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB scripts/run-quiet backend-test -- bash -c 'cd apps/backend && exec go test -v -race -trimpath -tags=fts5 -p 1 ./internal/gitlab -run "^TestGitLabSquashChoice$" -count=1'
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/loop-evidence/61dadc35-4a42-4581-8c94-62309c160fd9/child029/green GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB scripts/run-quiet backend-test -- bash -c 'cd apps/backend && exec go test -v -race -trimpath -tags=fts5 -p 1 ./internal/gitlab -run "^TestGitLabSquashChoice$" -count=1'
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/loop-evidence/61dadc35-4a42-4581-8c94-62309c160fd9/child029/package GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB scripts/run-quiet backend-test -- bash -c 'cd apps/backend && exec go test -race -trimpath -tags=fts5 -p 1 ./internal/gitlab -count=1'
```

Then, without the test memory cap, lint the exact changed Go paths (add any
additional actually changed Go test path to this list):

```bash
KANDEV_RUN_QUIET_DIR=/home/jcfs/.kandev/loop-evidence/61dadc35-4a42-4581-8c94-62309c160fd9/child029/lint GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=3GiB scripts/run-quiet backend-lint -- bash scripts/lint-go-changed apps/backend/internal/gitlab/service_merge.go apps/backend/internal/gitlab/service_mr_automation_auth.go apps/backend/internal/gitlab/service_mr_automation_auth_test.go apps/backend/internal/gitlab/merge_head_test.go apps/backend/internal/gitlab/squash_choice_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
git status --short
```

Use a fresh task-owned quiet directory for every additional invocation; do not
reuse these names on a rerun. No summary flag applies to backend commands.
Record real exit statuses and join every actual command handle. Do not run
broad backend, frontend, browser, typecheck or i18n suites without a new need.

## Files likely touched

- `apps/backend/internal/gitlab/service_merge.go`
- `apps/backend/internal/gitlab/service_mr_automation_auth.go`
- `apps/backend/internal/gitlab/service_mr_automation_auth_test.go`
- `apps/backend/internal/gitlab/merge_head_test.go` (automation squash expectation only)
- `apps/backend/internal/gitlab/squash_choice_test.go`
- `docs/public/integrations.md`, owning requirements and design, this package

## Dependencies

Separate explicit implementation instruction, full design review and exclusive
HEAVY release. No other work order.

## Risks

Fixture policy behavior is modeled rather than live; project restrictions remain
authoritative. Preserve the current source-head race guard and all identity
boundaries, including errors and no automatic retry.

## Parallelism

Sequential.

## Inputs

AC .24-.30 and the [GitLab integration design](../../specs/integrations/system-design/gitlab-integration-02.md); existing service, automation and merge-head tests.

## Results

- Frozen workspace installation passed with pnpm 9.15.9 and Node 24.21.0.
- Independent RED: the unchecked default-off route and default-on automation
  cases failed on a forced `squash: true`; nine controls passed.
- Independent GREEN: all 11 cases passed under Go 1.26.0 with race detection.
  The owning GitLab package also passed its race run. After the fixture-only
  staticcheck correction, focused GREEN passed again.
- Five-file changed-Go lint passed at 3 GiB after the initial QF1003 finding
  was corrected. Specification catalog, spec lint and both public-doc
  validators and documentation-coverage preflight passed. No browser or live
  GitLab test was performed.
