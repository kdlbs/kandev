---
id: "02-connections-geometry"
title: "Repair touch MCP Connections geometry"
status: done
wave: 2
depends_on:
  - "01-picker-consistency"
plan: "plan.md"
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
acceptance_criteria:
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.4
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.5
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.9
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.19
system_design:
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
---

# Task 02: Repair touch MCP Connections geometry

## Summary

Let server rows contain their complete explanatory text. Center touch header
copy on the explorer surface while preserving its close action and existing
server/tool navigation. Deliver verified assets to the owned preview.

## In scope

Feature-local responsive height overrides, balanced touch header, rendered
geometry regressions, testing-guide checks and guarded asset-only deployment.

## Out of scope

Global Button/control sizing, status evidence semantics, session controller or
polling, saved selections, runtime restart, reseeding or modifying main :9998.

## Acceptance

1. Every visible server row contains its name/status/explanation/icons; no
   content overlaps a following row. Rows remain keyboard/tap accessible and
   at least 44px tall on phone/coarse pointer, with content height as needed.
2. Touch title and description are centered on the drawer midpoint within 1px
   with computed centered text alignment. Close remains at least 44x44px,
   viewport-contained and separate from wrapped text. Desktop stays left-aligned.
3. Fixed header/views, one active scroller, final-row reachability, back/dismiss
   and focus return survive at 320/393/767px phone, 900px coarse tablet and
   1440px fine-pointer desktop. User preview selections remain byte-for-byte
   equal before/after asset deployment, with protected process identities intact.

## ASCII UI preview

UI-03 excerpt from [combined preview](plan.md#ascii-ui-preview):

```text
[space]           MCP servers                [Close]
             Centered explanatory copy
Connections | Customize                     (fixed)
---------------------------------------------------
CURRENT AGENT                               (scrolls)
  o atlas-docs                            [status]
    Delivered, connection unverified
    Configuration sent to the agent. The connection
    has not been verified.

  o issue-tracker                         [status]
    Not available
    This agent does not support HTTP MCP servers.
```

Map to AC-006.4/5/9/19. Content determines each row's height, with a 44px touch
minimum. Desktop retains its server/tool split and left-aligned header. Empty,
loading and unknown states keep their existing text and navigation.

## Verification

Write `contains multiline MCP connection rows and centers the touch header`
in the existing chat E2E specs. Reproduce the regression before changing code;
assert bounding boxes for each row, all its content and the following row.
Use mixed short/long/unknown translated states, not a single short Active row.
Do not infer correctness from `height >= 44` or class names alone.

Run from the repository root, sequentially:

```bash
(cd apps/web && corepack pnpm@9.15.9 run typecheck)
(cd apps && corepack pnpm@9.15.9 --filter @kandev/web lint)
make build-web PNPM="corepack pnpm@9.15.9"
(cd apps/web && corepack pnpm@9.15.9 e2e:run --host --no-build --project chromium e2e/tests/chat/mcp-status.spec.ts e2e/tests/settings/workspace-mcp-configuration.spec.ts e2e/tests/task/task-mcp-selection.spec.ts --grep 'desktop MCP explorer|explains mixed connections|tablet MCP connections|one disclosure|matches neighboring Advanced' --retries=0)
(cd apps/web && corepack pnpm@9.15.9 e2e:run --host --no-build --project mobile-chrome e2e/tests/chat/mobile-mcp-status.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/mcp-picker-mobile-repair
```

Perform read-only screenshots on the existing :48595 preview after guarded
asset copy. Extend the existing temporary `reproduce-review-layout.mjs` to
check phone/coarse/desktop sizes and both themes; inspect the screenshot as
well as numerical results. Selection-write interception remains enabled.
Inspect the owned deployment helper before using it: validate PID start IDs,
snapshot the four scope-selection tables read-only, copy built assets into
`/tmp/kandev-pr3293-test/apps/web/dist` retaining older hash assets, replace
index atomically and compare selection snapshots exactly. Record new index
hash and process identities. Never restart/stop :9998 or blindly kill a PID.
If any guard fails, leave deployment pending rather than copying to an unknown
instance. No production backend change is expected.

Update the manual guide to require non-overlapping connection rows, centered
touch header and contained close action. Synchronize this plan and both work
orders with actual command results; no invented test counts or reused passes.

## Files likely touched

- `apps/web/components/task/chat/mcp-explorer/mcp-server-list.tsx`
- `apps/web/components/task/chat/mcp-explorer/mcp-explorer-header.tsx`
- `apps/web/e2e/tests/chat/mcp-status.spec.ts`
- `apps/web/e2e/tests/chat/mobile-mcp-status.spec.ts` and bounded geometry helpers
- `docs/plans/workspace-mcp-configuration/testing-guide.md`

## Dependencies

Task 01. This order keeps implementation and preview delivery sequential;
it does not imply a domain dependency or authorize delegation.

## Risks

The Button primitive adds responsive fixed heights that base `h-auto` cannot
remove. Match both responsive variants locally; do not change all controls.
Narrow translated copy can collide with close if gutters are not symmetric.
The user's preview has live edits, so do not reseed it to simplify test data.

## Parallelism

`sequential`

## Inputs

Owning design Responsive behavior, shared `controlSizingClassName`, current
explorer header/drawer, `MobilePickerSheet`, and read-only reproduction evidence
listed in the plan. Keep session-owned evidence interpretation unchanged.

## Results

Implemented. RED: the 393px regression failed the content/row containment
predicate before production edits. The fix overrides only the feature's phone
and coarse-pointer heights and centers the title using balanced columns; the
centered description spans the row below it.

- Typecheck and full frontend lint: passed. Targeted geometry-helper lint and
  all changed TS/TSX formatting checks also passed after the final test edit.
- Production web build: passed. No backend build/restart was needed.
- Final desktop command above: 5 passed (all 3 chat scenarios, including the
  900px touch tablet, plus repository/task regression reruns).
- Required phone chat command: 4 passed (existing tool navigation plus mixed
  geometry at 320px, 393px and 767px).
- Final phone picker rerun: 2 passed using the Task 01 mobile command with
  `--grep 'one disclosure|keeps task additions inside' --retries=0`.
- Catalog validation, all-spec lint, whitespace check and PR-documentation
  coverage preflight: passed. Public docs validation: 47 pages passed.
- Asset-only deployment to :48595: passed with exact scope-selection row
  equality and unchanged owned backend/proxy/main process start identities.
- Read-only deployed checks: desktop, phone dark/light, 320px phone, 767px
  fine-pointer phone, 900px coarse tablet and 320px Portuguese. Every tested
  row contains its explanation; touch header offsets are 0px, no horizontal
  overflow, and the final Customize choice remains reachable. English phone
  rows now measure 70-86px as content requires, replacing the fixed 44px rows.
- Inspected deployed desktop, light-phone and Portuguese screenshots. Their
  geometry agrees with UI-03; existing desktop server/tool composition remains.

Evidence: `/tmp/kandev-pr3293-test-runtime/picker-mobile-repair-deployment.json`,
`picker-mobile-repair-visual.json`, `repair-documentation-coverage.json`, and
`repaired-connections-*.png`. Browsers closed; no reseeding, saved-selection
mutation or main :9998 restart. Changes remain local and uncommitted.
