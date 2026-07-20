---
id: "15-onboarding-e2e-docs"
title: "Onboarding end-to-end coverage and public docs"
status: done
wave: 11
depends_on: ["14-github-app-onboarding-ui"]
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 15: Onboarding End-to-End Coverage and Public Docs

## Inputs

- Tasks 11-14 integrated behavior.
- Spec scenarios for manifest success/failure, source precedence, binding-safe deletion, workspace
  handoff, and mobile parity.
- E2E and docs guidance from `.agents/skills/e2e/`, `mobile-parity`, and `docs-maintainer`.

## Acceptance

1. Desktop E2E covers unconfigured setup, generated manifest submission, callback hot-enable,
   replay failure, environment read-only state, and deletion blocked by a workspace binding.
2. Pixel 5 E2E completes the same user value and asserts touch targets, viewport containment, scroll
   ownership, and no document horizontal overflow; desktop/mobile screenshots are inspected.
3. Public docs explain owner choice, HTTPS/tunnel prerequisites, environment precedence, webhook
   health, safe removal, GitHub.com scope, and deployment/workspace/personal identity boundaries.

## Files Likely Touched

- `apps/web/e2e/tests/settings/github-app-registration.spec.ts` (new)
- `apps/web/e2e/tests/settings/mobile-github-app-registration.spec.ts` (new)
- `apps/web/e2e/fixtures/backend.ts`
- `docs/public/integrations.md`
- `docs/public/configuration.md`
- `apps/backend/AGENTS.md` only if runtime/provider guidance changed

## Verification

```bash
cd apps/web && rtk pnpm e2e:run tests/settings/github-app-registration.spec.ts -- --project=chromium
cd apps/web && rtk pnpm e2e:run --no-build tests/settings/mobile-github-app-registration.spec.ts -- --project=mobile-chrome
```

## Dependencies

Task 14 and all backend dependencies it integrates.

## Output Contract

Report scenario results, screenshot paths/inspection, docs changed, files touched, commands run,
blockers, and the external real-GitHub staging gap. Mark this task `done` and update `plan.md` only
after both viewport projects pass.

## Results

- Chromium desktop: 4 scenarios passed, covering manifest submission, replay/callback handling,
  immediate availability, environment precedence, workspace handoff, and binding-safe removal.
- Pixel 5: 3 scenarios passed with 44px touch-target, viewport, scroll-owner, and horizontal-overflow
  assertions. The manifest handoff and workspace installation screenshots were inspected and are
  contained without overlap.
- Public docs were updated in `docs/public/configuration.md` and `docs/public/integrations.md`.
- A real GitHub.com manifest conversion and signed webhook delivery remain staging checks because
  CI uses the deterministic GitHub mock.
