---
id: "01-opencode-reinstall-entry"
title: "Open the installer from the OpenCode card"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-OPENCODE-V2-001
acceptance_criteria:
  - AC-AGENTS-OPENCODE-V2-001.4
  - AC-AGENTS-OPENCODE-V2-001.13
  - AC-AGENTS-OPENCODE-V2-001.14
system_design:
  - ../../specs/agents/system-design/opencode-v2-adoption.md
---

# Task 01. Open the installer from the OpenCode card

## Summary

Put a Reinstall link on a configured OpenCode card when its managed runtime
is missing. The link opens the existing installer and puts OpenCode first.

## In scope

- Use the current catalog availability, saved OpenCode profiles, runtime
  source, install command, and existing agent management permission.
- Add the localized action and pass the agent hint to the existing catalog.
- Verify the user flow and document both ways to reach the installer.

## Out of scope

- Startup downloads, native runtime maintenance, and changes to the pinned version policy.
- New API endpoints and changes to install job semantics.

## Acceptance

1. When an operator can install agents and the configured OpenCode runtime is
   missing and managed, desktop and phone show Reinstall. Installed,
   native-only, and read-only cards continue to show their usual controls.
2. Reinstall opens the catalog with OpenCode first. The operator starts the
   existing job by selecting Install. On failure, the OpenCode installer
   shows the error and allows another attempt.
3. After a successful install and discovery refresh, the configured card shows
   its saved profiles with the runtime available. The Reinstall link clears.

## ASCII UI preview

UI-01 and UI-02 from the [plan preview](plan.md#ascii-ui-preview).
These views cover AC-AGENTS-OPENCODE-V2-001.14.

```text
Desktop
OpenCode [Configured] [Not installed] [Reinstall] [v] [New profile]

Phone
OpenCode
[Configured] [Not installed]
[Reinstall] [v] [New profile]

Destination
Browse available agents
OpenCode
<selected install command>
[Install]
```

Keep Reinstall in the phone card's visible action group and measure the
rendered hit target. The catalog owns progress and failure controls.

## Verification

Add the desktop regression `reinstalls managed OpenCode from its configured card`
and the phone regression `retries managed OpenCode installation from Settings`.
The fixture supplies a saved OpenCode profile, a missing managed runtime, and an
installable target alongside another missing agent. The desktop test checks
eligibility, target ordering, the install request, and the refreshed
profile card. The phone test measures the Reinstall hit target, exercises
failure and retry from the catalog, and verifies that the runtime is detected.
Use route and job fixtures for the browser checks so npm access is unnecessary.

```bash
(cd apps/backend && go test -tags fts5 -trimpath ./internal/agent/agents ./internal/agent/settings/controller -run 'TestOpenCodeACPManagedIsInstalled|TestOpenCodeInstallPreparesSelectedManagedRuntime' -count=1)
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web run i18n:check)
(cd apps && pnpm --filter @kandev/web e2e:run --project chromium e2e/tests/settings/agents-browse-page.spec.ts)
(cd apps && pnpm --filter @kandev/web e2e:run --project mobile-chrome e2e/tests/settings/mobile-agents-browse-page.spec.ts)
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/app/settings/agents/page.tsx`
- `apps/web/components/settings/installed-agent-card.tsx`
- `apps/web/app/settings/agents/browse/page.tsx`
- `apps/web/components/settings/agents/agent-install-catalog.tsx`
- `apps/web/src/locales/*/agents.json`
- `apps/web/e2e/tests/settings/agents-browse-page.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agents-browse-page.spec.ts`
- `docs/public/agents-and-profiles.md`

## Dependencies

None.

## Risks

The runtime can become available while the catalog is opening. Preserve normal
catalog navigation and current discovery in that case. Use the existing
Settings scroll region so the reordered target is visible after navigation.

## Parallelism

`sequential`

## Results

Implemented the localized Reinstall action for eligible configured managed
OpenCode runtimes, routed it to the existing catalog with OpenCode first, and
documented the recovery path. Desktop and phone browser coverage confirms
eligibility, install retry, refreshed availability, and saved profile retention.
The follow-up checks arm the install responses and WebSocket subscription,
reload Settings after recovery, and confirm no profile mutation requests occur.

Verification passed: focused Go tests; web typecheck, lint, i18n completeness,
and formatting; desktop E2E (4 cases); mobile E2E (2 cases); public docs
validation (56 documents) and tests (63 cases); specification catalog and lint;
and `git diff --check`. Post-review desktop E2E passed 4 cases and phone E2E
passed 2 cases with the added reload and no-mutation assertions.
