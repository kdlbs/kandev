---
id: "05-nono-provider-lifecycle"
title: "Implement Nono in its own repository"
status: pending
wave: 5
depends_on:
  - "01-runtime-contract"
  - "04-policy-authority"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SANDBOX-001
  - REQ-AGENTS-SANDBOX-003
  - REQ-PLUGINS-SANDBOX-001
acceptance_criteria:
  - AC-AGENTS-SANDBOX-001.1
  - AC-AGENTS-SANDBOX-001.2
  - AC-AGENTS-SANDBOX-001.3
  - AC-AGENTS-SANDBOX-001.4
  - AC-AGENTS-SANDBOX-003.1
  - AC-AGENTS-SANDBOX-003.2
  - AC-AGENTS-SANDBOX-003.3
  - AC-AGENTS-SANDBOX-003.4
  - AC-AGENTS-SANDBOX-003.5
  - AC-AGENTS-SANDBOX-003.6
  - AC-PLUGINS-SANDBOX-001.1
  - AC-PLUGINS-SANDBOX-001.2
  - AC-PLUGINS-SANDBOX-001.3
  - AC-PLUGINS-SANDBOX-001.4
  - AC-PLUGINS-SANDBOX-001.5
system_design:
  - ../../specs/agents/system-design/sandbox-policies.md
  - ../../specs/plugins/system-design/sandbox-provider-capability.md
---

# Task 05: Implement Nono in its own repository

## Summary

Build the production Nono plugin in a dedicated repository and connect core-owned launch lifecycles.

## In scope

Use the official template, pinned runtime probe, policy compiler, foreground supervisor, host-owned artifacts, pipe/PTY handling, and retained snapshots.

## Out of scope

No remote execution, detached Nono ownership, global installation, marketplace publication, or macOS support claim without evidence.

## Acceptance

- The packaged plugin runs discovery, ACP, and terminal fixtures under the selected policy.
- Initial, restart, cancellation, timeout, failed startup, plugin loss, and cleanup preserve ownership and policy snapshots.
- Plugin source stays outside this repository and uses only public SDK contracts.

## Verification

Commands run from the Kandev repository root unless a subshell selects the dedicated plugin checkout.
New test and runner paths below are implementation deliverables, not existing passing evidence.

```bash
(cd apps/backend && go test -trimpath -race ./internal/agent/hostutility ./internal/agent/runtime/lifecycle ./internal/agentctl/server/process ./internal/agentctl/server/utility -run 'SandboxLifecycle|SandboxTransport|GenericWrapper' -count=1)
(cd "${NONO_PLUGIN_DIR:?Set the dedicated plugin checkout}" && go test ./... -count=1)
(cd "${NONO_PLUGIN_DIR:?Set the dedicated plugin checkout}" && make validate package)
python3 scripts/check-nono-real.py --plugin-smoke --record docs/plans/nono-sandbox-integration/acceptance.md
```

Use TDD for changed logic. Before the first pnpm command in this workspace, run `(cd apps && pnpm install --frozen-lockfile)`.
Run each listed suite after its files exist. Do not count a zero-test filter as a pass.

## Files likely touched

- `Dedicated kdlbs/kandev-plugin-nono repository: manifest.yaml, Go provider, conformance tests, Makefile validation/package targets`
- `apps/backend/internal/agent/hostutility/`
- `apps/backend/internal/agent/runtime/lifecycle/`
- `apps/backend/internal/agentctl/server/process/`
- `apps/backend/internal/agentctl/server/utility/`
- `apps/backend/internal/agent/runtime/lifecycle/sandbox_lifecycle_test.go (new)`

## Dependencies

01-runtime-contract, 04-policy-authority

## Risks

Repository creation/attachment occurs only after the design checkpoint. Nested PTYs require actual transport evidence.

## Parallelism

`sequential`

## Inputs

- [Evidence](investigation.md).
- [Design](../../specs/agents/system-design/sandbox-policies.md).
- [Design](../../specs/plugins/system-design/sandbox-provider-capability.md).

## Results

Pending. No production implementation is authorized during the design turn.
