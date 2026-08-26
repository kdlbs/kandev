---
spec: "../../specs/platform/requirements/feature-toggles.md"
status: done
created: 2026-08-04
---

# Plan: Restore the Feature Toggles restart action

One task, four production files. The task detail lives in the sibling
[`task-01-restore-restart-action.md`](task-01-restore-restart-action.md).

## Root cause

[`apps/web/src/settings-routes.tsx:217`](../../../apps/web/src/settings-routes.tsx)
renders the Feature Toggles page as:

```tsx
<FeatureTogglesSettings initialFlags={[]} restartCapability={null} />
```

`RestartRequiredAlert`
([`feature-toggles-settings.tsx:207`](../../../apps/web/components/settings/system/feature-toggles-settings.tsx))
computes `const supported = capability?.supported === true`, so a `null` prop is
permanently `false`. The restart `<Button>` at line 221 is never rendered and the
`system:restartManualHint` copy is always appended.

Before the SPA migration, the former Next Feature Toggles page supplied the
capability from `fetchRestartCapability`. During the migration,
`settings-routes.tsx` inlined `FeatureTogglesSettings` but stubbed the prop to
`null` instead of porting that fetch. The former page was later removed during
the Next cleanup, leaving the SPA route as the only live composition. The
`useKandevRestart` flow calls only `fetchSystemInfo` and `requestRestart`; it
does not detect capability.

Introduced by `aee1846cd` *"feat: remove nextjs production runtime (#1389)"* —
the line was born stubbed when the Next→Vite port dropped the server-side fetch.

**Conditions:** every install whose backend reports restart support, which is the
normal case — [`launcher/supervisor.go:168`](../../../apps/backend/internal/launcher/supervisor.go)
sets `KANDEV_RESTART_ADAPTER=supervisor`. This contradicts ADR 0019, which states
the UI needs a restart path "without asking users to manually find and restart
the right process."

**Reproduction:** run any supervised `kandev`; confirm
`GET /api/v1/system/restart-capability` returns `{"supported":true,"mode":"supervisor"}`;
save any override with `requires_restart_to_apply` (for example `features.office`);
open Settings → System → Feature Toggles. The notice shows the manual hint with
no restart button. Verified on a production bundle (hashed `/assets/index-*.js`,
no `@vite/client`) served by the Go binary, not a dev server.

**Why tests missed it:**
[`feature-toggles-settings.test.tsx`](../../../apps/web/components/settings/system/feature-toggles-settings.test.tsx)
passes `restartCapability={null}` in seven places and `{supported: false}` in
one. It never passes `supported: true`, and it tests the *component*, never the
route that supplies the prop. A component-level test cannot catch this class of
bug, so the regression test must render the route.

## Scope

`settings-routes.tsx:217` is the only hardcoded `null`/`[]` prop in the settings
route table — a grep for `={null}|={[]}` returns that single line, so there are
no sibling instances to fix. `initialFlags={[]}` is harmless: the component
reloads flags itself on mount.

## Task 1 — Fetch restart capability in the SPA route

**Acceptance:**

- The Feature Toggles route resolves `restartCapability` from the running
  backend through the system-domain hook and passes it to `FeatureTogglesSettings`.
- Toggle cards render immediately; they do not block on the capability request.
  While capability detection is pending, the notice stays neutral. Manual
  guidance appears only after an unsupported or unavailable result.
- A failed or rejected capability request resolves to `null`, preserving today's
  manual-guidance behavior — fail closed, never a phantom button.
- The system-domain hook uses a `cancelled` guard so an unmounted route does not
  set state.
- With `{supported: true}`, the notice renders the restart action and omits
  `system:restartManualHint`. With `{supported: false}` or `null`, it renders the
  manual hint and no action.

**Files:** `apps/web/src/settings-routes.tsx`,
`apps/web/components/settings/system/feature-toggles-route.tsx`,
`apps/web/components/settings/system/feature-toggles-settings.tsx`, and
`apps/web/hooks/domains/system/use-restart-capability.ts` (production changes),
`apps/web/src/settings-routes.test.ts` or a new
`apps/web/src/settings-routes.feature-toggles.test.tsx` (route-level regression
test, via the exported `renderSettingsRoute`), the hook test, and desktop/mobile
Playwright specs.

**Regression test — must fail before the change:** render the
`/settings/system/feature-toggles` route with `fetchRestartCapability` mocked to
`{supported: true, mode: "supervisor"}` and a pending-restart flag; assert the
restart action is present. Against current `main` this fails because the route
passes `null`.

**Verification:**

```bash
cd apps/web && pnpm vitest run src/settings-routes.test.ts components/settings/system/feature-toggles-settings.test.tsx && pnpm run typecheck
```

**parallelism:** sequential

**Status: done.** The fix landed as a new
`apps/web/components/settings/system/feature-toggles-route.tsx` rather than an
inline component in `settings-routes.tsx`: inlining it pushed that file to 605
counted lines against the 600-line `max-lines` limit, so the route wrapper was
extracted beside the component it wraps. `settings-routes.tsx` now imports
`FeatureTogglesRoute` and no longer imports `FeatureTogglesSettings`,
`fetchRestartCapability`, or `RestartCapability`.

Recorded results:

- Regression test failed first for the expected reason — 2 failed / 2 passed,
  both `supported: true` cases failing because the route supplied `null`.
- `pnpm vitest run src/settings-routes.test.ts src/settings-routes.feature-toggles.test.tsx components/settings/system/feature-toggles-settings.test.tsx` → 3 files, 34 tests passed.
- `pnpm vitest run src/ components/settings/` → 97 files, 523 tests passed.
- `pnpm run typecheck` → clean.
- `pnpm exec eslint` on all changed files → 0 errors, 0 warnings.
- `pnpm run i18n:ratchet` → clean, guard allowlist intact (199 entries).
- Focused Feature Toggles Vitest suite (route, component, and capability hook) →
  3 files, 14 tests passed.

## Out of scope

- Retiring the 44 orphaned `async` server components under `apps/web/app/**`,
  including `app/settings/system/feature-toggles/page.tsx`. Each one is a silent
  invitation to repeat this bug: it looks authoritative, still typechecks, and
  can drift from the SPA route that actually renders its component. Auditing
  which are orphaned and deleting those is a separate change with its own blast
  radius; this fix must not depend on it.
- Any change to the backend restart adapter, `boot_id` polling, or ADR 0019.
- E2E coverage of the restart round-trip. A real restart under Playwright would
  bounce the worker's backend; the route-level unit test is the proportionate
  guard.

## Risks

- Low. One added client fetch on a settings route, already-typed API helper, and
  a fail-closed fallback that reproduces today's behavior on error.
- The restart button becomes reachable for the first time since #1389. The
  underlying `useKandevRestart` flow and the supervisor endpoint are unchanged
  and independently exercised, but this fix is what puts them in front of users —
  worth a manual restart on a supervised install before merge.
