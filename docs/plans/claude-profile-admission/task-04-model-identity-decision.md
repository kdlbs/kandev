---
id: "04-model-identity-decision"
title: "Qualify model identity evidence and choose the repair surface"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-002
  - REQ-AGENTS-PROFILE-DISCOVERY-003
  - REQ-AGENTS-HOST-CLI-002
acceptance_criteria:
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.1
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.3
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.5
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.8
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.10
  - AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.1
  - AC-AGENTS-PROFILE-DISCOVERY-003.3
  - AC-AGENTS-PROFILE-DISCOVERY-003.4
  - AC-AGENTS-HOST-CLI-002.3
system_design:
  - ../../specs/agents/system-design/no-silent-model-fallback-01.md
  - ../../specs/agents/system-design/no-silent-model-fallback-02.md
  - ../../specs/agents/system-design/profile-capability-discovery.md
  - ../../specs/agents/system-design/host-cli-model-discovery.md
---

# Task 04: Model identity evidence and decision prerequisite

## Summary

Determine an evidence-backed corrective outcome for #5183 without weakening exact-model identity.
This is a pending investigation/decision work order. It authorizes no corrective production or permanent test patch.
It does not gate the confirmed authentication fix.

## In scope

- Resolve the current managed Claude bridge/version and inspect its authoritative model/configuration representation.
  Reuse the pinned 0.100.0 source inspection in the evidence record. Refresh only evidence that changed or needs executor-specific qualification.
  Recheck the actual catalog contract and any explicit alias mapping under the complete authorized profile context.
  Use existing tests, disposable probes, upstream source at a pinned revision, or a user-supplied sanitized trace.
  Never inspect a live credential store or require a real model prompt for qualification.
- Keep observed facts separate from provider claims and hypotheses.
  The existing model-policy probe proves strict alias rejection and full-ID application only.
  The reporter's successful SSH automation remains external evidence, not a local reproduction.
- Produce a decision note with one of two outcomes:
  authoritative runtime-bound alias evidence supports a reviewed contract amendment, or evidence remains insufficient and explicit full-ID repair is the proposed UX.
- For alias admission, define identity provenance, context/version binding, ambiguity rejection, refresh invalidation, and all start/reset/resume consumers.
  Record the alternatives in a proposed ADR and update the owning requirements/designs for review before implementation.
- For full-ID repair, specify actionable pre-inference recovery and a profile-context selection advisory.
  Preserve custom-ID saving, host-advisory status, strict runtime rejection, explicit user selection, and saved fallback values.
  Do not present host absence as authoritative remote unavailability.
- Extend this plan with corrective work orders only after the model outcome is settled by evidence and user review.
  Include the exact desktop/phone previews, mobile exemplar, i18n keys/locales, targeted tests, public docs, and owned files for the selected surface.

## Out of scope

No acceptance of arbitrary `current_model_id`, family-prefix matching, alias guessing, automatic fallback/provider switching, profile rewriting, or automatic prompt retry.
No new remote-probe API or provider qualification project beyond this issue.
No implementation of an unresolved alias or settings-save policy.

## Acceptance

1. The decision record pins the bridge/profile/catalog evidence and states whether it proves alias equivalence.
   Unknown or ambiguous aliases remain rejected before inference. A full advertised ID succeeds without fallback.
2. The selected proposal preserves exact runtime authority and existing saved settings across selection/save/start/reset/resume.
   If it changes that contract, a proposed ADR and amended requirements/designs expose the change for review.
3. The package contains independently actionable corrective work orders for the selected outcome, or an explicit evidence/decision blocker without an invented patch.
   Any UI proposal includes localized desktop/mobile acceptance and exact E2E commands before implementation.

## Current facts and alternatives

`start_model.go:applyStartModelPolicy` checks exact string membership before `SetModel`.
It rejects unadvertised `sonnet` even when `CurrentModelID` is `claude-sonnet-5-5`.
The same policy applies the full advertised ID with strictness enabled and fallback disabled.
That behavior follows AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.1 and .10.

The baseline bridge package was inspected at git head `981bf9a28ac3afef26e7877a2a638814c34f1679`.
Its preference resolver includes substring and token-scoring matches.
Its model response exposes selected IDs and current ID without a qualified alias map.
Those heuristics do not establish the missing strict identity proof.
The remaining qualification concerns the actual executor context and the intended corrective experience.

Profile discovery already uses profile launch settings and exposes exact custom choices.
`ModelPicker` can retain an unadvertised Claude selection as a custom ID.
That behavior follows AC-AGENTS-HOST-CLI-002.3 and cannot be removed through a generic host-catalog save gate.
The recovery card already shows typed requested/effective-model details and a generic unavailable-model explanation.
The remaining UX gap must therefore be specified as an observable improvement, not assumed from the original runtime failure alone.

The conservative recommendation is explicit full-ID repair, with an advisory before save and an actionable preflight failure.
It does not require alias equivalence. It also does not fulfill the issue's possible request for save-time rejection by itself.
The user must review that distinction before the companion is claimed as a fix.
No current decision authorizes accepting an alias or blocking all host-unadvertised custom IDs.

## Verification

This prerequisite uses existing regression controls and disposable probes rather than new permanent tests.
Run from the repository root:

```bash
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle -run '^(TestApplyStartModelPolicyExecutorAuthority|TestStrictStartModelFailure.*|TestRequiredEmptyStartModelReportsSelectionMissing|TestStartModelCancellationAndDeadlineKeepTheirMeaning|TestStartModelTransportFailureIsNotReportedAsModelApplicationFailure|TestStartModelProviderRejectionStaysModelApplicationFailure|TestStartModelConsumersPreserveProviderReplyCause|TestStartModelStartupSuccessDispatchesPrompt)$' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/decisions docs/plans/claude-profile-admission
```

For a later corrective package, the TDD matrix must include:

- Full-ID success with strictness on, dormant fallback values retained, and no automatic fallback.
- Unadvertised alias, ambiguous alias, absent alias metadata, catalog-empty, strict mismatch, unsupported selection, transport loss, and cancellation.
- Profile-aware discovery using `ANTHROPIC_MODEL`, stale results, unavailable host runtime, and different host/executor catalogs.
- Actual executor evidence on start, fresh reset/rebind, and cold resume. Replacement evidence must not come from the prior session.
- Explicit saved-model repair, reload, omitted fields in partial saves, and preserved unrelated settings.
- A command that reaches the mock ACP prompt only after successful full-ID selection, with no automatic replay of a refused attempt.

The exact future frontend commands must name the selected surfaces' tests.
Likely existing controls are `profile-model-fields.test.tsx`, `profile-form-fields.test.tsx`, and `session-bootstrap-recovery-card.test.tsx`.
Use `chromium` and `mobile-chrome` as separate managed E2E runs for any new UI.
An alias contract change also needs pinned bridge qualification, beyond these host policy tests.

## Files likely read

- `apps/backend/internal/agent/runtime/lifecycle/{start_model,manager_profile,session}.go`
- `apps/backend/internal/agent/runtime/lifecycle/{manager_interaction,manager_workspace_rebind,manager_kubernetes_refresh}.go`
- `apps/backend/internal/agent/settings/controller/{profile_discovery,profile_crud}.go`
- `apps/backend/internal/agent/agents/claude_acp.go` and current managed runtime registration
- `apps/backend/internal/agentctl/server/adapter/transport/acp/session_models.go`
- `apps/web/components/settings/{profile-model-fields,profile-form-fields}.tsx`
- `apps/web/components/task/chat/session-bootstrap-recovery-{model,card,content}.ts` or `.tsx`
- Current model catalog types and canonical pinned bridge source

## Files owned for changes

Only this work order, `plan.md`, `evidence.md`, and any clearly proposed model ADR/spec/work orders created by this prerequisite.
There are no owned production or permanent test files until a later corrective work order exists.

## Dependencies

None. Re-resolve main, #4764, and current grouped/profile-discovery contracts before investigation.
Auth Tasks 01 through 03 may be reviewed and delivered without this decision.

## Risks

The bridge may expose no trustworthy alias map.
A host observation can differ from the actual SSH runtime and account.
The verified full-ID workaround proves a successful explicit selection, not equivalence between the saved alias and every matching family ID.
Earlier rejection can conflict with the active custom-ID/host-advisory contract.

## Parallelism

`sequential`

## Inputs

- [Canonical issue and verified workaround](https://github.com/kdlbs/kandev/issues/5183#issuecomment-6720279723)
- [Exactness ADR](../../decisions/2026-09-15-explicit-profile-model-strictness.md)
- [Provider catalog ADR](../../decisions/2026-11-03-provider-neutral-model-catalog.md)
- [Policy](../../specs/agents/system-design/no-silent-model-fallback-01.md) and [recovery](../../specs/agents/system-design/no-silent-model-fallback-02.md)
- [Evidence and limits](evidence.md)

## Results

Pending prerequisite. The present package deliberately does not claim an implemented or fully specified corrective fix for #5183.
