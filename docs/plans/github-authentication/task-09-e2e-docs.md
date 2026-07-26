---
id: "09-e2e-docs"
title: "End-to-end coverage and public docs"
status: done
wave: 7
depends_on: ["08-frontend-settings"]
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 09: End-To-End Coverage And Public Docs

## Inputs

- Every spec scenario.
- Task 07 mock controls and Task 08 user interface.
- `docs/public/integrations.md`, `docs/public/configuration.md`, E2E and docs-maintainer skills.

## Acceptance

- Playwright covers cross-workspace isolation, legacy/new migration, CLI/PAT selection, App install
  callback validation, App-only/personal behavior, capabilities, suspension/revocation, and
  disconnect/reconnect on desktop and mobile.
- Screenshots confirm both identity sections and all status/action text fit without overlap at the
  supported viewports.
- Public docs explain local and hosted setup, App permissions/events, actor semantics, migration,
  callback/webhook configuration, secret files, and recovery without exposing example live secrets.

## Files Likely Touched

- `apps/web/e2e/tests/integrations/github-workspace-settings.spec.ts`
- `apps/web/e2e/tests/integrations/github-authentication.spec.ts` (new)
- `apps/web/e2e/helpers/api-client.ts`
- `docs/public/integrations.md`
- `docs/public/configuration.md`
- `apps/backend/internal/integrations/AGENTS.md` only if the shared integration pattern changes
- `apps/web/AGENTS.md` only if frontend conventions change

## Verification

```bash
cd apps/web && rtk pnpm exec playwright test --config e2e/playwright.config.ts e2e/tests/integrations/github-authentication.spec.ts --project=chromium
cd apps/web && rtk pnpm exec playwright test --config e2e/playwright.config.ts e2e/tests/integrations/mobile-github-auth-settings.spec.ts --project=mobile-chrome
```

Also validate every documented environment variable against the typed config and inspect the
captured desktop/mobile screenshots.

Completed on 2026-07-19. The desktop suite passed four scenarios and the dedicated mobile suite
passed two scenarios. Captures for App suspension, personal revocation, permission gaps, and narrow
viewport layout were inspected; controls and the fixed Config Chat action do not overlap. Public
configuration and integration docs were validated against the typed config. The organization App
installation, OAuth exchange, and webhook round trip still require a deployed staging instance
with real GitHub credentials.

## Output Contract

Report scenario coverage, screenshots inspected, documentation/config cross-check, tests run, files
touched, blockers, and any platform flows requiring a real GitHub App staging check.
