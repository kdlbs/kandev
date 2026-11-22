---
id: "01-deliver-managed-credentials"
title: "Deliver managed credentials to the relaunched agent"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001
  - REQ-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.15
  - AC-PLATFORM-GIT-CREDENTIAL-LEASE-REISSUE-001.10
system_design:
  - ../../specs/integrations/system-design/github-authentication-02.md
  - ../../specs/platform/system-design/git-credential-lease-reissue.md
---

# Task 01: Deliver managed credentials to the relaunched agent

## Summary

Use the fresh launch credentials when a workspace-only execution starts a new
agent subprocess. Surface missing required credentials before agent start.

## Root cause

The three-test sequence at `30a3e06372d` failed at the fresh managed child
process assertion. Temporary diagnostics for the same session recorded:

| Stage | Lease present | Reissue capability present |
| --- | --- | --- |
| Resume issued one managed repository scope | Yes | Yes |
| `promoteWorkspaceExecution` received the launch request | Yes | Yes |
| Agentctl configure received the workspace execution's environment | No | No |

The workspace read created the execution without a lease. Promotion copied the
agent command and reused the workspace's environment. The startup path read
that snapshot because the execution had no per-run overlay.
The missing-issuer branch was a separate silent return reproduced by Go tests.

## In scope

- Managed Git environment composition during workspace promotion.
- Launch error when a managed GitHub repository requires an unavailable broker.
- Regression coverage at issuance, resume dispatch, and agentctl configuration.

## Out of scope

- Task queue behavior, browser test changes, and broker authorization changes.

## Acceptance

- A workspace-only execution sends the new session-bound lease and capability to
  agentctl, with the generated helper and unrelated indexed Git entries.
- Missing required credentials prevent launch. Resume restores its prior
  recoverable state when it owns the failed attempt.
- The three-test Playwright sequence passes five consecutive runs. The full
  agent-survival restart and repeated native resume specs pass.

## Verification

From the repository root:

```bash
(cd apps && pnpm install --frozen-lockfile)
make build-backend build-backend-linux-helpers build-web-e2e build-e2e-plugin-package build-e2e-delivery-fixture build-e2e-send-now-fixture
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle ./internal/orchestrator/executor -count=1)
bash scripts/lint-go-changed \
  apps/backend/internal/agent/runtime/lifecycle/{managed_git_environment.go,manager_launch.go,manager_launch_credentials_test.go} \
  apps/backend/internal/orchestrator/executor/{executor_credentials.go,executor_credentials_availability_test.go,executor_resume_credentials_test.go}
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

From `apps/web`:

```bash
for run in 1 2 3 4 5; do
  pnpm exec playwright test --config e2e/playwright.config.ts --workers=1 --retries=0 \
    e2e/tests/session/agent-survival-restart.spec.ts \
    -g 'in-flight turn survives a graceful|fresh managed Git' || exit
done
pnpm exec playwright test --config e2e/playwright.config.ts --workers=1 --retries=0 \
  e2e/tests/session/agent-survival-restart.spec.ts
pnpm exec playwright test --config e2e/playwright.config.ts --workers=1 --retries=0 \
  e2e/tests/session/repeated-native-resume.spec.ts
```

## Files

- `apps/backend/internal/agent/runtime/lifecycle/managed_git_environment.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch_credentials_test.go`
- `apps/backend/internal/orchestrator/executor/executor_credentials.go`
- `apps/backend/internal/orchestrator/executor/executor_credentials_availability_test.go`
- `apps/backend/internal/orchestrator/executor/executor_resume_credentials_test.go`

## Dependencies

None.

## Risks

A prior overlay can carry an obsolete lease. The promotion regression tests both
an empty credential environment and a lease-only overlay. The workspace's
unrelated Git configuration must survive both cases.

## Results

The Go regressions reproduced the missing or stale environment and the missing
issuer success before the fix. After the fix, promotion sent the fresh lease,
capability, session identity, and helper to agentctl. The missing-issuer test
confirmed that resume returned an error and restored the cancelled state.

- Frozen workspace install and all six E2E artifact targets passed.
- Both Go packages passed. The changed-file Go lint reported zero issues.
- The three-test sequence passed five consecutive runs with 15 tests passed.
- The full `agent-survival-restart.spec.ts` run passed all four tests.
- The `repeated-native-resume.spec.ts` run passed both tests.
- The documentation catalog, spec lint, PR documentation coverage, and diff
  whitespace checks passed.
