---
id: "16-fixture-provider-proof"
title: "Fixture provider proving every capability (R8, M1-M8)"
status: pending
wave: 10
depends_on:
  - "15"
  - "17"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PLUGIN-PROFILE-003
  - REQ-EXECUTORS-PLUGIN-PLACEMENT-002
  - REQ-EXECUTORS-PLUGIN-CAPABILITY-003
  - REQ-EXECUTORS-NATIVE-WORKER-008
  - REQ-EXECUTORS-PLUGIN-CAPABILITY-002
  - REQ-EXECUTORS-PLUGIN-SHARED-001
  - REQ-EXECUTORS-PLUGIN-ADMISSION-001
  - REQ-EXECUTORS-PLUGIN-PLACEMENT-001
  - REQ-EXECUTORS-PLUGIN-OPS-001
  - REQ-EXECUTORS-PLUGIN-LAUNCHPOLICY-001
  - REQ-EXECUTORS-PLUGIN-PREPARATION-001
  - REQ-EXECUTORS-PLUGIN-MIGRATION-001
acceptance_criteria:
  - AC-EXECUTORS-NATIVE-WORKER-008.4
  - AC-EXECUTORS-NATIVE-WORKER-008.5
  - AC-EXECUTORS-NATIVE-WORKER-001.3
  - AC-EXECUTORS-PLUGIN-CAPABILITY-002.2
  - AC-EXECUTORS-PLUGIN-SHARED-001.6
  - AC-EXECUTORS-PLUGIN-ADMISSION-001.9
  - AC-EXECUTORS-PLUGIN-PLACEMENT-001.5
  - AC-EXECUTORS-PLUGIN-OPS-001.8
  - AC-EXECUTORS-PLUGIN-LAUNCHPOLICY-001.4
  - AC-EXECUTORS-PLUGIN-PREPARATION-001.4
  - AC-EXECUTORS-PLUGIN-MIGRATION-001.4
  - AC-EXECUTORS-PLUGIN-PROFILE-003.3
  - AC-EXECUTORS-PLUGIN-PLACEMENT-002.4
  - AC-EXECUTORS-PLUGIN-CAPABILITY-003.7
  - AC-EXECUTORS-PLUGIN-PLACEMENT-001.9
  - AC-EXECUTORS-PLUGIN-PROFILE-003.5
system_design:
  - ../../specs/executors/system-design/remote-executor-plugin-native-workers.md
  - ../../specs/executors/system-design/remote-executor-plugin-capability-versioning.md
  - ../../specs/executors/system-design/remote-executor-plugin-shared-admission-placement.md
  - ../../specs/executors/system-design/remote-executor-plugin-operations-migration.md
  - ../../specs/executors/system-design/remote-executor-plugin-profile-contract.md
---

# Task 16: Fixture provider proving every capability (R8, M1-M8)

## Summary

Extend cmd/plugin-fixture to declare every capability behind configuration and run one conformance test through the managed plugin process covering each capability and barrier, with no vendor, cluster, network, or credentials.

## Size

M: ~300 source, ~650 test lines, including one Playwright spec.

## In scope

- One parametrized Playwright spec triggers each remaining host-originated failure code (`capability_mismatch`, `missing_capability`, `executor_migrating`, `admission_denied` including `scope_full`, `admission_unavailable`, `launch_policy_unsupported`, `launch_policy_unenforced`, `secret_reference_missing`, `secret_in_state`, `bootstrap_consumed`, `repository_inventory_mismatch`, `state_too_large`, `api_unreachable`, `placement_failed`, `placement_timeout`) through the fixture and asserts localized text with an icon, using the pseudo-locale.
- Extend cmd/plugin-fixture to declare every capability behind configuration and run one conformance test through the managed plugin process covering each capability and barrier, with no vendor, cluster, network, or credentials.

## Out of scope

- Any vendor code or harness.

## Acceptance

- The conformance test exercises tunnel, descriptor, intent, drain, destroy checkpoints, durable bootstrap, each barrier, and one proof per medium-term item: negotiation mismatch (M7), two-owner release (M1), admission racing resume (M2), queued launch cancelled during placement (M3), status, event, and maintenance reads (M4), required launch policy without a wrapper (M5), two unseen preparation strategies (M6), freeze then rollback (M8), secret rotation before recovery, a provision refusal that settles without recovery, and a 12 KiB representative state with the combined worst-case envelope.
- The unchanged legacy conformance test still passes.

## Failing-first tests

- Conformance test over the real plugin process fails first, then passes capability by capability.

## Verification

Run from the repository root:

```bash
make -C apps/backend e2e-plugin-package
```

```bash
(cd apps/backend && go test -race -trimpath ./cmd/plugin-fixture ./pkg/pluginsdk ./internal/plugins/... -count=1)
```

```bash
(cd apps/backend && go test -race -trimpath ./internal/agent/runtime/lifecycle -count=1)
```

## Likely files and risks

- `apps/backend/cmd/plugin-fixture`
- `docs/public/plugins-authoring.md (docs-maintainer pass)`
- `docs/public/plugins-manifest.md`
- Risk: concurrency on both dialects; add the PostgreSQL race run before marking done.
