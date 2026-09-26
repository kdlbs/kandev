---
created: 2026-09-26
status: done
requirements:
  - REQ-WORKSPACES-REPOSITORY-SECRETS-001
system_design:
  - ../../specs/workspaces/system-design/repository-secrets.md
legacy_specs: []
---

# Secret binding guidance

## Overview

Issue #3982 reports that a new secret appears ready for agent use after saving, although the runtime only resolves explicit references. The Global Secrets description mentions executor profile variables but omits agent profiles and repository bindings. The Workspace description says secrets are available to repositories without explaining that a repository must bind them. Update the descriptions for both scopes so the required next step stays visible with an empty or populated list.

## Scope

### In scope

- State that saving a secret stores it and does not add it to a session environment.
- Name every supported binding location for Global secrets and the repository-only binding for Workspace secrets.
- Keep the guidance visible in both scopes on desktop and phone, in all supported locales.

### Out of scope

- Automatic secret injection or a change to runtime resolution.
- A per-secret “used by” list. The existing reference endpoint supports deletion safety, but a list needs its own loading, authorization, and stale-reference presentation contract.
- New navigation, binding editors, or secret-value disclosure.

## Technical approach

`apps/web/components/settings/secrets-settings.tsx` already renders scope-specific descriptions in `SettingsPageTemplate` and `SettingsGroup`. Update the existing `manageApiKeysAndCredentialsSecrets` and `workspaceSecretsDescription` values in `apps/web/src/locales/*/settings.json`. Keep the current component and routes. The Global text must correct the current executor-only description; the Workspace text must make repository binding explicit. Generate the Traditional Chinese pair with `pnpm run i18n:zh-hant` and preserve pseudo-locale coverage.

The existing runtime remains the authority for session environment construction: `appendAgentProfileDefinitions`, `executorProfileEnvironmentDefinitions`, and `repositoryEnvironmentDefinitions` in `apps/backend/internal/agent/runtime/lifecycle/environment_resolution.go`. No API or backend change is needed.

## ASCII UI preview

`UI-01: Global Secrets guidance`, Settings > General > Secrets. Current and proposed text region; the list and Add secret action retain their current positions.

```text
Current, desktop or phone
Global Secrets
Manage API keys and credentials. Secrets are ... injected ... via executor profile env vars.
[Add secret]  [secret list or empty state]

Proposed, desktop or phone
Global Secrets
Saving a secret stores it but does not add it to a session. Bind it in an agent
profile, executor profile, or repository environment to make it available.
[Add secret]  [secret list or empty state]
```

`UI-02: Workspace Secrets guidance`, Settings > Workspaces > [workspace] > Secrets.

```text
Current, desktop or phone
Secrets
Manage secrets available to repositories in this workspace. Shared profiles ...
[Add secret]  [secret list or empty state]

Proposed, desktop or phone
Secrets
Saving a secret stores it but does not add it to a session. Bind it to a
repository in this workspace. Shared profiles cannot use Workspace secrets.
[Add secret]  [secret list or empty state]
```

The text wraps within the existing settings content on phones; it adds no overlay or scroll owner. The binding distinction and persistent visibility are required by AC-WORKSPACES-REPOSITORY-SECRETS-001.13. Exact wording and line breaks are illustrative. The existing Settings page remains the closest phone composition.

## Tests

No new logic or API is introduced. `pnpm run i18n:check` verifies all six catalogs, placeholders, and pseudo-locale generation. The browser tests below verify the rendered guidance in both list states.

## E2E tests

- `apps/web/e2e/tests/settings/secret-binding-guidance.spec.ts` in `chromium`: Global and Workspace descriptions are visible before and after secret creation and name the correct binding locations without showing the secret value.
- `apps/web/e2e/tests/settings/mobile-secret-binding-guidance.spec.ts` in `mobile-chrome`: the same guidance remains readable in both scopes and list states at phone width without horizontal overflow.

Both tests cover AC-WORKSPACES-REPOSITORY-SECRETS-001.13. Write the first assertions and run them against the current copy before editing the catalogs; they must fail on the missing binding explanation.

## Work orders

- [x] [Task 01: Explain secret bindings in Settings](task-01-explain-secret-bindings.md)

## Verification results

Completed Task 01 and a follow-up review finding. The desktop and phone browser tests first failed on the missing session-binding explanation. A new assertion then reproduced the removed encryption-at-rest assurance. Both E2E specs pass after the Global descriptions retained that assurance alongside the binding guidance.

- `pnpm run i18n:zh-hant` and `pnpm run i18n:pseudo` completed.
- `pnpm run i18n:check` passed.
- `pnpm run typecheck` passed.
- `pnpm run build` passed.
- `make build` and `make e2e-plugin-package` passed to refresh artifacts after locale generation.
- `pnpm e2e:run --no-build --project chromium tests/settings/secret-binding-guidance.spec.ts` passed.
- `pnpm e2e:run --no-build --project mobile-chrome tests/settings/mobile-secret-binding-guidance.spec.ts` passed.

## Risks

- The current Global description claims executor-only injection; translations must remove that misleading restriction without implying automatic injection.
- Text can wrap poorly on phones. The mobile browser test must check containment and a rendered phone view.
- Existing public guidance in `docs/public/security.md` and `docs/public/executors.md` already explains bindings; the repair changes the Settings surface where the gap occurs.
