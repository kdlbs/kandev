---
id: "02-prove-recovered-profile-status"
title: "Prove recovered profile status"
status: done
wave: 2
depends_on:
  - "01-recover-host-utility-probes"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.6
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.7
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.8
system_design:
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
---

# Task 02: Prove recovered profile status

## Summary

Add user-visible evidence that startup recovery publishes a healthy capability
catalogue before task launch. Update the operator-facing explanation of stale
npm metadata recovery.

## In scope

- Add a host E2E fixture that fails only the offline exact-package probe.
- Assert the online retry succeeds without task launch or manual refresh.
- Assert the profile remains selectable with its saved model and no capability warning.
- Update the public agents guide and internal managed-runtime command reference.

## Out of scope

- New UI copy, controls, or tooltip changes.
- Container and SSH recovery, which existing E2E tests already cover.
- Failure-card behavior for task-session launches.

## Acceptance

- The test observes one offline failure followed by one online probe for the same exact version.
- The available-agent API and task selector expose recovered capabilities without changing the saved profile model.
- Documentation distinguishes automatic host probe recovery from a final visible runtime failure.

## Verification

```bash
pnpm e2e:run --host tests/settings/host-utility-managed-runtime-recovery.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/web/e2e/tests/settings/host-utility-managed-runtime-recovery.spec.ts`
- `apps/web/e2e/fixtures/managed-runtime-npx.sh`
- `apps/backend/internal/backendapp/main.go`
- `docs/public/agents-and-profiles.md`
- `apps/backend/internal/agent/agents/ACP_BRIDGE_VERSIONS.md`

## Dependencies

- Task 01 provides host utility recovery and the structured probe failure code.

## Risks

- The fixture must remain isolated from the developer's real npm cache and registry configuration.

## Parallelism

`sequential`

## Inputs

- `REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001`
- Host capability-probe flow in the system design.
- Existing managed-runtime recovery fixtures and settings profile selection flows.

## Results

- Added a host fixture that reproduces strict offline `ETARGET`, proves one exact cache repair and online retry, and preserves sibling cache data.
- Proved the recovered OpenCode profile keeps its saved model and appears without a capability warning before task launch.
- Removed the duplicate host utility bootstrap that could race two startup probes against the same npm execution tree.
- Kept a persisted profile across the recovery restart and made the cache-key fixture portable to macOS.
- Updated the public agent guide and internal managed-runtime reference.
- Passed `pnpm e2e:run --host tests/settings/host-utility-managed-runtime-recovery.spec.ts`.
- Passed the public documentation validators and complete specification lint.
