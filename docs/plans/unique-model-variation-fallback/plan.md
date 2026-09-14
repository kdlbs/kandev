---
created: 2026-09-07
status: complete
requirements:
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-002
system_design:
  - ../../specs/agents/system-design/no-silent-model-fallback-01.md
  - ../../specs/agents/system-design/no-silent-model-fallback-02.md
legacy_specs: []
---

# Implementation Plan: Unique Model Variation Fallback

## Overview

Extend the central executor-authoritative model policy with one deterministic
inference step. Implement the runtime contract first, expose its stable decision
in the existing frontend surfaces, then prove and document the user outcome.

## Scope

### In scope

- Resolve a missing bare model to its only distinct advertised bracketed
  variation.
- Keep exact models and advertised explicit fallbacks at higher precedence.
- Persist a distinct warning outcome and reason without rewriting the profile.
- Present host results as advisory on desktop and touch interfaces.
- Test and document unique and ambiguous catalogs.

### Out of scope

- Provider-specific ranking of variation labels.
- Selection among multiple variations.
- Profile schema changes or saved-model migration.
- Office post-start route changes or mid-turn model switching.

## Technical approach

### Runtime policy

Add a pure unique-candidate helper in
`apps/backend/internal/agent/runtime/lifecycle/start_model.go`. It accepts the
requested ID and executor model IDs. It returns one distinct advertised ID only
when the structural contract matches.

For profiles with automatic fallback disabled, extend `applyStartModelPolicy`
with this order: exact request, advertised explicit fallback, unique variation,
provider current or default. Automatic-fallback profiles retain their legacy
no-selection behavior when the requested model is absent. Add
`unique_variation` to `ModelSelectionOutcome` and
`unique_variation_applied` to stable warning reasons.

Keep `EffectiveModel` as the inferred ID. Do not set `FallbackModel` for this
outcome. `publishModelSelectionWarningEvent` must therefore emit the new reason
without the legacy `session_model_fallback` compatibility event.

Initial launch, context reset, and workspace rebind already use this policy.
Targeted tests must prove that no separate path bypasses the new order.

### Frontend advisory

Add the mirrored pure matcher under `apps/web/lib/`. Its table tests use the
same contract cases as the Go helper. Backend lifecycle data remains the launch
authority.

Use the helper in `task-create-dialog-options.tsx` and
`settings/profile-model-fields.tsx`. One host variation names the possible
target while keeping the saved value and selectable profile. Zero or multiple
variations keep the current missing-model presentation.

Map `unique_variation_applied` in `status-message.tsx`. Add localized copy to
all required `settings` and `task` catalogs. Reuse the existing desktop tooltip,
touch drawer, responsive model selector, and task-chat status layout.

### Documentation

Update `docs/public/agents-and-profiles.md` and `docs/public/executors.md` with
the four-step order, one unique example, one ambiguous example, and the
unchanged saved-profile rule.

## Tests

- AC-002.1 and AC-002.2: Go policy tests prove exact and explicit-fallback
  precedence before one inferred variation, including the legacy
  auto-fallback path.
- AC-002.3 through AC-002.5: mirrored Go and TypeScript table tests prove zero,
  duplicate, multiple, case-different, bracketed, prefix, and malformed cases.
- AC-002.6: lifecycle event and React status-message tests prove the distinct
  outcome, effective model, absent explicit-fallback field, and durable copy.
- AC-002.7: task picker and profile editor tests prove advisory host behavior.
- AC-002.8: lifecycle integration tests prove launch, reset, and rebind parity.

## E2E tests

- AC-002.2 and AC-002.6: extend
  `apps/web/e2e/tests/session/model-mismatch-warning.spec.ts` to launch `opus`
  against one `opus[1m]`, show the effective model, and retain one warning after
  reload.
- AC-002.3: add an ambiguous catalog with `opus[270k]` and
  `opus[1m, fast]`; prove that Kandev does not select either variation.
- AC-002.7: extend the desktop and mobile settings mismatch specs. Prove that
  the profile remains selectable, the advisory is keyboard or tap accessible,
  and mobile has no horizontal document overflow.

## Work orders

- [x] [Task 01: Resolve one executor variation](task-01-runtime-resolution.md)
- [x] [Task 02: Present variation decisions](task-02-frontend-advisory.md)
- [x] [Task 03: Prove variation workflows](task-03-e2e-verification.md)
- [x] [Task 04: Document variation fallback](task-04-public-docs.md)

## Verification results

Backend verification passed: 2,364 lifecycle tests, 29 focused model-policy
tests, and 212 mock-agent tests. Frontend verification passed: 51 focused
unit tests, typecheck, lint, `i18n:check`, and `i18n:ratchet`. Desktop E2E
passed 5 tests and mobile E2E passed 3 tests. Public-document validation
passed 61 tests and validated 46 published pages. Specification lint passed.

## Risks

- Provider variation labels can change. Kandev must keep their content opaque.
- Host and executor catalogs can differ. Host preview logic cannot become a
  launch gate.
- Older frontends can render the new reason as unknown during mixed-version
  use. Requested and effective fields must remain useful.
- Duplicate or malformed catalog rows can make a naive candidate count unstable.
