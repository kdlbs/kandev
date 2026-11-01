---
id: "01-output-codec"
title: "Lossless compact output codec"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INCREMENTAL-SHELL-001
acceptance_criteria:
  - AC-PLATFORM-INCREMENTAL-SHELL-001.1
  - AC-PLATFORM-INCREMENTAL-SHELL-001.2
system_design:
  - ../../specs/platform/system-design/incremental-shell-delivery.md
---

# Task 01: Lossless compact output codec

## Summary

Implement the typed checkpoint/patch representation and pure encode/apply helpers without activating a new producer. Preserve exact normalized output, including field presence and UTF-8 semantics.

## In scope

- Add proposed `shell_output_update` contract and `shell_output_patch.go` / `shell_output_patch_test.go` in `internal/agentctl/types/streams`.
- Test append, maximal suffix/prefix overlap after cap, repeated text, arbitrary replacements, command-echo normalization results, explicit empty/omitted streams, flags and nullable exit.
- Validate revisions/digests, identity mismatch, base64 decoding, decoded UTF-8/byte bounds and event sizing. Exercise both saturated fields with JSON-escaped/incompressible text.

## Out of scope

No production activation, schema, live runtime, retention or browser changes.

## Acceptance

- Round-trip every table case against a complete normalized snapshot; invalid bases and malformed payloads fail without mutation.
- Saturated append encodes only changed suffix bytes plus metadata; identical text/metadata is a no-op. All normal maximum checkpoints fit 1 MiB.
- No producer, protocol advertisement or existing snapshot reader is changed in this work order.

## Verification

Run from the repository root. Add the regression tests and show their expected
red result before making production changes. Commands below define the required checks; actual results are recorded below. Run PostgreSQL only against an isolated configured test database.

```bash
(cd apps/backend && go test -trimpath -race ./internal/agentctl/types/streams -count=1)
(cd apps/backend && go test -trimpath ./internal/agentctl/server/adapter/transport/acp -run 'Shell|CommandEcho' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agentctl/types/streams/shell_output_patch.go apps/backend/internal/agentctl/types/streams/shell_output_patch_test.go
```

Run `bash scripts/lint-go-changed` with every changed Go file in this task before
completion, plus `git diff --check`. Record those exact paths/results here.

## Files likely touched

- `apps/backend/internal/agentctl/types/streams/agent.go`
- `apps/backend/internal/agentctl/types/streams/tool_payload.go`
- `apps/backend/internal/agentctl/types/streams/shell_output_patch.go` (new)
- `apps/backend/internal/agentctl/types/streams/shell_output_patch_test.go` (new)

## Dependencies

None.

## Risks

Ambiguous repeated-text overlap must be judged by exact reconstructed output, never inferred provider provenance.

## Parallelism

`sequential`. The primary session owns integration and shared status updates.

## Inputs

- [Requirement](../../specs/platform/requirements/incremental-shell-delivery.md).
- [System design](../../specs/platform/system-design/incremental-shell-delivery.md).
- [Plan](plan.md), including diagnosis, compatibility matrix and test names.
- [Decision record](../../decisions/2026-10-19-incremental-shell-delivery.md).
- Existing `adapter_shell_snapshot_test.go`, `shell_output_test.go`,
  `agent_delivery_test.go`, `message_payload_test.go` and `durable_delivery_stream_test.go`
  patterns in their respective implementation packages.

## Results

Implemented the dormant typed codec and event fields. The first behavioral RED
was `TestShellOutputPatchRoundTrip`: a minimal codec stub returned nil instead
of a patch. GREEN covers exact result metadata, saturated UTF-8 overlap,
malformed/conflicting baselines and payloads, no-ops and maximum checkpoint size.

- `go test -trimpath -race ./internal/agentctl/types/streams -count=1`: PASS.
- `go test -trimpath ./internal/agentctl/server/adapter/transport/acp -run 'Shell|CommandEcho' -count=1`: PASS.
- `bash scripts/lint-go-changed apps/backend/internal/agentctl/types/streams/agent.go apps/backend/internal/agentctl/types/streams/shell_output_patch.go apps/backend/internal/agentctl/types/streams/shell_output_patch_test.go`: PASS, zero issues.
No producer or version advertisement is activated.
