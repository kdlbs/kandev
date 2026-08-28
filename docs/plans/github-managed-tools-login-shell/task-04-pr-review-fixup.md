---
id: "04-pr-review-fixup"
title: "PR review fixup"
status: done
wave: 4
depends_on: ["03-review-blocker-remediation"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-authentication.md"
---

# Task 04: PR Review Fixup

## Acceptance

- Broker-enabled Local and Worktree requests publish the standalone launcher's absolute
  `agentctl` path before checkout, worktree creation, or repository setup can invoke Git.
- Executor-inheritance requests remove that managed helper path together with the broker lease and
  indexed helper configuration.
- Sprites environment construction avoids overflow-prone capacity arithmetic without changing its
  filtering or managed-helper output.
- A managed Bash startup environment never records itself as its inherited parent hook.

## Verification

RED first:

```bash
cd apps/backend && rtk go test ./internal/orchestrator/executor -run '^TestConfigureGitHubCredentialBrokerPublishesLocalHelperBeforePreparation$' -count=1
cd apps/backend && rtk go test ./internal/orchestrator/executor -run '^TestConfigureGitHubCredentialBrokerSkipsExecutorInheritedPolicy$' -count=1
```

GREEN and affected-package checks:

```bash
cd apps/backend && rtk go test ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle -run '^Test(ConfigureGitHubCredentialBroker(HelperSurvivesPathReset|PublishesLocalHelperBeforePreparation|SkipsExecutorInheritedPolicy)|BuildSpriteEnvPublishesManagedGitCredentialHelperBeforeAgentctlStartup)$' -count=1
cd apps/backend && rtk go test ./internal/agentctl/server/config -run '^TestCollectAgentEnvAvoidsManagedBashEnvSelfSourcing$' -count=1
cd apps/backend && rtk go test ./internal/backendapp ./internal/orchestrator ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle ./internal/agentctl/server/config -count=1
cd apps/backend && rtk golangci-lint run ./... --new-from-rev="$(git merge-base origin/main HEAD)" --timeout=5m
```

## Results

- The Local/Worktree RED test failed because the executor had no launcher-path seam. The
  executor-inheritance RED test failed because the managed path remained in the request.
- GitHub CodeQL alert 301 supplied the static RED evidence for `len(env)+1`; reproducing an integer
  overflow dynamically would require an infeasibly max-sized Go map.
- The final focused commands passed six tests across the executor and lifecycle packages plus the
  reviewer-requested Bash self-source guard. The five complete affected-package suites passed
  2,969 tests, and changed-file Go lint reported no issues.
- The launcher path is injected only for host-side Local, Worktree, and legacy-empty executor
  types. Docker and Sprites retain their installed remote path binding, and executor inheritance
  removes the host path. No lease, token, scope, or credential value is persisted or logged.
- CodeRabbit's optional shared-shell-constant refactor was not adopted: the two call sites have
  different broker and parent-hook guards, and moving their partial overlap into `githubauth` would
  add raw-shell coupling without changing the managed path contract.
