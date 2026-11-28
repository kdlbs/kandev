---
id: "03-verify-provider-option-reports"
title: "Verify provider option reports"
status: done
wave: 3
depends_on:
  - "02-reconcile-persisted-options"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001
acceptance_criteria:
  - AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.1
  - AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.6
  - AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.9
system_design:
  - ../../specs/platform/system-design/session-config-cross-agent-reconcile.md
---

# Task 03. Verify provider option reports

## Summary

Carry verified options for the active model through agentctl for any provider.
The adapter derives the evidence from what its protocol reports.

## In scope

- In ACP v1 and v2, verify the model, native session, settings generation, and
  option IDs from typed setup, setter replies, and updates. Mark a list complete
  when the report defines the complete state and the projection preserves every
  unique ID. A separate model field from the same provider report can identify
  the model when the option list has no model row.
- Carry the evidence on the existing setup state, correlated result, and
  `session_models` event. Preserve the distinction between a complete list,
  a verified subset, and missing evidence. Keep the evidence within the same
  settings revision as the projected state.
- Use Grok's existing provider metadata for reasoning options after its
  confirmed model switch. The metadata verifies the option it describes.
  Copied entries in the display list cannot supply evidence for other options.

## Out of scope

- Startup replay and saved option updates. Task 04 consumes the evidence.
- New option types, provider install changes, and changes to the public
  selector event.

## Acceptance

- Cursor and a second ACP v1 agent get the same evidence from the same typed
  report. ACP v2 delivers equivalent evidence for its active native session.
- A projection with missing IDs cannot remove saved keys. Acknowledgments,
  ambiguous models, and copied display options cannot verify settings for a
  new model. Existing Grok metadata verifies its model-specific reasoning
  setting when the confirmed model offers it.
- Older agentctl results without evidence fields still decode.

## Regression cases

Extend `TestCursorModelResultCarriesVerifiedCatalog` into
`TestModelResultCarriesVerifiedCatalog` with both agent IDs. The temporary
`TestGenericModelResultCarriesVerifiedCatalog` case returned a nil
`ModelState` for the same typed reply on the draft head.

Use the same ACP v2 wire fixture as `TestWireAdvertisedMCPAndTypedConfig` to
check setup, correlated model reply, and live update. The fixture includes a
supported boolean with value `false`. The ACP v1 test covers projection loss.
Extend Grok's existing model-switch tests for models with and without
Reasoning Effort. Keep their stale-session cases.
Exercise both legacy convergence setters after load and check that the
synchronous snapshot retains the display-only marker until the next load.
Check Grok's advertised default and its display fallback separately.

## Verification

Run from `apps/backend`.

```bash
go test -trimpath -tags fts5 -p=2 -count=1 -timeout=20m \
  ./internal/agentctl/server/adapter/transport/acp \
  ./internal/agentctl/server/adapter/transport/acpv2 \
  ./internal/agentctl/server/api \
  ./internal/agent/runtime/agentctl
go test -trimpath -tags fts5 -p=1 -count=1 ./internal/agentctl/types/streams
go test -trimpath -tags fts5 -p=2 -count=1 -timeout=20m \
  ./internal/agentctl/acpcompat \
  ./internal/agentctl/server/adapter/transport/acp
```

## Likely files

- `internal/agentctl/server/adapter/transport/acp/model_catalog.go`
  and `adapter_session.go`, `adapter_updates.go`, and `dialect_grok.go` in that
  package.
- `internal/agentctl/server/adapter/transport/acpv2/settings.go`,
  `session.go`, and `updates.go`.
- `internal/agentctl/types/streams/configuration_result.go`,
  `session_settings_policy.go`, and `agent.go`.
- `internal/agent/runtime/agentctl/agent.go` and `client.go`.

## Risk

Typed lists can lose an entry during projection. Verify the full ID set before
stating that the report establishes which IDs are absent.

## Results

The ACP v1 red check returned no evidence for the non-Cursor fixture. The
ACP v2 red check found no completeness marker on the setup event.
The ACP v1, ACP v2, agentctl API, runtime agentctl, and stream types package
suites passed after the evidence changes.

The review checks reproduced missing provenance after both convergence setters
and a Grok display fallback marked as verified. Both cases passed after the
fix in the [full review verification](plan.md#verification-results).
