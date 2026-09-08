---
id: "01-contain-progress-text"
title: "Contain LSP project progress text"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/lsp-file-intelligence.md"
---

# Task 01: Contain LSP Project Progress Text

## Acceptance

- A long URL-, path-, or identifier-like server title/message stays fully readable inside the fixed-width toolbar and status-bar popovers without horizontal overflow.
- The shared coarse-pointer tablet drawer wraps the same text, remains within the viewport, retains one vertical scroll owner and touch-sized controls, and creates no document horizontal overflow.
- The focused Playwright checks fail on the current normal-wrapping behavior before the production change and pass after the minimal containment rule; LSP state and phone behavior remain unchanged.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm e2e:run tests/lsp/lsp-file-intelligence.spec.ts -- --grep "shows server-reported project work|persists status-bar placement"
cd apps/web && pnpm e2e:run --project mobile-chrome tests/lsp/mobile-lsp-file-intelligence.spec.ts -- --grep "contained tablet drawer"
cd apps/web && pnpm run typecheck
cd apps/web && pnpm exec eslint components/editors/lsp-status-button.tsx e2e/helpers/layout-assertions.ts e2e/tests/lsp/lsp-e2e-helpers.ts e2e/tests/lsp/lsp-file-intelligence.spec.ts e2e/tests/lsp/mobile-lsp-file-intelligence.spec.ts
git diff --check
```

The first desktop and tablet commands must be recorded once in RED against current CSS and again in GREEN after the frontend edit. The managed E2E runner rebuilds the production Vite and backend artifacts before each final run.

## Files Likely Touched

- `apps/web/components/editors/lsp-status-button.tsx`
- `apps/web/e2e/helpers/layout-assertions.ts`
- `apps/web/e2e/tests/lsp/lsp-e2e-helpers.ts`
- `apps/web/e2e/tests/lsp/lsp-file-intelligence.spec.ts`
- `apps/web/e2e/tests/lsp/mobile-lsp-file-intelligence.spec.ts`
- `docs/plans/lsp-progress-text-containment/plan.md`
- `docs/plans/lsp-progress-text-containment/task-01-contain-progress-text.md`

## Dependencies

None.

## Parallelism

`sequential`. The responsive cases share one production component, one fake-server fixture, and one layout assertion.

## Inputs

- `docs/specs/platform/requirements/lsp-file-intelligence.md`, especially the server-progress and responsive-surface scenarios.
- Confirmed reproduction: a 320px content box rendered the representative progress message at `scrollWidth: 502px`; applying `overflow-wrap: anywhere` reduced it to `320px` without truncation.
- Existing desktop progress and status-placement scenarios in `apps/web/e2e/tests/lsp/lsp-file-intelligence.spec.ts`.
- Existing tablet drawer geometry scenario in `apps/web/e2e/tests/lsp/mobile-lsp-file-intelligence.spec.ts`.
- Existing shared status body in `apps/web/components/editors/lsp-status-button.tsx` and layout assertions in `apps/web/e2e/helpers/layout-assertions.ts`.

## Output Contract

Report the RED failure, minimal CSS containment change, files changed, exact tests and counts, rendered desktop/tablet behavior, cleanup evidence, blockers, and risks. Reconcile this file's touched-file list with the final diff, replace `## Results`, set this task to `done`, and synchronize the plan checkbox, status, and verification results in the same conversation.

## Results

- RED:
  - Desktop toolbar and status-bar popovers both failed the new element-containment assertion with `scrollWidth: 506px` versus `clientWidth: 320px`.
  - The original artifact URL fit the 820px tablet drawer, so the shared fixture was extended with one long checksum-like identifier. The revised tablet test then failed the intended message assertion with `scrollWidth: 1406px` versus `clientWidth: 750px`.
  - An initial count of every computed `overflow-y: auto` descendant found two framework-level candidates; the assertion was narrowed to the drawer's existing `[data-vaul-no-drag]` scroll body before accepting the RED signal.
- GREEN:
  - Added inherited `overflow-wrap: anywhere` plus `min-width: 0` at the shared `ProjectProgress` boundary; full server text now wraps without clipping or truncation in both popovers and the tablet drawer.
  - Desktop focused production E2E: 2 passed.
  - Tablet focused production E2E: 1 passed, including the existing vertical scroll owner, 44px action, viewport bounds, drawer/message containment, and no document horizontal overflow.
  - `pnpm run typecheck` passed.
  - Focused ESLint passed with no output.
  - `git diff --check` passed.
- Files changed match `## Files Likely Touched`; the spec and plan were committed in the preceding design-package commit.
- Cleanup: managed E2E runs stopped their isolated runtime processes and repositories; no temporary source or tracked build output remains.
- Security/trust and external side effects: none. The fake server and seeded repositories remained local to worker-owned temporary directories.
