---
id: "01-linked-remote-repository"
title: "Render a linked single remote repository"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-REMOTE-REPO-TOPBAR-001
acceptance_criteria:
  - AC-UI-REMOTE-REPO-TOPBAR-001.1
  - AC-UI-REMOTE-REPO-TOPBAR-001.2
  - AC-UI-REMOTE-REPO-TOPBAR-001.3
  - AC-UI-REMOTE-REPO-TOPBAR-001.4
  - AC-UI-REMOTE-REPO-TOPBAR-001.5
system_design:
  - ../../specs/ui/system-design/single-remote-repository-task-topbar.md
---

# Task 01: Render a linked single remote repository

## Summary

Derive one remote repository's display and browser destination from the
existing task and repository records. Render its provider icon, short name,
and link in both task topbars while preserving the task title action.

## In scope

- Topbar-only repository descriptor and safe browser URL helper.
- Desktop breadcrumb and phone header link, including provider icons and
  accessible names.
- Required locale keys and focused unit and Playwright coverage.

## Out of scope

- Changing stored repository data, task card labels, filters, or
  multi-repository selection.

## Acceptance

1. Exactly one remote repository renders as provider icon plus short name
   before the task title, with a separate usable task-title action on desktop
   and phone.
2. A validated HTTPS clone URL for a supported first-party provider whose
   path maps to its browser page opens in a new tab. Missing, unsafe, or
   unsupported plugin-provider destinations produce a static label, with no
   broken link.
3. No-, local-, and multi-repository tasks retain their current topbar
   behavior. The phone link stays touch-reachable without horizontal overflow.

## ASCII UI preview

`UI-01: Desktop task topbar` and `UI-02: Phone task topbar` from the
[full preview](plan.md#ascii-ui-preview) apply to this work order
(`AC-UI-REMOTE-REPO-TOPBAR-001.1` to `.5`).

```text
Desktop: [GitHub icon agent-orchestrator] > [Explain agent connections]
Phone:   [Back] [GitHub icon agent-...] > [Explain agent... v] [Actions]
                                                   branch / diff summary
```

The repo link and task action are independent controls. For a missing browser
URL, the repository label is static. Spacing and truncation are illustrative.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/page-topbar.test.tsx components/task/task-page-content-helpers.test.ts components/task/task-top-bar.test.tsx components/task/mobile/session-mobile-top-bar-repository.test.tsx components/task-create-dialog-remote-repo-provider-tabs.test.tsx lib/utils/remote-repository-browser-url.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run build)
(cd apps/web && pnpm e2e:run tests/task/remote-repository-topbar.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-remote-repository-topbar.spec.ts)
```

If this is a fresh worktree, first run `(cd apps && pnpm install
--frozen-lockfile)` once.

Targeted lint and design-package validation:

```bash
(cd apps/web && pnpm exec eslint components/page-topbar.tsx components/page-topbar-parent-crumb.tsx components/page-topbar.test.tsx components/task-create-dialog-remote-repo-provider-tabs.tsx components/task/task-page-content-helpers.ts components/task/task-page-content-helpers.test.ts components/task/task-top-bar.tsx components/task/task-top-bar.test.tsx components/task/mobile/session-mobile-top-bar.tsx components/task/mobile/session-mobile-top-bar-repository.test.tsx lib/utils/remote-repository-browser-url.ts lib/utils/remote-repository-browser-url.test.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `apps/web/components/task/task-page-content-helpers.ts`
- `apps/web/components/task/task-page-inner.tsx`
- `apps/web/components/task/task-layout.tsx`
- `apps/web/components/task/task-top-bar.tsx`
- `apps/web/components/page-topbar.tsx`
- `apps/web/components/page-topbar-parent-crumb.tsx`
- `apps/web/components/task/mobile/session-mobile-layout.tsx`
- `apps/web/components/task/mobile/session-mobile-top-bar.tsx`
- `apps/web/lib/utils/remote-repository-browser-url.ts` (new)
- `apps/web/src/locales/*/task.json`
- Focused existing unit tests and two task Playwright specs named above.

## Dependencies

None.

## Risks

- The repo name and title may collide on narrow phones; verify real element
  bounds and title reachability.
- Plugin hosts such as Bitbucket Server can use clone routes such as
  `/scm/TEAM/fixture.git` while their browser route is `/projects/TEAM/repos/fixture`.
  Keep these providers static until a contract supplies the authoritative
  browser URL.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/single-remote-repository-task-topbar.md)
- [System design](../../specs/ui/system-design/single-remote-repository-task-topbar.md)
- Existing task topbar, page topbar, mobile topbar, and repository-provider
  icon patterns.

## Results

Passed after review remediation. Six focused Vitest files reported 102 passing
tests, including the Bitbucket Server clone-route fallback and desktop/phone
provider registration and unregistration after mount. Frontend typecheck,
i18n validation, targeted ESLint, the production Vite build, specification
validation, and spec lint passed. The desktop Playwright scenario verified the
repository link, new-tab navigation, and title rename action. The Pixel 5
scenario verified the repository link, picker, 44px targets, truncation, and
no horizontal overflow. Both E2E scenarios passed with successful frontend and
backend builds. The phone structure was visually inspected in the earlier
implementation run; this remediation only changes provider metadata updates
and unsupported-provider link eligibility.
