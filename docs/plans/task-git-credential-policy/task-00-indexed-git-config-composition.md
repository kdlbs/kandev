---
id: "00-indexed-git-config-composition"
title: "Preserve ordered indexed Git configuration across task environments"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-authentication.md"
---

# Task 00: Preserve Ordered Indexed Git Configuration

## Acceptance

- Valid `GIT_CONFIG_COUNT` / `GIT_CONFIG_KEY_<n>` / `GIT_CONFIG_VALUE_<n>` blocks from the host,
  executor, profile, task, and Kandev compose into one contiguous ordered block.
- Higher-precedence entries follow lower-precedence entries. Only the longest exact suffix/prefix
  overlap is emitted once; arbitrary repeated entries remain untouched.
- A locstat-shaped host block followed by Kandev's two managed credential entries produces count 4,
  preserves hooks and notes settings, and a real Git commit executes the configured hook.
- Docker retains `safe.directory` and SSH-to-HTTPS rewrites, while standalone and remote agentctl
  instance creation do not clobber or duplicate forwarded entries.
- Malformed or unreasonably large blocks fail environment preparation with a sanitized error.
  `internal/repoclone` remains intentionally isolated from ambient Git config.

## Verification

```bash
cd apps/backend && go test ./internal/gitconfigenv ./internal/agentctl/server/config ./internal/agent/runtime/lifecycle ./internal/orchestrator/executor -run 'Test.*(IndexedGitConfig|CollectAgentEnv.*GitConfig|BuildEnvVars.*GitConfig|GitConfigPreservesHook)'
```

## Files likely touched

- new shared package under `apps/backend/internal/gitconfigenv/`
- `apps/backend/internal/agentctl/server/config/config.go`
- `apps/backend/internal/agentctl/server/config/config_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/profile_env.go`
- `apps/backend/internal/agent/runtime/lifecycle/container.go`
- `apps/backend/internal/agent/runtime/lifecycle/remote_github_env.go`
- focused standalone, Docker, and SSH/remote tests
- `apps/backend/internal/orchestrator/executor/executor_credentials.go`
- `apps/backend/internal/orchestrator/executor/executor_credentials_test.go`
- this task file and `plan.md`

## Dependencies

None.

## Parallelism

Parallel-safe with Task 01: this task owns environment composition/runtime plumbing while Task 01
owns workspace settings persistence.

## Inputs

- The reproduced `CollectAgentEnv` failure: parent count 2 plus task count 2 returns count 2 instead
  of 4.
- Existing Docker composition in `container.go`, remote allowlisting in
  `remote_github_env.go`, and managed clone isolation in `internal/repoclone`.
- Spec scenarios for locstat-shaped hooks/notes preservation and exact boundary overlap.

## Output contract

Report the parser validity rules and count bound, precedence and overlap algorithm, every updated
merge boundary, real Git subprocess RED/GREEN evidence, executor-specific focused results,
confirmed `repoclone` isolation, files changed, and update task/plan status.
