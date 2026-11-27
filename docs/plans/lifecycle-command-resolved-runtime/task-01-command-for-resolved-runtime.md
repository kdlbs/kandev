---
id: "01-command-for-resolved-runtime"
title: "Command for the resolved runtime"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-OPENCODE-V2-001
acceptance_criteria:
  - AC-AGENTS-OPENCODE-V2-001.11
system_design:
  - ../../specs/agents/system-design/opencode-v2-adoption.md
---

# Task 01: Command for the resolved runtime

## Summary

Resolve managed command options for the runtime the execution resolved, not
the requested executor type, so a native OpenCode launch does not end with an
empty command.

## In scope

- Add an execution test for a native OpenCode selection with no, `local` and
  `local_docker` requested executor types resolved to standalone.
- Pass the resolved runtime into `buildAgentCommandWithContext`.
- Record the rule in the OpenCode v2 design.

## Out of scope

Backend resolution, native version probing, and UI changes.

## Acceptance

- The execution resolved to standalone uses the native OpenCode command for
  each requested executor type.
- Existing command tests pass with an explicit runtime.

## Verification

From the repository root:

```bash
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle -count=1)
(cd apps/backend && go vet -trimpath ./internal/agent/runtime/lifecycle)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- apps/backend/internal/agent/runtime/lifecycle/manager_launch.go
- apps/backend/internal/agent/runtime/lifecycle/default_executor_command_test.go
- apps/backend/internal/agent/runtime/lifecycle/managed_runtime_command_test.go
- apps/backend/internal/agent/runtime/lifecycle/manager_launch_test.go
- docs/specs/agents/system-design/opencode-v2-adoption.md

## Dependencies

None.

## Risks

A caller that resolved a different backend than requested gets that backend's
command.

## Parallelism

`sequential`

## Inputs

- REQ-AGENTS-OPENCODE-V2-001 and AC-AGENTS-OPENCODE-V2-001.11.
- The linked OpenCode v2 adoption design.

## Results

- RED: on the original command builder, a request with no executor type failed
  with `agent command cannot be empty`, and a `local_docker` request resolved
  to standalone got the managed `npx` command; `local` already passed.
- GREEN: the new test and the existing command tests pass.

### PR review verification

The request matrix now includes an unknown executor type. A separate workspace
promotion regression uses the resolved standalone runtime with empty, unknown,
local, and local-Docker requests and checks the native ACP argv. This full
lifecycle regression is assigned to hosted Backend CI; no broad local
lifecycle package rebuild is claimed.
