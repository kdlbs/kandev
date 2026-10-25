---
id: "04-e2e-docs"
title: "Desktop and mobile E2E plus docs"
status: completed
wave: 4
depends_on: ["03-status-bar-item"]
plan: "plan.md"
spec: "../../specs/platform/requirements/lsp-file-intelligence.md"
---

# Task 04: Desktop and Mobile E2E Plus Docs

## Acceptance

- Production-build desktop E2E proves launched/long-running initialization, Stop availability, status-bar relocation, persistence, lifecycle use, and active-file scoping.
- Coarse-pointer tablet proves effective toolbar fallback and drawer geometry; phone proves no control, socket, or process even with the status-bar preference.
- Public docs explain Kotlin initialize/Gradle behavior, the absence of a universal ETA/automatic timeout, and placement fallbacks.

## TDD sequence

1. Extend the held-initialize and placement Playwright scenarios and confirm they fail against the production build for the expected missing copy/item.
2. Make only test-support adjustments required for deterministic browser-time advancement and setting restoration.
3. Run desktop, tablet, and phone scenarios; then update and validate public docs.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/lsp/lsp-file-intelligence.spec.ts
cd apps/web && pnpm e2e:run --no-build -- --project=mobile-chrome tests/lsp/mobile-lsp-file-intelligence.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/web/e2e/tests/lsp/lsp-file-intelligence.spec.ts`
- `apps/web/e2e/tests/lsp/mobile-lsp-file-intelligence.spec.ts`
- `apps/web/e2e/tests/lsp/lsp-e2e-helpers.ts`
- `apps/web/e2e/fixtures/fake-lsp-server.mjs` only if deterministic held-init evidence needs another observable event
- `docs/public/developer-tools.md`
- `docs/public/feature-status.md`

## Dependencies

Task 03.

## Parallelism

Sequential. E2E and docs verify the completed vertical slice.

## Inputs

- Spec initialization, placement, tablet fallback, and phone boundary scenarios.
- Existing fake Kotlin initialize hold/release controls and App status bar E2E surface.
- E2E skill requirements for production builds, setting restoration, and mobile geometry.

## Output contract

Record failing/passing Playwright evidence, screenshots or exact rendered verification, docs validation, files changed, blockers/risks, and update this task plus `plan.md`.

## Result

- RED: the desktop placement scenario timed out because the shared E2E status helper only located the editor-toolbar trigger. The first pass also exposed preview-tab retention and selector assumptions in the new scenario.
- GREEN: the production build passed all 13 desktop LSP scenarios in 1.3 minutes. This includes held initialization, status-bar persistence and active-editor scoping, protocol navigation, shared connections, secondary repositories, reload, crash recovery, archive cleanup, and connection capacity.
- Browser time advances deterministically past the 60-second threshold. The held Kotlin server remains in `starting`, shows possible Gradle-import guidance without an ETA or percentage, and keeps Stop enabled until the initialize response is released.
- The mobile Chromium project passed all 3 scenarios: phone creates no control, socket, or process; coarse-pointer tablet preserves `status_bar` while using the 44 px toolbar and contained drawer; mobile Editors guidance persists without horizontal overflow.
- Every E2E-modified user setting is restored in `finally`, and the shared helper now opens either status trigger.
- Public docs describe initialization semantics, possible Kotlin Gradle work, no automatic timeout/universal ETA, and responsive placement. All 58 public-doc validator tests and all 41 published pages pass validation.
- Review follow-up: the production-build desktop placement scenario now opens a binary `.kt` fixture, proves the static viewer is rendered, and proves `builtin:lsp` is absent before continuing through unsupported, non-file, and restored Monaco states.
