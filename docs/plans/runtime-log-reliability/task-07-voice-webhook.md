---
id: "07-voice-webhook"
title: "Diagnose voice-plugin webhook failures"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001
acceptance_criteria:
  - AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.2
system_design:
  - ../../specs/platform/system-design/runtime-failure-attribution.md
---

# Task 07: Diagnose voice-plugin webhook failures

## Summary

Identify the plugin-owned cause of the observed 503 responses. Produce a versioned regression proposal in the dedicated plugin repository without changing host dispatch policy.

## In scope

- Generic host response/attribution tests for plugin-supplied 503.
- A pinned read-only checkout of `https://github.com/kdlbs/kandev-plugin-voice` and its local instructions.
- Synthetic provider results for missing configuration, credential refusal, capacity/rate limits, timeout, and successful transcription.
- `voice-evidence.md` with plugin version, source boundary, cause, response semantics, and proposed dedicated-repository regression.

## Out of scope

- Moving voice code into core, exposing keys/audio/transcripts, live or billable provider requests, plugin release, and deployment.

## Acceptance

- Host tests retain the plugin-supplied status and safe `plugin_response` origin. No response body or credential enters generic logs.
- The investigation identifies the owning response branch at a pinned plugin revision or records why evidence remains insufficient.
- The report separates configuration recovery, expected dependency failure, and implementation defect. A code repair needs the plugin's own concrete package and exact tests.

## Verification

Run this block from the repository root after the implementation result exists.
New test names below are required planned regressions, not claims of existing coverage.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/plugins -run 'TestWebhook' -count=1)
python3 scripts/list-docs.py validate
```

## Files likely touched

- `apps/backend/internal/plugins/handlers_webhook_lifecycle_test.go`
- `apps/backend/internal/plugins/handlers.go` (read only)
- `dedicated repository kdlbs/kandev-plugin-voice: inspect its manifest, declared webhook, dependency client, and tests after checkout`
- `docs/plans/runtime-log-reliability/voice-evidence.md (new)`

## Dependencies

None. Follow the plan's sequential priority order.

## Risks

- The current logs identify origin but do not include plugin error detail. Do not fabricate a causal diagnosis.
- No universal plugin test command is assumed. Record and run the repository's actual non-billable regression command after discovering its toolchain.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md), especially evidence, contract ownership, and completion rules.
- [Design](../../specs/platform/system-design/runtime-failure-attribution.md) and its linked requirements.

- Scoped backend/agentctl instructions for any touched package.
- Existing source and tests listed above. Preserve completed companion-package results.

## Results

Completed as an evidence-only investigation. See [voice-evidence.md](voice-evidence.md). Pinned source revision `ea92f43f8aae2568ff5e37faf7ea0922b59f2930` (manifest `0.1.1`) maps missing/blank configuration and host/configuration failures to 503, upstream HTTP errors to 502, transport failures to 500, and success to 200. The historical host event lacks the plugin version and branch details needed to attribute its 503. No repair was established or applied.

The host webhook attribution regression passed. The plugin Go and UI suites passed in a disposable archive with test-only local toolchain adjustments; all 57 UI tests passed. Synthetic handler probes confirmed 429 → 502 and provider timeout → 500. The plugin build passed with VCS stamping disabled for the source archive.
