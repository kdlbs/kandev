---
id: "01-legacy-streaming"
title: "Restore legacy streaming through the real repository"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.4
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.1
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
---

# Task 01: Restore legacy streaming through the real repository

## Summary

Restore legacy streaming through the real repository. Preserve the existing durable identity and admission boundaries.

## In scope

Guard canonical projection with positively identified durable event identity. Route genuine legacy message, reasoning, tool, and terminal events through existing callbacks.
Reject malformed events from an established v1 stream; an empty identifier must not silently downgrade that stream.
Use the real SQLite repository in lifecycle integration coverage. Existing fakes omit the canonical projector and conceal this defect.
Cover authenticated older peers and Kubernetes storage_not_durable. Preserve pending-v1 admission fences.

## Out of scope

Other work orders, automatic prompt resend, native conversation replacement, and unrelated cleanup.

## Acceptance

- Legacy text and reasoning render with the production repository; v1 events still enter the inbox and project once.
- Missing identity on an established v1 stream produces recovery, not a legacy callback.

## Regression evidence

Add durable_stream_legacy_test.go in lifecycle: TestLegacyStreamRealRepository and TestDurableStreamRejectsMissingIdentity. Exercise callbacks, canonical rows, reasoning metadata, and terminal ordering.

## Verification

Run from the repository root after implementation.

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite -count=1)
make -C apps/backend lint
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/streams.go`
- `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_stream.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes.go`

Add named regression files beside the relevant production package.

## Dependencies

None.

## Risks

Partial failure must preserve ownership, original submission identity, and replay evidence. Do not clear uncertainty solely because transport reconnects.

## Parallelism

`sequential`

## Inputs

- [Plan and source evidence](plan.md).
- [Delivery requirements](../../specs/platform/requirements/durable-agent-delivery.md).
- [Delivery design](../../specs/platform/system-design/durable-agent-delivery.md).
- Existing tests beside the listed files.

## Results

Implemented the durable identity guard and real SQLite legacy projection path.
Malformed events from an established v1 stream now surface uncertain delivery
instead of silently taking the legacy callback path.

Validation passed:

- `(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite -count=1)`
- `make -C apps/backend lint`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`
