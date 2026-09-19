---
id: "01-default-pins"
title: "Add reviewed default pins"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/runtime-updates.md"
---

# Task 01: Add reviewed default pins

## Acceptance

- One validated machine-readable catalogue defines an exact stable default for
  every built-in managed npm runtime.
- Every command built without an operator selection uses
  `package@default_version`; no supported managed runtime can fall back to an
  unversioned package.
- Normal launches retain `--prefer-offline`, while explicit preparation and the
  bounded stale-metadata retry retain online-preferred behavior.

## Verification

```bash
cd apps/backend && go test ./internal/agent/agents ./internal/agent/registry ./internal/agent/runtime/lifecycle
```

## Files likely touched

- `apps/backend/internal/agent/agents/managed_npm_runtime_versions.json`
- `apps/backend/internal/agent/agents/managed_npm_runtime_versions.go`
- `apps/backend/internal/agent/agents/managed_npm_runtime.go`
- `apps/backend/internal/agent/agents/agent.go`
- `apps/backend/internal/agent/agents/{claude_acp,codex_acp,opencode_acp,copilot_acp,gemini}.go`
- Focused `*_test.go` files beside the managed agents and lifecycle command builder

## Dependencies

None.

## Parallelism

Sequential. This establishes the shared catalogue and command invariant used by
all later tasks.

## Inputs

- Spec: managed package set, effective version semantics, command routing
- Plan: Central managed-runtime pins
- Existing pattern: `ManagedNPMRuntimeSpec.PackageSpec` and command tests

## Output contract

Report the selected initial pins, files changed, exact test result, absence of
unversioned managed commands, risks, and synchronized task/plan status.

## Results

Complete. The catalogue pins are Claude `0.70.0`, Codex `1.6.0`, OpenCode
`1.18.18`, Copilot `1.0.75`, and Gemini `0.52.0`. The embedded Go validator
rejects missing, malformed, prerelease, and non-stable defaults. Empty-version
commands now use `package@default_version`; explicit validated selections use
`package@selection`, while normal launches remain `--prefer-offline` and
preparation/recovery remain online-preferred.

Verification: `go test ./internal/agent/agents ./internal/agent/registry
./internal/agent/runtime/lifecycle -count=1` passed as part of the current
2,646-test post-remediation scoped backend run.

Follow-up review verification removed duplicated current-pin snapshots from
the managed-agent and lifecycle tests. Their command expectations now derive
from the embedded catalogue, so the weekly updater can change a pin without
making the generated PR fail on stale test literals. The focused catalogue and
command suite passed again as part of the 2,594-test follow-up backend run.
