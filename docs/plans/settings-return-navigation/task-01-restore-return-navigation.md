---
id: "01-restore-return-navigation"
title: "Restore Settings return navigation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SETTINGS-RETURN-001
acceptance_criteria:
  - AC-UI-SETTINGS-RETURN-001.1
  - AC-UI-SETTINGS-RETURN-001.2
  - AC-UI-SETTINGS-RETURN-001.3
  - AC-UI-SETTINGS-RETURN-001.4
  - AC-UI-SETTINGS-RETURN-001.5
  - AC-UI-SETTINGS-RETURN-001.6
system_design:
  - ../../specs/ui/system-design/settings-return-navigation.md
---

# Task 01: Restore Settings return navigation

## Summary

Implement one shared Settings return destination. Use it in the desktop sidebar
and a phone header close action. Retain the existing navigation guard.

## Inputs

- [Requirements](../../specs/ui/requirements/settings-return-navigation.md)
- [System design](../../specs/ui/system-design/settings-return-navigation.md)
- [Plan and evidence](plan.md)
- Existing footer tests, client-router tests, Settings index E2E, and
  `AppNavSurface` close-button presentation.

## In scope

Own the provider, existing history-wrapper notifications, transition helper, footer integration, phone header action,
and their tests. Use `/tdd`, `/e2e`, `/mobile-parity`, and the requested
`better-ui` skill. Keep changes sequential.

## Out of scope

Backend changes, persistence, custom browser history formats, task/session
mutations, new Settings entry menus, and unrelated visual changes.

## Acceptance

1. Desktop and phone close actions restore the complete originating URL and
   visible task/session after nested Settings navigation, without a document reload.
2. Cancelled navigation retains context and drafts. Direct loads use the existing
   Home fallback. Repeated visits replace stale origins and preserve explicit Home actions.
3. Phone controls satisfy the preview, accessible-name, target-size, and containment
   requirements. All targeted tests pass, including existing Settings behavior.

## UI preview

See the [combined previews](plan.md#ui-preview).

```text
UI-01 desktop: Settings sidebar [Close settings] -> originating task
UI-02 phone:   Home / Settings              [X] -> originating task
               Existing tree/form scroller
```

UI-01 covers .1-.5. UI-02 additionally covers .6. Keep Home and Settings
breadcrumbs meaningful. The phone close target is at least 44px square and
must fit its header at 767px with a fine pointer. Compose existing integration
header actions beside it. Do not add desktop-sized empty header space.

## Implementation sequence

1. Add a failing regression named `returns to the originating task after closing settings`.
   Use actual provider/router transitions so the test cannot pass with an invented return URL.
2. Add helper and provider coverage for full URL context, rapid redirects,
   direct loads, cancelled guards, history, silent `replaceTaskUrl` session
   changes, and a second visit from another task.
3. Implement the smallest shared tracking and close behavior described by the design.
4. Add desktop and phone E2E scenarios from the plan. Demonstrate failure before
   the correction, then rerun against freshly built assets.
5. Record exact results and documentation impact. Promote the paired specs only
   after the implementation satisfies them. Mark this work order done and the plan implemented.

## Verification

Install dependencies once from `apps/` if this worktree lacks its own installation:

```bash
cd apps
pnpm install --frozen-lockfile
```

From `apps/web`:

```bash
pnpm exec vitest run lib/settings/settings-return-navigation.test.ts components/settings/settings-navigation-provider.test.tsx components/app-sidebar/app-sidebar-footer.test.tsx components/settings/settings-layout-client.test.tsx lib/routing/client-router.test.ts lib/routing/navigation-guard.test.ts src/app-shell.test.tsx
pnpm exec eslint lib/settings/settings-return-navigation.ts components/settings/settings-navigation-provider.tsx components/app-sidebar/app-sidebar-route.ts components/app-sidebar/app-sidebar-footer.tsx components/settings/settings-layout-client.tsx src/app-shell.tsx lib/routing/navigation-guard.ts
pnpm run typecheck
pnpm run i18n:check
pnpm e2e:run --project chromium tests/settings/settings-gear-only.spec.ts
pnpm e2e:run --project mobile-chrome tests/settings/mobile-settings-return.spec.ts tests/settings/mobile-settings-index.spec.ts
```

Run E2E commands sequentially. The managed runner builds fresh assets. Record
test discovery and results. Reuse `sidebar:closeSettings` for the accessible
name. Any additional copy requires all supported translations.

From the repository root after specification lifecycle updates:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/lib/routing/navigation-guard.ts` and its new test.
- `apps/web/lib/settings/settings-return-navigation.ts` and its new unit test.
- `apps/web/components/settings/settings-navigation-provider.tsx` and its new test.
- `apps/web/src/app-shell.tsx` and `app-shell.test.tsx` if provider setup requires it.
- `apps/web/components/app-sidebar/app-sidebar-route.ts`.
- `apps/web/components/app-sidebar/app-sidebar-footer.tsx` and its test.
- `apps/web/components/settings/settings-layout-client.tsx` and its test.
- `apps/web/e2e/tests/settings/settings-gear-only.spec.ts`.
- `apps/web/e2e/tests/settings/mobile-settings-return.spec.ts` (new).
- Paired requirement/design lifecycle metadata and this plan's delivery records.

## Dependencies

None. The user authorized implementation on 2026-10-21.

## Risks

Do not consume context when the guard cancels a close. Do not let Settings
redirects overwrite the origin. Avoid provider remounts during navigation and
viewport changes. Keep the existing workspace-aware fallback and route loading.

## Parallelism

Sequential. No delegation is authorized.

## Results

Completed on 2026-10-21. The original footer regression failed on the Home URL
before implementation. All 72 targeted unit tests passed after implementation.
Desktop Playwright passed all 6 tests. Phone Playwright passed all 6 tests,
including draft cancellation and narrow fine-pointer geometry. Typecheck,
ESLint, localization checks, public documentation validation, and specification
validation passed. See the [plan results](plan.md#verification-results) for
commands, logs, environment overrides, and visual evidence.

The design and requirement now describe the implemented behavior. The user
requested commit, push, and a ready PR on 2026-10-22. No delegation was requested.

### Review follow-up

The cancelled Back/fragment regression belongs to AC-UI-SETTINGS-RETURN-001.3.
Run the existing targeted verification commands with the added provider test.
The focused regression failed on the original provider and passed after removing
its hashchange listener. The complete targeted unit selection passed 73 tests.

Post-remediation typecheck, lint, specification validation, and all 12 targeted
browser tests passed. See the plan for exact results and log paths.
