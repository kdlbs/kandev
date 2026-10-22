---
id: "03-plugin-capability"
title: "Add the optional sandbox provider contract"
status: pending
wave: 3
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-SANDBOX-001
  - REQ-AGENTS-SANDBOX-001
acceptance_criteria:
  - AC-PLUGINS-SANDBOX-001.1
  - AC-PLUGINS-SANDBOX-001.2
  - AC-PLUGINS-SANDBOX-001.3
  - AC-PLUGINS-SANDBOX-001.4
  - AC-PLUGINS-SANDBOX-001.5
  - AC-AGENTS-SANDBOX-001.1
  - AC-AGENTS-SANDBOX-001.2
  - AC-AGENTS-SANDBOX-001.3
  - AC-AGENTS-SANDBOX-001.4
system_design:
  - ../../specs/plugins/system-design/sandbox-provider-capability.md
  - ../../specs/agents/system-design/sandbox-policies.md
---

# Task 03: Add the optional sandbox provider contract

## Summary

Add the typed optional capability, manifest validation, SDK methods, and deterministic fixture.

## In scope

Implement negotiation, bounded calls, artifact validation, and compatibility failure paths through the existing plugin transport.

## Out of scope

No production Nono implementation in the monorepo and no remote executor interface reuse.

## Acceptance

- New and old plugin combinations negotiate or refuse without affecting ordinary prefixes.
- Malformed replies, unknown versions, timeout, crash, and disabled providers cannot cause unwrapped fallback.
- The fixture proves calls cannot replace the admitted child or persist a policy.

## Verification

Commands run from the Kandev repository root unless a subshell selects the dedicated plugin checkout.
New test and runner paths below are implementation deliverables, not existing passing evidence.

```bash
make -C apps/backend proto
(cd apps/backend && go test -trimpath ./pkg/pluginsdk ./internal/plugins/... ./internal/common/launchwrapper -run 'SandboxProvider|GenericWrapper' -count=1)
```

Use TDD for changed logic. Before the first pnpm command in this workspace, run `(cd apps && pnpm install --frozen-lockfile)`.
Run each listed suite after its files exist. Do not count a zero-test filter as a pass.

## Files likely touched

- `apps/backend/proto/kandev/plugin/v1/plugin.proto`
- `apps/backend/pkg/pluginsdk/sandbox_provider.go (new)`
- `apps/backend/internal/plugins/manifest/`
- `apps/backend/internal/plugins/`
- `apps/backend/cmd/plugin-fixture/`
- `apps/backend/pkg/pluginsdk/sandbox_provider_test.go (new)`

## Dependencies

None. Execute sequentially under the plan.

## Risks

Generated protobuf changes and manifest permissions must remain backward compatible.

## Parallelism

`sequential`

## Inputs

- [Evidence](investigation.md).
- [Design](../../specs/plugins/system-design/sandbox-provider-capability.md).
- [Design](../../specs/agents/system-design/sandbox-policies.md).

## Results

Pending. No production implementation is authorized during the design turn.
