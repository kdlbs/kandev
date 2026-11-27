---
id: "01-badge-projection"
title: "Correct integration enabled badge projection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-ENABLE-DISABLE-TOGGLE-001
acceptance_criteria:
  - AC-INTEGRATIONS-ENABLE-DISABLE-TOGGLE-001.3
  - AC-INTEGRATIONS-ENABLE-DISABLE-TOGGLE-001.9
system_design:
  - ../../specs/integrations/system-design/enable-disable-toggle.md
---

# Task 01: Correct Integration Enabled Badge Projection

## Summary

Filter the built-in badge projection by the row's saved workspace enable
preference. Keep existing connection predicates and plugin behavior, and read
GitLab status for the explicit row workspace.

## In scope

- Red-Green regression for connected GitLab whose stored toggle is false.
- All six built-ins, same-tab/storage event updates, re-enable and workspace isolation.
- Rendered badge proof and saved GitLab desktop flow in the existing E2E suite.
- Update misleading comments and delivery results.

## Out of scope

Provider operation gating, backend changes, new labels and responsive layout.
New provider operations, storage identities and interaction patterns.

## Acceptance

1. Connected GitLab with its saved toggle off has no Enabled badge; it stays
   listed while hide-disabled is off. Saving re-enable restores the badge.
2. Each built-in uses its row workspace's preference; GitLab status reads that
   workspace too. Storage/custom events update the projection without reload.
3. Unsaved drafts, unrelated workspace rows, plugins and provider features retain
   their existing behavior. No new mobile interaction or layout is introduced.

## ASCII UI preview

UI-01, desktop and phone; [combined preview](plan.md#ascii-ui-preview).
AC-INTEGRATIONS-ENABLE-DISABLE-TOGGLE-001.9:

```text
Before, saved off: GitLab [Enabled]
After, saved off:  GitLab
After, saved on:   GitLab [Enabled]
```

Existing row navigation and scroll owners remain. No new controls or copy.

## Verification

From repository root; install only if workspace dependencies are missing.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/domains/integrations/use-enabled-integrations.test.ts hooks/domains/integrations/use-integration-enabled.test.ts components/app-sidebar/sections/settings/use-settings-menu-branches.test.ts components/app-sidebar/sections/settings/settings-tree-render.test.tsx)
(cd apps/web && pnpm exec eslint hooks/domains/integrations/use-enabled-integrations.ts components/app-sidebar/sections/settings/integration-enabled.tsx components/app-sidebar/sections/settings/use-settings-menu-branches.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --host --project chromium tests/integrations/integrations-index-enabled-toggle.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/settings/mobile-settings-index.spec.ts tests/settings/mobile-settings-sidebar.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/hooks/domains/integrations/use-enabled-integrations.ts`
- `apps/web/hooks/domains/integrations/use-enabled-integrations.test.ts` (new)
- `apps/web/components/app-sidebar/sections/settings/integration-enabled.tsx`
- Rendered badge regression lives in `use-enabled-integrations.test.ts` with the
  hook regressions, reusing the same external connection fixtures.
- `apps/web/components/app-sidebar/sections/settings/use-settings-menu-branches.ts`
- `apps/web/e2e/tests/integrations/integrations-index-enabled-toggle.spec.ts`
- `apps/web/e2e/tests/settings/mobile-settings-index.spec.ts`
- `apps/web/components/settings/record-badges.tsx` (badge invariant comment).
- `docs/public/integrations.md` (saved-toggle badge behavior).
- This work order and `plan.md`.

## Dependencies

None. Reuse the existing toggle identity catalog and subscription reader.

## Risks

Saving is required before preferences change; preserve that boundary in tests.
Avoid using active workspace state to answer another workspace's GitLab row.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/enable-disable-toggle.md)
- [Design](../../specs/integrations/system-design/enable-disable-toggle.md)
- Existing tree render, branch visibility and integration toggle E2E suites.

## Results

Completed 2026-10-05 in the primary session without delegation.

- RED: the new hook/render suite failed all 10 cases before the fix, including
  connected GitHub/GitLab returning true with stored false and GitLab borrowing
  another workspace's status.
- GREEN: targeted Vitest command above passed 4 files, 65 tests. Rendered badge
  proof shares the hook suite's fixture, avoiding a duplicate component suite.
- ESLint passed for the changed hook, regression tests, badge/branch components,
  badge comment and E2E spec.
- `pnpm run typecheck`: passed.
- Managed host E2E command above rebuilt the backend, web and plugin fixture:
  2 Chromium tests passed (19.2s), including both providers' unsaved, saved-off
  and saved-on badge states. No standalone `make build-web` was needed because
  the managed runner performed the fresh build.
- Workspace dependency installation passed with the frozen lockfile.
- Specification catalog/lint and diff checks passed.
- The phone `/settings` index renders the same badge rows through
  `SettingsPageNav` and `SettingsTree`. Its desktop sidebar remains hidden.
  The initial claim that the phone lacked equivalent badges was incorrect and
  is superseded by the phone review remediation below.
- Node/pnpm were available after adding the installed Node directory to PATH:
  `export PATH=/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH`.
- PR capture: one disposable Chromium test passed (9.6s) against the fresh
  build; a synthetic desktop screenshot shows both saved toggles off and
  no Enabled badges. Temporary capture spec removed.
- No delegation. Commit and PR delivery follow these checks.

## Review remediation results

Completed locally: removed the unused GitLab availability mock, corrected the
phone surface contract in the requirement/design/plan, and added real phone
Settings-index badge coverage with a fresh synthetic screenshot. Sentry
already incorporates its toggle; the uniform projection filter remains
intentional. No runtime or layout changes; the previous desktop screenshot
still represents current behavior.

- Four targeted Vitest files: 65 tests passed after mock removal.
- Changed regression and phone E2E file ESLint: passed.
- `pnpm run typecheck`: passed.
- The mobile E2E command above: six tests passed against the already fresh
  production build; no production code changed during remediation.
- Specification catalog/lint and diff checks: passed.
- Phone screenshot validated against visible GitHub/GitLab rows, captured with
  synthetic E2E data, settled animations and rows clear of the floating search.
- Current-head CI/review evidence remains externally pending after push.
