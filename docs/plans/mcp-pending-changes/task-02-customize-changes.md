---
id: "02-customize-changes"
title: "Show exact changes waiting to apply"
status: done
wave: 2
depends_on:
  - "01-accepted-setup"
plan: "plan.md"
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
acceptance_criteria:
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.15
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.16
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.17
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.18
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.19
system_design:
  - ../../specs/agents/system-design/workspace-mcp-pending-changes.md
---

# Task 02: Show exact changes waiting to apply

## Outcome and ownership

Customize names the saved server actions and application timing. Own
`apps/web/lib/types/http-mcp.ts`, session selection hook/response guards and
adjacent tests; `components/task/mcp-session-selector.tsx`; shared selection
rows; a new small `mcp-selection-changes` presenter and its tests beside the
existing explorer notices. Own desktop/phone pending-change E2E, localized
settings keys and `docs/public/agents-and-profiles.md`. Read web AGENTS first.

Reuse the accepted comparison from Task 01. Do not infer it from browser-open
state or attachment health. Other selection callers, Connections, inherited
read-only choices, automatic save and lifecycle timing remain intact.

## Acceptance

1. Show named add/remove/update actions before selection sections with exact
   timing and count only when known. Each affected selection row carries its
   action; unchecked removal and deleted baseline entries remain understandable
   independent of catalog search.
2. Reload/reopen/session switching, saving/rejection, apply failure/retry,
   known-empty and unavailable comparisons preserve committed authority and
   explain their different meanings without green health claims or revisions.
3. Desktop and phone share logic, preserve focus/search/scroll on refresh and
   match UI-02/03/04 without new overlays or scroll owners. All new copy is
   translated and the longest list remains reachable with 44px touch controls.

## ASCII UI preview

UI-02/03 excerpt; see [full previews](plan.md#ascii-ui-preview).

```text
Desktop: Customize (one existing dialog body)
Changes waiting to apply [2]
Previous setup stays until idle.
[+] Atlas docs     Add to setup
[-] Issue tracker  Remove from setup
FROM TASK SETUP
ADDITIONAL SERVERS
[Search] [x] Atlas docs + action; [ ] Issue tracker + action

Phone: existing drawer
MCP servers [Close] / Connections | Customize  (fixed)
Changes waiting to apply [2]                  (scroll body)
[+] Atlas docs / Add to setup
[-] Issue tracker / Remove from setup
FROM TASK SETUP / ADDITIONAL SERVERS / [Search] / choices
```

UI-04: failed apply keeps names and Retry; deferred changes say next start;
unknown has no invented actions/count; known-empty explains equal setup while
acknowledgment is pending; applied clears annotations. Keep header/views fixed,
one safe-area-aware body scroll owner and no hover-only functionality.

## Verification

TDD shared comparison/presentation tests and hooked state tests first. Add
`e2e/tests/chat/mcp-pending-changes.spec.ts` and
`mobile-mcp-pending-changes.spec.ts` with real saved selections and explicitly
controlled accepted/setup evidence. Cover add/remove/update, reload, applied,
deferred, failed apply/retry, rejected save, revert, duplicate inherited origins,
missing baseline, stale comparison and sibling session isolation. Long lists
must scroll to the final choice with title/tabs fixed; keyboard dismissal
returns focus. Search must not hide the pending action list.

From apps/web, run in order:

```sh
corepack pnpm@9.15.9 exec vitest run components/task/chat/mcp-explorer/mcp-selection-changes.test.ts hooks/domains/session/use-task-session-mcp-selection.test.ts
corepack pnpm@9.15.9 run typecheck
corepack pnpm@9.15.9 run i18n:check
corepack pnpm@9.15.9 run i18n:ratchet
corepack pnpm@9.15.9 e2e:run --host --project chromium -- tests/chat/mcp-pending-changes.spec.ts
corepack pnpm@9.15.9 e2e:run --host --no-build --project mobile-chrome -- tests/chat/mobile-mcp-pending-changes.spec.ts
```

Use scoped ESLint/Prettier on all changed source/tests/catalogs. The first E2E
run must build current native/web artifacts; never overlap suites. Update public
docs using docs-maintainer and run its required checks. Finish with docs catalog,
all-spec lint and whitespace checks. Record exact tests and results.

## Delivery and risks

After Task 01/02 checks pass, re-inspect owned preview ports/process start IDs
and database schema. Back up the isolated preview DB, exercise migration on its
copy and compare all four selection tables before/after an owned runtime swap.
Preserve user's sandbox edits and old frontend assets, then rehearse the exact
change list on 48595. Do not affect :9998. No commit/push or delegation.

Snapshot absence will be normal for older sessions; explain it without
requiring a destructive reset. A known empty difference is not a false Applied
state. Retry and checkbox behavior must remain functional on phone.

## Results

Implemented the shared exact-change presenter and named pending list beneath
application timing. Matching inherited/additional rows show action words/icons;
unchecked removals explain that the old setup is still accepted. Known-empty
and unavailable comparisons remain distinct. Polling now refreshes deferred
and failed application states as well as pending-idle, stopping when applied.
New copy is localized in all seven shipped languages and pseudo.

Behavioral RED/GREEN covered stale comparisons and deferred/failed refresh.
The focused Vitest run passed 15 tests. Source and E2E scoped ESLint,
TypeScript, i18n check/ratchet and production web build passed. The manually
built host-native runtime, agentctl, mock agent and plugin fixtures were fresh;
the managed runner's artifact freshness guard passed. Exact browser commands:

```sh
corepack pnpm@9.15.9 e2e:run --host --no-build --project chromium e2e/tests/chat/mcp-pending-changes.spec.ts --trace on
corepack pnpm@9.15.9 e2e:run --host --no-build --project mobile-chrome e2e/tests/chat/mobile-mcp-pending-changes.spec.ts --trace on
```

Both passed sequentially, one worker each. A normal real startup established
the accepted baseline; a held mock turn allowed genuine saved add/remove/update
changes. The journey verifies deleted names, inline update, search independence,
long-list wheel/touch scrolling, rejected save, reverting an addition, retry,
unknown/empty/stale reports, focus return and real idle acceptance clearing the
open Customize view. Controlled response states exercise presentation, not
third-party connectivity. An early fixture stopped the agent and hid the
composer; another used an incorrect phone fixture name. Both harness issues
were corrected without weakening assertions.

Public docs explain the new list and baseline distinction (existing page:
explanation with how-to sections). All 62 public-doc validator tests and all
47 published pages passed; catalog and all-spec lint passed. Preview delivery and visual verification passed. On 2026-10-10 only the owned
backend on 48593 was replaced; its existing proxy remains on 48595. A fresh DB
backup preceded the migration. All four selection-table hashes stayed identical
(1 profile, 3 repository, 8 task, 11 session rows), and main PID/start identity on
9998 stayed unchanged. Old hashed frontend assets were retained.

Seeded only disposable task 06 with real accepted setup and genuine pending
add/remove/update actions. After seeding, every pre-existing selection row was
still present and unchanged. Desktop dark/light and phone captures passed with
zero horizontal overflow; unchecked removal explanations and inherited update
annotations were verified and visually inspected. Evidence:
`/tmp/kandev-pr3293-test-runtime/pending-deployment.json`,
`pending-scenario.json`, `pending-visual-check.json` and `pending-*.png`.
The fixture holds a mock turn; cancelling it makes normal idle application
accept the saved setup. The existing guarded shutdown command remains
`python3 /tmp/kandev-pr3293-test-runtime/stop.py`. No main-instance mutation,
commit, push, merge or delegation.


## Proposed origin-section continuation (2026-10-14)

The [Customize origin sections package](../mcp-customize-origins/plan.md)
proposes replacing the inherited origin-chip presentation and enriching session
choice rows. This record's delivered status, historical previews and checks
remain unchanged. The continuation is not implemented; its work order owns the
new composition and regression evidence. Application and pending-change
semantics from this delivery remain in force.
