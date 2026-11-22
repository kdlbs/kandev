---
id: "02-prove-and-deliver"
title: "Prove and deliver the desktop and phone journeys"
status: done
wave: 2
depends_on:
  - "01-explain-status"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-MCP-SESSION-OBSERVABILITY-001
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
acceptance_criteria:
  - AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.13
  - AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.14
  - AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.15
  - AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.16
  - AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.17
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.9
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.10
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.11
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.14
system_design:
  - ../../specs/platform/system-design/mcp-session-observability.md
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
---

# Task 02: Prove and deliver the desktop and phone journeys

## Summary

Exercise the actual desktop and phone surfaces with mixed evidence and saved
states, then deploy the verified frontend to the existing owned preview.
Keep the five complex sandbox tasks and record the limits of live mock coverage.

## In scope

- Extend existing desktop and phone MCP explorer specs with the new copy outcomes.
- Current production web build, targeted browser tests, public-doc wording.
- Owned sandbox deployment and a recorder-free desktop/phone rehearsal.

## Out of scope

Full-suite audit, main instance or database mutation, registry refresh, real
providers, unnecessary cross-platform builds, and commit/push.

## Acceptance

1. Desktop hover/focus and phone tap expose the same reasons and selection
   notices; assert an Active row retains its own explanation in a mixed set.
2. Long labels wrap, one phone drawer remains within the viewport, required
   targets measure at least 44px, Back/focus works, and refresh retains selection.
   Customize exposes named semantic groups and distinct status icon/headline
   and theme accent. Group spacing exceeds row spacing, origins wrap, and
   screenshot checks confirm hierarchy in both themes without relying on color.
3. Forwarded save/reload works on owned port 48595, original data is retained,
   main 9998 is unchanged, and public docs/guide describe the actual result.

## ASCII UI preview

UI-03 from [the combined preview](plan.md#ascii-ui-preview):

```text
MCP servers                          [X]
[Connections] [Customize]
Saved changes: Saved, not applied yet.
Current agent
  kandev         Active                  >
  issue-tracker  Not available            >
    This agent does not support HTTP.
  atlas-docs     Delivered                >
    Connection not verified.
```

The existing full-height phone drawer owns one scroll body. Desktop retains
the wide dialog and read-only UI-02 hover. UI-04 notices replace the saved-change
region for deferred/failed states; applied omits it. AC-001.13 through .16 apply.

UI-06 and UI-07 from [the full Customize preview](plan.md#ascii-ui-preview):
application summary first, a visually grouped From task setup list second,
then Additional servers with its own search and stable choices. Phone wraps
origins and uses one scroll body. Platform .17 and agents .14 require both
hierarchy and words/icons that remain understandable without color. Applied
keeps a compact Customize summary but no pending hover notice.

## Verification

Run from repository root; each command owns its directory. Build fresh web assets explicitly. Locale generation refreshes embedded backend
file timestamps, so rebuild the native artifacts required by the freshness gate.
The guarded E2E launcher allocates isolated worker data. Run projects sequentially. Include the adjacent task-selection specs because
the existing clarity/refresh helpers and section selectors changed.

```bash
(cd apps && corepack pnpm@9.15.9 --filter @kandev/web build:e2e)
make -C apps/backend build-kandev build-agentctl build-mock-agent e2e-plugin-package
(cd apps/web && corepack pnpm@9.15.9 e2e:run --host --no-build --project chromium -- tests/chat/mcp-status.spec.ts tests/task/task-mcp-selection.spec.ts)
(cd apps/web && corepack pnpm@9.15.9 e2e:run --host --no-build --project mobile-chrome -- tests/chat/mobile-mcp-status.spec.ts tests/task/mobile-task-mcp-selection.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/plans/mcp-status-clarity
git status --short -- docs/plans/mcp-status-clarity
```

Deploy only the final web build to `/tmp/kandev-pr3293-test/apps/web/dist` after
recording prior artifact hashes and verifying owned process identities. The
existing static server may require only a browser reload; do not restart the
main instance. Use a named owned Playwright CLI browser and close it afterward.
Rehearse cases 01 through 05 according to the testing guide, restoring only
rehearsal mutations on those scenario records in finally/afterEach. Save exact
results and screenshot paths under the owned runtime root. Preserve manifest
and update the existing PID-checked shutdown script if ownership changes.

## Files likely touched

- `apps/web/e2e/tests/chat/mcp-status.spec.ts` and `mobile-mcp-status.spec.ts`.
- Existing MCP E2E helpers if a shared controlled-report fixture is needed.
- `docs/public/agents-and-profiles.md` and `docs/plans/mcp-status-clarity/testing-guide.md`.
- Owned ephemeral runtime manifests and build artifacts, outside source.

## Dependencies

Task 01.

## Risks

Mock delivery does not prove stdio launch or third-party connectivity. Test
reports with seeded Failed/deferred states must be distinguished from actual
provider events. Do not mistake a generic tool-call error for attachment failure.

## Parallelism

`sequential`

## Inputs

- Parent plan's compatibility matrix and UI-02 through UI-04.
- Testing guide, existing runtime ownership manifest, mobile-parity and E2E skills.
- Task 01 passing presenter/component checks.

## Results

Completed inherited metadata badge delivery. The shared status journey now
adds/restores profile and repository choices in its isolated E2E worker so one
definition is genuinely inherited through profile, repository and task. Assert
the description, metadata labels, named scope badges, complete long task name,
wrapping and row containment. Inspect owned live cases 01/02 in both themes and
deploy frontend assets only, preserving all four selection scopes and processes.
Desktop and final phone runs passed 2/2 each. A narrower 260px phone row verifies
multiline source-name wrapping, alongside natural-width badge containment.
Real cases 01/02 and both desktop/phone themes were inspected; supplied
descriptions, remote/SSE and executable/stdio badges and named multi-origins
render correctly. Owned preview 48595 serves the new frontend. All four
selection scopes and process identities were preserved; diagnostic browser
closed. Evidence paths and fixture corrections are recorded in the parent plan.

Completed the parent plan's dialog hierarchy revision. Added rendered external
heading/list hierarchy checks to the shared status journey, with RED against
the previous build. Desktop explorer/status 2/2 and phone 2/2 passed. Inspected
the real three-origin case in both themes on desktop and phone, including the
scrolled Additional servers list. Phone: one 390x844 drawer, 44px close target,
no document overflow. Deployed frontend assets to 48595, preserving all four
selection scopes, main/preview process identities and old assets for open tabs.
Results and evidence paths are recorded in the parent plan's revision section.

Completed on 2026-10-07. Final targeted browser evidence covers 16 distinct
cases: 2 desktop MCP explorer/status cases, 6 adjacent desktop task-selection
cases, and 8 phone cases. Desktop combined run passed the 6 task cases and
original explorer case; a mock-reconnect evidence assumption in the new status
case was corrected and its whole 2-case spec then passed. Phone combined run:
8/8 passed. No production state was changed to force Active; controlled reports
were limited to application-state presentation.

Fresh web build and native backend/mock/agentctl/plugin fixture artifacts passed
build and freshness checks. Scoped test discovery reported zero errors.
Final unit checks: 40/40; typecheck, scoped ESLint, translations/ratchet,
public docs (62 validator tests, 47 pages), catalog/spec lint and whitespace
passed. The public agents/profile how-to explains the new group names and
saved/current distinction. The test guide now describes the deployed UX.

Owned preview 48595 serves the final frontend. Live cases 01/02 prove named
origins, typed exclusion/delivery explanations and phone navigation. Case 03
save 200 survived reload and reached applied. Case 04 saved during a 12-second
turn, showed the new waiting headline, then applied in the same session; own
changes restored 200. Case 05 retained its disabled task selection and local
503 endpoint; agent start remains manual. No real-provider failure was claimed.

Light/dark desktop/phone screenshots and a waiting-state screenshot were
inspected under the owned runtime root. Phone drawer is 390x844 with a 44px
close control, one active drawer and no horizontal overflow. All four persisted
selection scopes match the pre-deployment snapshot. Main 9998 and owned preview
process start identities are unchanged; old hashed frontend assets retained for
open tabs. Diagnostic browser closed. No commit, push, delegation or task-local
exclusion implementation. Evidence: ux-rehearsal.json, ux-deployment.json and
ux-selection-preservation.json in /tmp/kandev-pr3293-test-runtime.

Post-delivery task 2 alignment repair: isolated dot and text columns keep wrapped
statuses aligned with names and explanations. Desktop 2/2 and phone 2/2 checks
passed; a final visible-tooltip geometry regression rerun passed 1/1. The
regression explicitly measures the visual content rather than the screen-reader
copy. Scoped lint, formatting and fresh web build passed. Live preview task 2
at 240px content width had matching text left edges; the previous visual layout
reproduced a 16px shift. Frontend-only deployment preserved task 2 selection,
process identities and old hashed assets. See the parent plan's repair results
and `alignment-deployment.json` in the owned runtime root.

Completed the parent plan's Customize scroll/source-heading repair. The
coarse-pointer drawer has a definite height and inherited rows have an explicit
translated source subheading. Tablet RED and final desktop/tablet/phone long-list
journeys are documented in the parent plan. Source badges retain resolved names
and every contributing scope; saved selections and process identities were
preserved through frontend-only deployment on 48595.
