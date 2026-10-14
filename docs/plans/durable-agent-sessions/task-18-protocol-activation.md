---
id: "18-protocol-activation"
title: "Activate negotiated durable delivery"
status: complete
wave: 18
depends_on: ["17-disconnect-reconciliation"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.4
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
---

# Task 18: Activate negotiated durable delivery

## Summary

Activate v1 automatically for compatible peers and supported retained storage, without a feature flag.

## In scope

- Advertise v1 only after journal storage, submissions, replay, inbox projection, and reconciliation are wired.
- Implement mixed-version fallback, retained-v1 recovery during rollback, and newer-format rejection.
- Add automatic-activation, legacy-peer, supported-storage-failure, and upgrade tests. Failed journal initialization must not select legacy mode.
- Document the minimum recovery-capable rollback version. Reject unsupported downgrade with unresolved durable work.
- Update root and scoped AGENTS guidance and requirement/design status only to match verified implementation.

## Out of scope

Runtime toggles, feature graduation, broad generic QA, commits, and release publication.

## Acceptance

- Compatible installations use v1 automatically. Genuine legacy peers remain identified, while journal failures block admission.
- Restart and rollback retain data. Active or uncertain v1 work cannot silently enter legacy admission.
- Operator docs state executor storage requirements, explicit continuation, uncertainty, and unsupported guarantees.

## Verification

Run from the repository root. New test names describe required evidence, not existing passing tests.
Use TDD for implementation. Record the failing assertion before the implementation result.

```bash
(cd apps/backend && rtk go test -tags fts5 -race ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/agentctl/server/api -count=1)
rtk make -C apps/backend build-agentctl build-agentctl-remote
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
rtk make -C apps/backend lint
(cd apps/web && rtk pnpm run lint)
rtk node --test scripts/validate-public-docs.test.mjs
rtk node scripts/validate-public-docs.mjs
```

Target evidence in `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_activation_test.go`:

- `TestDurableDeliveryLegacyPeerCompatibility`: `AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.1`.
- `TestDurableDeliveryRollbackPreservesUncertainty`: `AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.2`.

Additional tests in `durable_delivery_activation_test.go`:

- `TestDurableDeliveryActivatesWithoutConfiguration`: `AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.3`.
- `TestJournalFailureNeverSelectsLegacyDelivery`: `AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.4`.

## Files likely touched

- `apps/backend/internal/agent/runtime/agentctl/`
- `apps/backend/internal/agent/runtime/lifecycle/`
- `apps/backend/internal/agentctl/server/api/`
- `docs/public/sessions-and-review.md`
- `docs/public/executors.md`
- `docs/public/agents-and-profiles.md`
- `AGENTS.md` (agent_restore_* and agent_delivery_* producer documentation).
- `README.md` and `docs/screenshots.md` (inspect terminology, change only if affected).
- `apps/backend/internal/agentctl/AGENTS.md`
- `apps/backend/AGENTS.md`
- `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_activation_test.go` (new tests or extensions).

## Dependencies

[Task 17](task-17-disconnect-reconciliation.md).

## Risks

- Rollback cannot safely downgrade an active submission. Data-format compatibility and protocol compatibility require separate checks.

## Parallelism

`sequential`

The primary session owns integration. This work order does not authorize subagents.
Preserve existing user edits and unrelated changes.

## Inputs

- [Owned system design](../../specs/platform/system-design/durable-agent-delivery.md).
- [Package manifest](plan.md), including shared regression gates and test prerequisites.
- Existing source and adjacent tests in the listed files.
- [Boundary decision](../../decisions/2026-09-10-durable-harness-session-boundaries.md).

## Results

Implemented initialize-time capability propagation and automatic negotiation without a feature
flag. Legacy peers, unavailable retained storage, unresolved durable work, and unsupported
rollback are distinguished fail-closed. Agentctl native builds, focused race tests, public-doc
validation, specification lint, backend lint, and web lint pass. Mixed-version rollback and
shipped-harness evidence remain environment-dependent.
