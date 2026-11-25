---
id: "02-setup"
title: "Route Antigravity setup actions"
status: complete
wave: 2
depends_on:
  - "01-local-runtime"
plan: "plan.md"
requirements:
  - REQ-AGENTS-ANTIGRAVITY-ACP-009
  - REQ-AGENTS-ANTIGRAVITY-ACP-010
  - REQ-AGENTS-ANTIGRAVITY-ACP-015
acceptance_criteria:
  - AC-AGENTS-ANTIGRAVITY-ACP-009.1
  - AC-AGENTS-ANTIGRAVITY-ACP-010.7
  - AC-AGENTS-ANTIGRAVITY-ACP-015.1
  - AC-AGENTS-ANTIGRAVITY-ACP-015.2
  - AC-AGENTS-ANTIGRAVITY-ACP-015.3
  - AC-AGENTS-ANTIGRAVITY-ACP-015.4
system_design:
  - ../../specs/agents/system-design/antigravity-acp-agent.md
---

# Task 02 Route Antigravity setup actions

## Summary

Show which runtime the built-in profile uses. Route each sign-in action to that runtime and refresh its model list after the attempt.

## In scope

- Project the backend's resolved runtime to the agent list and profile. Show CLI setup when needed and label the pinned standalone installation.
- Reuse the authorized agent login terminal for CLI consent. Resolve the absolute `agy acp --login` argv on the backend. Reject stale requests after variant changes.
- Reuse the standalone consent dialog in Settings and for saved standalone task recovery. Probe standalone after consent even when the CLI is the default.
- Bind cached status and models to the runtime variant that produced them. Return the variant with profile-probe responses and ignore stale snapshots after a host runtime change.
- Report no installed runtime when the only CLI candidate fails version qualification and no complete standalone bundle is available.
- Keep the phone flow touch-accessible. Use localized labels, status text, and errors in all supported locales.
- Update the README agent row and the installation and sign-in guidance in `docs/public/agents-and-profiles.md`.

## Scope boundary

This work order covers setup, status, and model refresh. Google's CLI installer and the pinned standalone install remain separate entry points.

## Acceptance

1. The list and profile display the selected runtime. A ready CLI shows models. A signed-out CLI opens the vendor's login command in the existing operator terminal.
2. Standalone selection and saved-session task recovery open standalone consent even when the host default is CLI. The flow probes that server after consent. A stale settings action returns to a fresh readiness check.
3. Settings never pair models or auth state with a different runtime variant; a refresh publishes the variant used for its models.
4. Phone and desktop can complete sign-in and select a model. Phone controls meet 44px touch targets and the login surface fits the viewport.

## ASCII UI preview

### UI-01 Antigravity profile needs sign-in

```text
Desktop
  Antigravity CLI
  Google sign-in required    [Sign in] [Refresh]

Phone
  Antigravity CLI
  Google sign-in required
  [              Sign in              ]
  [              Refresh              ]
```

The settings page scrolls as one column on phone. The existing terminal or consent dialog owns its internal scrolling. See [full views and states](plan.md#ascii-ui-preview) for AC-AGENTS-ANTIGRAVITY-ACP-009.1, AC-AGENTS-ANTIGRAVITY-ACP-010.7, and AC-AGENTS-ANTIGRAVITY-ACP-015.4.

## Verification

Write `TestAntigravityCLILoginUsesResolvedCommand` in loginpty. Cover executable paths with spaces, operator authority, and variant changes before start. Add `TestAntigravityStandaloneLoginWithCLIDefault` in hostutility. Render the signed-in and signed-out settings states with fake ACP and login fixtures.

```bash
(cd apps/backend && go test -trimpath ./internal/agent/loginpty ./internal/agent/hostutility ./internal/agent/settings/controller ./internal/agent/capabilities/handlers -run 'Antigravity' -count=1)
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web exec vitest run components/settings/antigravity-runtime-setup.test.tsx components/settings/agent-login-dialog.test.tsx components/task/chat/session-recovery-card-authentication.test.tsx)
pnpm --dir apps/web e2e:run --project chromium tests/settings/antigravity-runtime-setup.spec.ts
pnpm --dir apps/web e2e:run --project mobile-chrome tests/settings/mobile-antigravity-runtime-setup.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

Compare the rendered layouts with UI-01 and UI-02. In the phone test, assert the opener's hitbox, terminal or consent surface containment, and horizontal overflow. Verify that dismissal returns focus and triggers the model refresh.

## Files likely touched

- `apps/backend/internal/agent/settings/controller/agent_discovery.go` and its DTOs
- `apps/backend/internal/agent/loginpty/handlers.go` and `internal/agent/hostutility/antigravity_login.go`
- `apps/web/components/settings/installed-agent-card.tsx`, `profile-status-panels.tsx`, `profile-capabilities-row.tsx`, and `agent-login-dialog.tsx`
- Web settings API types, agent locale catalogs, and the two Playwright specs
- `README.md` and `docs/public/agents-and-profiles.md`

## Dependencies

Task 01 supplies the resolved runtime and readiness status.

## Risks

The user can install or replace the CLI while a settings page is open. Revalidate the selected runtime before creating a login terminal.

## Parallelism

`sequential`

## Inputs

- [Setup UI design](../../specs/agents/system-design/antigravity-acp-agent.md#setup-ui)
- [Agent login dialog](../../../apps/web/components/settings/agent-login-dialog.tsx)
- [Phone drawer precedent](../../../apps/web/components/kanban/mobile-menu-sheet.tsx)

## Results

The review regressions pass for stale runtime snapshots and profile probe variants, old CLI-only discovery, and saved standalone recovery when the host default is CLI. The added login and session-recovery suites pass 30 tests across 3 files; the Antigravity locale interpolation and pseudo catalog pass `i18n:check`. The prior focused web suite passes 29 tests; web typecheck, lint, and production build pass. Desktop and mobile setup E2E each pass 2 tests; the phone flow verifies consent routing, focus return, readiness refresh, touch-sized controls, and viewport containment. Public docs validation passes 63 checks across 56 pages; specification validation passes for 2,106 specs and 464 decisions.
