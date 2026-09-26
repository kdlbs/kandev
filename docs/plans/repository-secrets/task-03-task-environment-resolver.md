---
id: "03-task-environment-resolver"
title: "Resolve multi-source task environments"
status: done
wave: 3
depends_on: ["01-scoped-secret-storage", "02-repository-secret-bindings"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/repository-secrets.md"
---

# Task 03: Resolve Multi-Source Task Environments

## Acceptance

- One origin-aware resolver merges managed env, the selected executor profile, and all attached
  repository bindings independent of repository order.
- Same-key/same-secret references deduplicate; every other differing same-key definition fails.
- Missing, deleted, unreadable, unauthorized, and wrong-workspace repository refs fail before
  provisioning.
- User-facing/log errors expose key and origin labels but no value or secret ID.
- Fresh launch, workspace prepare, full relaunch, cold resume, and reset use the resolver.
- Agent-profile env remains Global-only fill-missing behavior after the task environment.

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_state.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- New focused resolver/error files under `apps/backend/internal/orchestrator/executor/`
- `apps/backend/internal/orchestrator/executor/executor_multi_repo_test.go`
- Launch, prepare, resume, reset, credential-collision, and resolver tests

## Inputs

- Completed scope-specific resolution APIs from Task 01.
- Completed repository binding reads from Task 02.
- ADR `Origin-aware task environment merge` tree.

## Dependencies

Tasks 01 and 02.

## TDD sequence

1. Write table-driven RED tests for dedupe, all conflict pairs, order invariance, managed collisions,
   and redaction.
2. Write launch-spy RED tests proving resolution failure happens before `LaunchAgent` and setup.
3. Preserve executor source definitions in `executorConfig`, implement the resolver, and wire all
   fresh/cold paths.
4. Add regression tests for warm snapshot retention and unchanged agent-profile fill-missing rules.

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator/executor/... ./internal/agent/runtime/lifecycle/...
make -C apps/backend lint
```

## Risks

- Do not compare plaintext to decide identity or dedupe.
- Managed Git, GitLab, Office, and caller-provided values enter at different stages; perform the
  final collision validation after all task-environment origins are known.
- Workspace-only preparation and later agent start must not resolve different environments.

## Output contract

Report the origin model, exact launch checkpoint, conflict matrix, error redaction, all wired entry
points, files changed, tests run, and residual risks.

## Result

Implemented the origin-aware task environment resolver and wired it into fresh launch, preparation,
relaunch, cold resume, and reset paths. Same-reference bindings deduplicate; all other same-key
collisions and broken references fail before provisioning with key/origin-only diagnostics. Agent
profile values remain Global-only fill-missing inputs. Resolver, multi-repository, launch-order,
redaction, and snapshot tests passed.
