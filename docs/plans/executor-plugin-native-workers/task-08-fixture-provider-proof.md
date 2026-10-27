---
id: "08-fixture-provider-proof"
title: "Fixture provider proving every capability (R8)"
status: pending
wave: 5
depends_on:
  - "07"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-NATIVE-WORKER-008
acceptance_criteria:
  - AC-EXECUTORS-NATIVE-WORKER-008.4
  - AC-EXECUTORS-NATIVE-WORKER-008.5
  - AC-EXECUTORS-NATIVE-WORKER-001.3
system_design:
  - ../../specs/executors/system-design/remote-executor-plugin-native-workers.md
---

# Task 08: Fixture provider proving every capability (R8)

## Summary

Extend cmd/plugin-fixture to declare every capability behind configuration and run one conformance test through the managed plugin process covering each capability and barrier, with no Nomad, network, or credentials.

## Size

M: ~250 source, ~400 test lines.

## In scope

- Extend cmd/plugin-fixture to declare every capability behind configuration and run one conformance test through the managed plugin process covering each capability and barrier, with no Nomad, network, or credentials.

## Out of scope

- Any vendor code or harness.

## Acceptance

- The conformance test exercises tunnel, descriptor, intent, drain, destroy checkpoints, durable bootstrap, and each barrier.
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
go test -race -trimpath ./internal/agent/runtime/lifecycle -count=1
```

## Likely files and risks

- `apps/backend/cmd/plugin-fixture`
- `docs/public/plugins-authoring.md (docs-maintainer pass)`
- `docs/public/plugins-manifest.md`
- Risk: concurrency on both dialects; add the PostgreSQL race run before marking done.
