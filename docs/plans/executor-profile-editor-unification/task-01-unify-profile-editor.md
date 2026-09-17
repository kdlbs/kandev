---
id: "01-unify-profile-editor"
title: "Unify profile navigation and bookmark handling"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PROFILE-EDITOR-001
acceptance_criteria:
  - AC-EXECUTORS-PROFILE-EDITOR-001.1
  - AC-EXECUTORS-PROFILE-EDITOR-001.2
  - AC-EXECUTORS-PROFILE-EDITOR-001.3
  - AC-EXECUTORS-PROFILE-EDITOR-001.4
  - AC-EXECUTORS-PROFILE-EDITOR-001.5
  - AC-EXECUTORS-PROFILE-EDITOR-001.6
  - AC-EXECUTORS-PROFILE-EDITOR-001.7
system_design:
  - ../../specs/executors/system-design/profile-editor.md
---

# Task 01: Unify profile navigation and bookmark handling

## Summary

Make every profile entry point open the existing complete editor. Replace valid
legacy bookmarks without losing ownership checks, query parameters, or fragments.

## In scope

1. Add the failing canonical-route regression before production changes.
2. Reduce the profile helper to one encoded profile ID and migrate its callers.
3. Replace the reduced form with bookmark compatibility and localized recovery.
4. Add route, navigation, save, and phone regression coverage.
5. Update public executor documentation with one sentence about equivalent entry points.

## Out of scope

Runtime changes, data migrations, new profile fields, and connection-page redesign.

## Acceptance

- All production profile navigation uses the canonical helper. Exactly one editor owns profile form state and persistence.
- Legacy profile routes validate ownership, preserve suffixes, and replace history. Invalid pairs never mount an editor or mutate data.
- All referenced criteria pass their targeted checks, including desktop and phone save/reload, discard, and existing permissions.

## Implementation sequence

Start with the failing helper regression described in the plan. Extend
`LegacyExecutorSettingsRoute` tests with local Docker, SSH, Sprites, local,
worktree, remote Docker, and Kubernetes cases. Cover missing executors, missing
profiles, a foreign profile beside a valid profile, and later store hydration.
The foreign-profile case must not select the executor's first profile.

Retain existing executor-only Kubernetes recovery and non-Kubernetes connection
behavior. Cover encoded IDs, query/hash preservation, and router replacement.

Migrate the helper and hardcoded production links. Update obsolete assertions
that expect the singular profile route. Preserve tests for old bookmarked URLs.
Use `rg` to inventory callers and literals after the migration.

Keep the plural editor as the form owner. Reduce the singular page to a small
adapter or unavailable state. Do not retain its save contributor or duplicate
field components. Do not remove an i18n guard entry to silence path checks.

Add the two routing E2E files described in the plan. Reuse Docker response mocks
from `docker-profile-persistence.spec.ts` and mobile settings-index navigation.
Use actual clicks or taps for navigation evidence. Add
`components/settings/executor-profile-navigation.test.tsx` for hub and
task-disclosure navigation. Extend `lib/settings-discovery/catalog.test.ts`
for discovery destinations.

The documentation change belongs in `docs/public/executors.md`, a reference
page. Preserve connection-page guidance. No new public page is required.

## ASCII UI preview

Excerpt from [UI-01 through UI-03](plan.md#ascii-ui-preview), covering criteria
.1 through .6:

```text
UI-01 desktop: Settings tree | Profile > Details > Docker build > Policies
UI-02 phone:   Settings > Executors > Profile
               Details
               Dockerfile + image tag + [Build Image]
               Credentials / environment / scripts / MCP
               [Save changes] when dirty
UI-03 invalid: Profile not found > recovery link
```

The phone uses direct navigation and one content scroll region. The existing
floating save control retains safe-area clearance. No desktop sidebar appears.
Use the full plan preview for structural requirements and type applicability.

## Verification

Run from the repository root. Install dependencies once if this is a fresh
worktree. Run desktop and mobile E2E commands sequentially.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/settings/executor-settings-routes.test.ts components/settings/legacy-executor-settings-route.test.tsx components/settings/executor-profiles-card.test.tsx components/settings/executor-profile-navigation.test.tsx components/app-sidebar/sections/settings/settings-menu-branches.test.ts src/settings-routes.test.ts src/settings-route-helpers.test.ts lib/settings-discovery/catalog.test.ts components/settings/settings-layout-client.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/executor-profile-routing.spec.ts tests/settings/docker-profile-persistence.spec.ts tests/settings/kubernetes-executor.spec.ts tests/settings/ssh-profile-connection-link.spec.ts tests/settings/executor-agent-config.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-executor-profile-routing.spec.ts tests/settings/mobile-kubernetes-executor.spec.ts tests/settings/mobile-ssh-profile-connection-link.spec.ts tests/settings/mobile-executor-agent-config.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run ESLint on every changed TS/TSX file with `pnpm exec eslint --max-warnings 0`
from `apps/web`, using explicit paths from the final diff. Add exact commands
for any further changed test suite to Results before marking this task done.

## Files likely touched

All paths below are relative to `apps/web`, except the public document.

- `lib/settings/executor-settings-routes.ts` and new `.test.ts`.
- `components/settings/legacy-executor-settings-route.tsx` and `.test.tsx`.
- `app/settings/executor/[id]/profile/[profileId]/page.tsx`.
- `app/settings/executors/page.tsx`.
- New `components/settings/executor-profile-navigation.test.tsx`.
- `app/settings/executors/new/[type]/page.tsx`.
- `app/settings/executors/new/[type]/ssh-create-page.tsx`.
- `app/settings/executors/new/[type]/kubernetes-create-page.tsx`.
- `app/settings/executors/k8s/[executorId]/page.tsx`.
- `components/settings/executor-profiles-card.tsx` and `.test.tsx`.
- `components/app-sidebar/sections/settings/settings-menu-branches.ts` and `.test.ts`.
- `components/task/executor-environment-disclosure.tsx`.
- `components/task-create-dialog-form-body.tsx`.
- `lib/settings-discovery/resolve.ts` and `catalog.test.ts`.
- `src/settings-routes.test.ts`, `src/settings-route-helpers.test.ts`.
- `components/settings/settings-layout-client.test.tsx`.
- New `e2e/tests/settings/executor-profile-routing.spec.ts` and `mobile-executor-profile-routing.spec.ts`.
- `docs/public/executors.md` (repository-relative).

The canonical editor needs no form redesign. Change it only if a small shared
unavailable-state extraction is necessary.

## Dependencies

None. Implementation requires a later explicit user request.

## Risks

See the plan's ownership, compatibility, permissions, and test-entry-point risks.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/profile-editor.md).
- [System design](../../specs/executors/system-design/profile-editor.md).
- Existing legacy route and profile-list component tests.
- Existing Docker persistence and mobile settings-sidebar E2E patterns.
- Repository `/tdd`, `/mobile-parity`, `/e2e`, and `/docs-maintainer` skills.

## Results

Pending. No implementation or regression test changes exist yet.
