---
id: "01-managed-command"
title: "Accept and prepare native managed commands"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-CODEX-NATIVE-001
  - REQ-AGENTS-CODEX-NATIVE-002
acceptance_criteria:
  - AC-AGENTS-CODEX-NATIVE-001.1
  - AC-AGENTS-CODEX-NATIVE-002.1
system_design:
  - ../../specs/agents/system-design/codex-app-server.md
---

# Task 01: Accept and prepare native managed commands

## Summary

Make the native utility validator accept Kandev's actual managed Codex command.
Prepare its trusted npm prefix before spawning, using the same helper as ACP.

## In scope

- Add a failing regression using the real Codex agent command builder for default and selected exact versions.
- Accept only the canonical trusted prefix marker alongside the pinned Codex package and app-server arguments.
- Preserve supported existing command forms; reject arbitrary prefix paths, packages, wrappers, and extra flags.
- Copy command arguments before prefix preparation so shared config is not mutated.
- Invoke `managedruntime.PrepareNPMProjectPrefix` before process launch and propagate preparation failures.
- Test `Probe` with a fake executable that initializes and returns models; assert the prepared absolute private prefix reaches the process.
- Exercise the shared launch path used by `Execute`, without authenticated model calls.

## Acceptance

1. The real generated command passes validation and model probing returns the fake provider's models.
2. Both utility paths use the prepared isolation directory without mutating the original command.
3. Invalid commands remain rejected, and no user workspace npm configuration is used as the project root.

## Files likely touched

- `apps/backend/internal/agentctl/server/utility/codex_app_server.go` and its tests.
- `apps/backend/internal/agent/hostutility/managed_runtime_test.go`, if needed for the selected-version boundary.

## Verification

Run from repository root:

```bash
(cd apps/backend && go test -race ./internal/agentctl/server/utility ./internal/agent/hostutility ./internal/agent/managedruntime ./internal/agent/agents)
make -C apps/backend lint
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Dependencies and parallelism

None. Sequential implementation in the primary session; preserve other sessions' edits.

## Risks

Do not fix the mismatch by removing npm isolation or allowing arbitrary executable paths.

## Inputs

- Native agent design, registration and client execution sections.
- `ManagedNPMRuntimeSpec.RuntimeCommand`, `NPMProjectPrefixArgs`, and ACP utility prefix preparation.

## Results

Completed. The resolver now accepts only the canonical managed-prefix form and retains the previous supported command forms. The shared launcher prepares a cloned argument slice before spawn and fails closed if preparation fails. Fake-process probing returned the expected model; selected exact versions, invalid prefixes/arguments, original command preservation, and preparation failure are covered.

Verification passed: the four-package backend race command, `make -C apps/backend lint` (0 issues), catalog validation (306 decisions, 1155 specifications), full specification lint, and `git diff --check`. After the CI lint correction, `go test -race ./internal/agentctl/server/utility` and the CI-style changed-code lint also passed. No authenticated model request ran.
