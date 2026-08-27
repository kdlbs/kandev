---
id: "01-pwa-overlay-capability"
title: "PWA overlay capability"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/pwa-window-controls-overlay/spec.md"
---

# Task 01: PWA overlay capability

## Acceptance criteria

- The Web app manifest requests `window-controls-overlay` first and keeps `standalone` as the fallback for browsers that do not support the capability.
- A hook owned by the root shell reports active title-bar geometry, responds to visibility and geometry changes, rereads after listener registration, and removes the listener on unmount. It stays inactive when the capability is missing.
- `AppShell` publishes the current overlay state and safe geometry through root-scope CSS variables. It does not change the ordinary browser layout or persist browser capability state.

## Verification

Run the command before production code changes and confirm the expected RED result. Run it again after GREEN and REFACTOR:

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- --run hooks/use-window-controls-overlay.test.tsx lib/browser/pwa-manifest.test.ts
```

## Possible files to change

- `apps/web/public/manifest.webmanifest`
- `apps/web/global.d.ts`
- `apps/web/hooks/use-window-controls-overlay.ts`
- `apps/web/hooks/use-window-controls-overlay.test.tsx`
- `apps/web/lib/browser/pwa-manifest.test.ts`
- `apps/web/src/app-shell.tsx`

## Dependencies

None.

## Parallel work

Run this task first. Task 02 consumes the root overlay state and CSS variable contract created here.

## Inputs

- The `What` and `Scenarios` sections of the spec
- The `PWA capability contract` and `Tests` sections of the plan
- `apps/web/src/app-shell.tsx`
- `apps/web/public/manifest.webmanifest`

## Risks

- Tests must fail on a behavior assertion. A missing module alone must not cause the RED result.
- Type declarations must cover only the browser API that Kandev consumes.

## Output contract

Report the RED and GREEN command results, changed files, final hook contract, blockers, and risks in the same session. Update this task and `plan.md` with the status and results.

## Results

- RED: `npx --yes pnpm@9.15.9 --filter @kandev/web test -- --run src/app-shell.test.tsx lib/browser/pwa-manifest.test.ts` produced two expected behavior failures. After the manifest test path was corrected, the failures were the missing AppShell overlay contract and the missing `display_override`.
- RED: `npx --yes pnpm@9.15.9 --filter @kandev/web test -- --run hooks/use-window-controls-overlay.test.tsx` produced two expected failures because `geometrychange` was not subscribed or removed.
- GREEN: The final focused Vitest suite passed four files and eight tests. It covers the manifest, AppShell state, overlay hook lifecycle, geometry reread, and theme integration.
- The implementation adds manifest opt-in, narrow browser types, root-shell overlay geometry variables, immediate post-registration geometry reread, and visibility lifecycle handling. It has no external side effects.
