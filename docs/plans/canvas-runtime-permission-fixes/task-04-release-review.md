---
id: "04-release-review"
title: "Make release review readable"
status: done
wave: 4
depends_on:
  - "03-runtime-startup"
plan: "plan.md"
requirements:
  - REQ-CANVASES-AGENT-WEB-APPS-006
  - REQ-CANVASES-AGENT-WEB-APPS-007
acceptance_criteria:
  - AC-CANVASES-AGENT-WEB-APPS-006.8
  - AC-CANVASES-AGENT-WEB-APPS-006.9
  - AC-CANVASES-AGENT-WEB-APPS-007.4
  - AC-CANVASES-AGENT-WEB-APPS-007.7
  - AC-CANVASES-AGENT-WEB-APPS-007.8
system_design:
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
---

# Task 04: Make release review readable

## Summary

Replace UUID-led, repeated release cards with a selected-release review and
visible actions. Share review state between a wider desktop dialog and a
focused, full-height phone surface.

## In scope

- Release date and lifecycle labels, with distinct Active and Previous states.
  Default to the pending release, otherwise active; retain release selection.
- Authorized source task title/session name projected by the existing backend
  response, plus safe deleted/inaccessible-source fallbacks. Keep identifiers
  for internal mutations only, never visible or accessible fallback labels.
- One readable permission summary, new-permission markers, and exact HTTPS
  destinations. Share labels and selection state with promotion review.
- Separate fixed header/footer and a single middle scroll region. Use a
  desktop dialog up to 48rem wide; remove the 288px body cap. Match the existing
  focused task route and mobile-menu-sheet safe-area mechanics on phones.
- Localized copy in all supported languages; focus restoration, keyboard
  access, 44px phone actions, mutation errors, loading, and stale selection.
- Preserve approval, rejection, rollback, promotion, and permission-revocation
  semantics. Unknown permission kinds remain explicit and cannot be silently
  approved through a friendly-label fallback.
- Update public release/permission guidance to match the shipped surface.

## Out of scope

Marketplace browsing, new permission kinds, authority changes, new task/session
navigation, canvas application layout, and altering historical release data.

## Acceptance

1. Backend tests prove source labels require current access and tolerate deleted
   sources. Frontend tests cover dates, statuses, permission groups, exact
   origins, unknown kinds, pending/default selection, and UUID-free fallbacks.
2. At 1280x720 the two-permission case fits without body scrolling. At 390x844,
   long content scrolls internally while actions remain visible and touch-sized.
   Test 767px/768px switching, keyboard focus, safe-area padding, and no overflow.
3. Browser tests exercise later-expansion approval and rejection, previous-release
   rollback, promotion, stale/revoked releases, loading, and failures on both
   surfaces. First-publication no-dialog behavior remains intact.

## ASCII UI preview

Excerpts of UI-02 and UI-03 in [the plan](plan.md):

```text
Desktop: wider dialog
+---------------------------------------------------+
| Releases and permissions                      [X] |
| [Release date - Pending v]                        |
| From: readable task / session                     |
| Read tasks                                New     |
| Read workflows                            New     |
|---------------------------------------------------|
| [Reject]                       [Approve and open] |
+---------------------------------------------------+

Phone: focused full-height surface
+-------------------------------+
| [< Back] Release permissions   | fixed header
| [Release date - Pending v]     |
| Readable source               | one scroll region
| Permissions / exact origins   |
|-------------------------------|
| [Reject] [Approve and open]    | fixed 44px actions
|         safe-area inset       |
+-------------------------------+
```

An active selection offers Close; a previous selection offers eligible rollback.
Keep loading/error/empty states in the same structure. These are shared states,
not separate mobile mutations. Restore focus to the opening control on close.

## Verification

Run from the repository root after adding the named regression file:

```bash
(cd apps/backend && go test ./internal/backendapp -run 'Canvas' -count=1)
(cd apps/web && pnpm exec vitest run lib/canvas-permission-copy.test.ts lib/canvas-lifecycle.test.ts lib/api/domains/canvas-api.test.ts components/settings/canvas-lifecycle-dialogs.test.tsx components/settings/canvas-host-route.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/canvas/plugin-canvas.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/canvas/mobile-plugin-canvas.spec.ts -- --retries=0)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run scoped ESLint on all changed frontend implementation files. Do not treat
mocked-dialog component tests as proof of layout. Assert real bounding boxes,
scroll ownership, focus, and action visibility in managed browser tests.

## Files likely touched

- `apps/backend/internal/backendapp/` canvas HTTP response mapping and canvas tests
- `apps/web/lib/api/domains/{canvas-api.ts,canvas-api.test.ts}` and canvas response types
- `apps/web/lib/{canvas-lifecycle.ts,canvas-lifecycle.test.ts}`
- New `apps/web/lib/{canvas-permission-copy.ts,canvas-permission-copy.test.ts}`
- `apps/web/components/settings/{canvas-lifecycle-dialogs.tsx,canvas-lifecycle-dialogs.test.tsx}` and extracted review components/hooks
- `apps/web/e2e/tests/canvas/{canvas-fixture.ts,plugin-canvas.spec.ts,mobile-plugin-canvas.spec.ts}`
- `apps/web/src/locales/` in all supported languages; generate Traditional Chinese with `pnpm run i18n:zh-hant`
- `docs/public/{canvases.md,security.md}`

## Dependencies

Task 03 supplies final startup and recovery states. Preserve its retry/release
entry points and task 02's distinction between initial and later permissions.

## Risks

Raw UUID fallbacks and unauthorized source labels can leak through accessible
names or tooltips. Test those surfaces too. Split the existing large dialog
module into cohesive helpers/components rather than increasing its complexity.

## Parallelism

`sequential`

## Inputs

- Canvas system design's Readable review surfaces section.
- `task-layout.tsx` focused composition and `mobile-menu-sheet.tsx` scroll/safe-area patterns.
- Existing release/promotion hooks and managed canvas E2E fixtures.

## Results

Implemented and verified.

- Added authorized task/session source-label projections to release and
  promotion responses. Inaccessible, deleted, or mismatched sources return a
  localized unavailable label without exposing identifiers.
- Replaced UUID-led repeated release cards with a selected-release review.
  Pending releases are selected first, followed by the active release, with
  readable date/status labels, grouped permission summaries, new-permission
  markers, exact HTTPS destination details, and explicit unsupported-permission
  blocking.
- Split the review surface into fixed header/footer and one internal scroll
  region. Desktop uses a wider bounded dialog; phone uses a full-height,
  safe-area-aware surface with touch-sized actions. Existing approval,
  rejection, rollback, promotion, and error semantics remain intact.
- Added backend projection tests, permission-copy tests, release-dialog tests,
  all supported locale keys, and public release/security guidance.
- `(cd apps/backend && go test ./internal/backendapp ./internal/canvas ./internal/plugins/instances ./internal/plugins/webapp ./internal/mcp/canvasskill)`: passed.
- `(cd apps/web && pnpm exec vitest run ... nine canvas/runtime files ...)`: 45
  tests passed.
- `(cd apps/web && pnpm run typecheck)`: passed.
- `(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)`: passed.
- Managed desktop and mobile Canvas E2E suites passed, including release
  recovery and the mobile focused canvas route.
