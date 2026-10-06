---
id: "01-active-first-port-management"
title: "Active-first port management"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-PORT-FORWARDING-ACTIVE-FIRST-001
acceptance_criteria:
  - AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.1
  - AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.2
  - AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.3
  - AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.4
  - AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.5
  - AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.6
  - AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.7
  - AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.8
system_design:
  - ../../specs/ui/system-design/port-forwarding-active-first.md
---

# Task 01: Active-first port management

## Summary

Deliver active-first port rows, readable forwarding state and URL actions, and
phone parity. Implement the pure projection and dialog integration with TDD,
then prove the requested behavior in the existing desktop/mobile E2E suites.

## In scope

- Projection, deduplication, ascending group order, late tunnel visibility, and
  stopped target retention for the current visit.
- Active headings/count/status, dedicated URL priority, stable keyed row
  presentation, localized copy, and phone/coarse-pointer geometry.
- Scoped async reads/mutations, regression tests, deterministic E2E fixtures,
  focused screenshots, and a public how-to subsection.

## Out of scope

New runtime transports, settings, polling, persistence, bulk controls, global
style sweeps, and unrelated port-input validation changes.

## Acceptance

1. The projection and integrated list satisfy AC .1-.5, including deferred tunnel
   hydration and pending/failed/successful operations, without mutating input collections.
2. Desktop/phone rendered behavior satisfies AC .6-.7 and UI-01/UI-02, while
   existing proxy/Browser actions still pass; moved rows retain keyboard focus.
3. Session replacement and initial hydration races satisfy AC .8; all new copy
   passes locale gates and public usage docs describe the implemented groups.

## ASCII UI preview

Excerpt from the [full preview](plan.md#ascii-ui-preview), AC .1-.7:

```text
UI-01 Desktop
Forwarded ports (1)
9000 Manual Forwarding                    [Stop tunnel]
  Tunnel URL                              [Copy/Open]
  Proxy URL                               [Copy/Open/Browser]
Other ports
3000 Detected                             [Start tunnel]

UI-02 Phone
Forwarded ports (1)
9000 Manual Forwarding
Tunnel URL
[Open] [Copy] [Stop tunnel]
Proxy URL
[Open] [Copy]
Other ports
3000 Detected
[Start tunnel]
```

Retain a fixed title/dismiss control, one bounded content scroller, and manual
addition after rows. Phone actions have 44px hitboxes and safe-area clearance.
UI-03 in the full plan defines empty/loading/pending/failure composition.
Preserve the same row identity across heading boundaries. Compare saved
desktop/phone screenshots with these structural choices before recording success.

## Verification

From repo root; run desktop and mobile commands sequentially. Managed runners
build current sources and enforce the existing resource budget. A fresh worktree
needs `(cd apps && pnpm install --frozen-lockfile)` once before package commands.

```bash
(cd apps/web && pnpm exec vitest run components/task/port-forward-rows.test.ts components/task/port-forward-dialog.test.tsx components/task/port-forwarding-visibility.test.ts components/task/port-forwarding-visibility-provider.test.tsx)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/port-forward-dialog.tsx components/task/port-forward-dialog-actions.tsx components/task/port-forward-list.tsx components/task/port-forward-rows.ts components/task/use-tunnel-actions.ts e2e/tests/session/port-forward-dialog.spec.ts e2e/tests/session/mobile-port-forwarding.spec.ts e2e/tests/session/port-forwarding-helpers.ts e2e/pages/session-page.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && PATH=/usr/local/go/bin:$PATH NODE_OPTIONS=--max-old-space-size=4096 pnpm e2e:run --project chromium tests/session/port-forward-dialog.spec.ts)
(cd apps/web && PATH=/usr/local/go/bin:$PATH NODE_OPTIONS=--max-old-space-size=4096 pnpm e2e:run --project mobile-chrome tests/session/mobile-port-forwarding.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use the repo PR documentation coverage preflight for this work order and its
linked artifacts. Record actual command results and discovered E2E test counts.

## Files likely touched

- `apps/web/components/task/port-forward-rows.ts` and `.test.ts` (new).
- `apps/web/components/task/port-forward-dialog.tsx` and `.test.tsx`.
- `apps/web/components/task/port-forward-dialog-actions.tsx`;
  `port-forward-list.tsx` (extracted presentation). The existing
  `use-tunnel-actions.ts` mutation path is unchanged.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/task.json`.
- `apps/web/e2e/tests/session/{port-forward-dialog,mobile-port-forwarding}.spec.ts`;
  `port-forwarding-helpers.ts` (new shared fixture); `e2e/pages/session-page.ts`.
- `docs/public/tasks-and-workflows.md` and this package's statuses/results.

## Dependencies

None. Read the primary design package after explicit implementation instruction.

## Risks

Stable row identity, delayed snapshot/mutation scope, and URL/action containment
need behavioral assertions. Do not mistake stubbed handlers or screenshots for
proof that Start/Stop updates the integrated projection.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/port-forwarding-active-first.md).
- [System design](../../specs/ui/system-design/port-forwarding-active-first.md).
- `apps/web/AGENTS.md`, `/tdd`, `/mobile-parity`, `/e2e`, `/docs-maintainer`.
- Existing dialog/component tests, `useTunnelActions`, `PortUrlActions`, and
  `e2e/helpers/ws-response-hold.ts` for correlated-response fixtures.

## Results

Implementation authorized on 2026-10-06 and completed in the primary session.

- Projection RED reproduced missing tunnel-only rows and incorrect ordering;
  integrated RED reproduced pending/success/failure ordering, stale hydration,
  and cross-session leakage. A further RED proved that skipping the entire late
  snapshot lost untouched tunnels beside a newer Start. GREEN merges per target.
- Four targeted Vitest files passed: 26 tests. The component suite uses the real
  mutation hook with deferred network boundaries; session replacement is tested
  here. Browser coverage adds distinct WS transport, keyboard, and phone risks.
- Targeted ESLint passed with zero warnings; typecheck passed with a 6144MB Node
  heap (the default heap exhausted memory). Full i18n checks and the new-copy
  ratchet passed. All seven real catalogs and pseudo include the three new keys.
- Mobile Chrome passed both tests, including actual 44px controls, active order,
  Copy/Stop/restart, internal scrolling/manual addition, overflow, and focus return.
  Desktop coverage completed: the latest full suite passed 15/16, and the
  remaining case passed its isolated rerun in a fresh browser (1/1). Both new
  ordering/keyboard and late-hydration cases passed in the full run. Browser-panel
  regressions passed as well. All 16 desktop cases have passing final evidence.
- Desktop and phone screenshots were inspected against UI-01/UI-02: active
  group/count/status, dedicated URL priority, compact desktop controls, stacked
  phone URLs/actions, bounded scrolling, and containment match the design.
- Public documentation was updated in `docs/public/tasks-and-workflows.md`.
  The public validator passed all 47 pages and its test suite passed 62 tests.
  Catalog validation passed (351 decisions, 1354 specifications), specification
  lint passed, and its suite passed 36 tests. PR documentation coverage returned
  `covered` with no errors using all actual modified and untracked files.

Verification environment: native Vite crashed in one rebuild and Chromium
crashed during task setup in otherwise passing desktop runs. The final desktop
commands reused the successful build of the same unchanged production source;
managed freshness and resource guards remained enabled:

```bash
(cd apps/web && PATH=/usr/local/go/bin:$PATH NODE_OPTIONS=--max-old-space-size=4096 pnpm e2e:run --host --no-build --project chromium tests/session/port-forward-dialog.spec.ts)
(cd apps/web && PATH=/usr/local/go/bin:$PATH NODE_OPTIONS=--max-old-space-size=4096 pnpm e2e:run --host --no-build --project chromium tests/session/port-forward-dialog.spec.ts --grep 'manual port row has correct proxy URL')
```

The initial keyboard dismissal check sent Escape twice during a tooltip closing
animation. It now waits for the tooltip layer to unmount before dismissing the
parent dialog and asserting focus restoration. No production dismissal change
was needed.

At the implementation handoff, no runtime/API changes, delegation, commit, push,
or PR had been made. Delivery proceeds separately when authorized.
