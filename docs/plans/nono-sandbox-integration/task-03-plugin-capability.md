---
id: "03-plugin-capability"
title: "Add the optional sandbox provider contract"
status: done
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
  - AC-PLUGINS-SANDBOX-001.6
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
Include canonical display metadata, override constraints, and `NormalizeSandboxStatus` evidence without an authoritative active boolean.
Use two differently named fixture providers to prove generic dispatch. Fixtures do not establish confinement.

## Out of scope

No production Nono implementation in the monorepo and no remote executor interface reuse.

## Acceptance

- New and old plugin combinations negotiate or refuse without affecting ordinary prefixes.
- Malformed replies, unknown versions, timeout, crash, and disabled providers cannot cause unwrapped fallback.
- Both fixtures use the same contract without provider-ID branches. Calls cannot replace the child, persist policy, or independently assert active confinement.

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

Completed with additive manifest, protobuf, SDK, and host dispatch contracts. Calls have deadlines, message limits, installation binding checks, artifact validation, and cancellation on disable, removal, upgrade, and configuration restart. Disabled providers remain visible as unavailable.

The packaged fixture exercises all six operations for two provider names through the same host path. It also covers disable, re-enable, upgrade, stale bindings, and removal. The fixture explicitly reports no confinement.

Validation passed: protobuf generation, focused provider tests, packaged conformance, existing SDK/manifest/wrapper compatibility tests, race tests, and changed-package Go lint. Old plugins return unsupported for the optional methods. This does not claim an old-host binary integration test or actual Nono product acceptance.

Policy persistence, launch snapshots, provider package digest pinning, and production provider enforcement belong to the following work orders.
